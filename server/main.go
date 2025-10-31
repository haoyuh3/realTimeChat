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

// connection store stream and user info
type connection struct {
	stream pb.ChatService_RealtimeChatServer
	user   string
}

// ChatServer struct
type ChatServer struct {
	pb.UnimplementedChatServiceServer
	mu          sync.RWMutex          // read write mutex to protect connections map
	connections map[string]connection // store active connection
}

// NewChatServer creates a new ChatServer
func NewChatServer() *ChatServer {
	return &ChatServer{
		connections: make(map[string]connection),
	}
}

// sendRoutine sends a message to a specific stream
func (s *ChatServer) sendRoutine(stream pb.ChatService_RealtimeChatServer, msg *pb.ChatMessage, username string) {
	if err := stream.Send(msg); err != nil {
		log.Printf("Failed to send PM to %s: %v", username, err)
	}
}

func (s *ChatServer) sendToUser(username string, msg *pb.ChatMessage) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	found := false
	for _, conn := range s.connections {
		if conn.user == username {
			go s.sendRoutine(conn.stream, msg, username)
			found = true
		}
	}
	return found
}

// RealtimeChat define in proto file
func (s *ChatServer) RealtimeChat(stream pb.ChatService_RealtimeChatServer) error {
	log.Println("New client connected...")

	// 1. accept the first message which should contain user info
	firstMsg, err := stream.Recv()
	if err != nil {
		log.Printf("Failed to receive first message: %v", err)
		return status.Error(codes.InvalidArgument, "First message must contain user info")
	}
	userName := firstMsg.User
	if userName == "" {
		return status.Error(codes.InvalidArgument, "Username cannot be empty")
	}

	// 2. create a unique client ID
	clientID := fmt.Sprintf("%s_%p", userName, stream)

	// 3. store connection to map
	s.mu.Lock()
	s.connections[clientID] = connection{
		stream: stream,
		user:   userName,
	}
	s.mu.Unlock()

	log.Printf("User '%s' (ID: %s) joined.", userName, clientID)

	// 4. broadcast joined msg
	joinMsg := &pb.ChatMessage{User: "System", Text: fmt.Sprintf("%s has joined the chat", userName)}
	s.broadcast(joinMsg, clientID)

	// 5. hear from client
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			// stream close
			break
		}
		if err != nil {
			log.Printf("Error receiving from %s: %v", clientID, err)
			break
		}

		if msg.RecipientUser == "" {
			// broadcast message
			log.Printf("Broadcasting message from %s: %s", msg.User, msg.Text)
			s.broadcast(msg, clientID)
		} else {
			// pm message
			log.Printf("Private message from %s to %s", msg.User, msg.RecipientUser)

			// 1. send to recipient
			found := s.sendToUser(msg.RecipientUser, msg)

			// 2. send copy back to sender
			if err := stream.Send(msg); err != nil {
				log.Printf("Failed to send PM copy back to sender %s: %v", clientID, err)
			}

			// 3. notify sender if recipient not found
			if !found {
				systemMsg := &pb.ChatMessage{
					User: "System",
					Text: fmt.Sprintf("User '%s' not found or is offline.", msg.RecipientUser),
				}
				if err := stream.Send(systemMsg); err != nil {
					log.Printf("Failed to send 'user not found' to %s: %v", clientID, err)
				}
			}
		}
	}

	// 7. close connection
	s.mu.Lock()
	delete(s.connections, clientID)
	s.mu.Unlock()

	log.Printf("User '%s' (ID: %s) disconnected.", userName, clientID)

	// 8. broadcast left msg
	leaveMsg := &pb.ChatMessage{User: "System", Text: fmt.Sprintf("%s has left the chat", userName)}
	s.broadcast(leaveMsg, "")

	return nil
}

// broadcast message to all clients except the sender
func (s *ChatServer) broadcast(msg *pb.ChatMessage, excludeID string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for id, conn := range s.connections {
		if id == excludeID {
			continue // skip sender
		}
		go s.sendRoutine(conn.stream, msg, conn.user)
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
