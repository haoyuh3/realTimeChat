// server/main.go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "realTimeChat/proto/chat"
)

// connection 结构体用于存储每个客户端的流和用户名
type connection struct {
	stream pb.ChatService_RealtimeChatServer
	user   string
}

// ChatServer 实现了 ChatServiceServer 接口
// 它管理着所有活跃的客户端连接
type ChatServer struct {
	pb.UnimplementedChatServiceServer
	mu          sync.RWMutex          // 用于保护 connections map 的读写锁
	connections map[string]connection // 存储所有活跃的连接 (key: 客户端唯一ID)
}

func NewChatServer() *ChatServer {
	return &ChatServer{
		connections: make(map[string]connection),
	}
}

// RealtimeChat 是 .proto 文件中定义的 RPC 方法的实现
func (s *ChatServer) RealtimeChat(stream pb.ChatService_RealtimeChatServer) error {
	log.Println("New client connected...")

	// 1. 接收第一条消息，它必须包含用户名
	firstMsg, err := stream.Recv()
	if err != nil {
		log.Printf("Failed to receive first message: %v", err)
		return status.Error(codes.InvalidArgument, "First message must contain user info")
	}
	userName := firstMsg.User
	if userName == "" {
		return status.Error(codes.InvalidArgument, "Username cannot be empty")
	}

	// 2. 为此连接创建一个唯一的 ID
	clientID := fmt.Sprintf("%s_%p", userName, stream) // 使用用户名和流的内存地址作为简单唯一ID

	// 3. 将新连接添加到 map 中（受锁保护）
	s.mu.Lock()
	s.connections[clientID] = connection{
		stream: stream,
		user:   userName,
	}
	s.mu.Unlock()

	log.Printf("User '%s' (ID: %s) joined.", userName, clientID)

	// 4. 广播 "加入" 消息给所有人
	joinMsg := &pb.ChatMessage{User: "System", Text: fmt.Sprintf("%s has joined the chat", userName)}
	s.broadcast(joinMsg, clientID) // 广播给除自己外的所有人

	// 5. 启动一个 goroutine 来持续接收来自此客户端的消息
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			// 客户端正常关闭了流 (例如: stream.CloseSend())
			break
		}
		if err != nil {
			// 发生错误 (例如: 客户端崩溃或网络中断)
			log.Printf("Error receiving from %s: %v", clientID, err)
			break
		}

		// 6. 收到消息，广播给所有人
		log.Printf("Broadcasting message from %s: %s", msg.User, msg.Text)
		s.broadcast(msg, clientID) // 广播给除自己外的所有人
	}

	// 7. 循环结束 (客户端断开连接)，清理
	s.mu.Lock()
	delete(s.connections, clientID)
	s.mu.Unlock()

	log.Printf("User '%s' (ID: %s) disconnected.", userName, clientID)

	// 8. 广播 "离开" 消息
	leaveMsg := &pb.ChatMessage{User: "System", Text: fmt.Sprintf("%s has left the chat", userName)}
	s.broadcast(leaveMsg, "") // 广播给所有人

	return nil
}

// broadcast 向所有连接的客户端发送消息
// excludeID 用于指定哪个客户端 *不* 接收此消息 (通常是发送者自己)
func (s *ChatServer) broadcast(msg *pb.ChatMessage, excludeID string) {
	s.mu.RLock() // 使用读锁，允许多个广播同时进行
	defer s.mu.RUnlock()

	for id, conn := range s.connections {
		if id == excludeID {
			continue // 跳过发送者
		}

		// 在单独的 goroutine 中发送，以避免一个慢客户端阻塞所有其他客户端
		go func(stream pb.ChatService_RealtimeChatServer) {
			if err := stream.Send(msg); err != nil {
				log.Printf("Failed to send message to %s: %v", id, err)
				// 在生产环境中，这里可能需要一个机制来清理失败的连接
			}
		}(conn.stream)
	}
}

func main() {
	port := ":50051"
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	chatServer := NewChatServer()
	pb.RegisterChatServiceServer(s, chatServer)

	log.Printf("Server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
