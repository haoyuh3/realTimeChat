package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	
)

// WebSocket 升级器
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

// WebSocket 客户端连接
type WSClient struct {
	conn       *websocket.Conn
	username   string
	grpcConn   *grpc.ClientConn
	grpcStream pb.ChatService_RealtimeChatClient
	send       chan []byte
	hub        *WSHub
}

// WebSocket 中心管理器
type WSHub struct {
	clients    map[*WSClient]bool
	broadcast  chan []byte
	register   chan *WSClient
	unregister chan *WSClient
	mu         sync.RWMutex
}

// 消息结构
type WSMessage struct {
	Type          string `json:"type"`
	User          string `json:"user"`
	Text          string `json:"text"`
	RecipientUser string `json:"recipientUser,omitempty"`
	Timestamp     string `json:"timestamp"`
}

func newWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

func (h *WSHub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("WebSocket client registered: %s", client.username)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				if client.grpcConn != nil {
					client.grpcConn.Close()
				}
			}
			h.mu.Unlock()
			log.Printf("WebSocket client unregistered: %s", client.username)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *WSHub) getOnlineUsers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	users := make([]string, 0, len(h.clients))
	for client := range h.clients {
		if client.username != "" {
			users = append(users, client.username)
		}
	}
	return users
}

func setupRouter(hub *WSHub) *gin.Engine {
	r := gin.Default()

	// 服务静态文件
	r.Static("/static", "./web/static")
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/favicon.ico", "./web/static/images/favicon.ico")

	// API 路由
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong!",
		})
	})

	// WebSocket 路由
	r.GET("/ws", func(c *gin.Context) {
		handleWebSocket(hub, c.Writer, c.Request)
	})

	// 用户相关 API
	r.GET("/api/users", func(c *gin.Context) {
		users := hub.getOnlineUsers()
		c.JSON(http.StatusOK, gin.H{
			"users": users,
			"count": len(users),
		})
	})

	return r
}

func handleWebSocket(hub *WSHub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := &WSClient{
		conn: conn,
		send: make(chan []byte, 256),
		hub:  hub,
	}

	// 注册客户端
	client.hub.register <- client

	// 启动协程处理读写
	go client.writePump()
	go client.readPump()
}

func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}

		var wsMsg WSMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			log.Printf("JSON unmarshal error: %v", err)
			continue
		}

		// 处理不同类型的消息
		switch wsMsg.Type {
		case "join":
			c.handleJoin(wsMsg)
		case "chat":
			c.handleChat(wsMsg)
		}
	}
}

func (c *WSClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *WSClient) handleJoin(msg WSMessage) {
	c.username = msg.User

	// 连接到 gRPC 服务器
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Failed to connect to gRPC server: %v", err)
		c.sendError("Failed to connect to chat server")
		return
	}
	c.grpcConn = conn

	client := pb.NewChatServiceClient(conn)
	stream, err := client.RealtimeChat(context.Background())
	if err != nil {
		log.Printf("Failed to start gRPC stream: %v", err)
		c.sendError("Failed to start chat stream")
		return
	}
	c.grpcStream = stream

	// 发送加入消息到 gRPC 服务器
	joinMsg := &pb.ChatMessage{
		User: c.username,
		Text: "has joined",
	}

	if err := stream.Send(joinMsg); err != nil {
		log.Printf("Failed to send join message: %v", err)
		c.sendError("Failed to join chat")
		return
	}

	// 启动协程接收 gRPC 消息
	go c.handleGRPCMessages()

	// 发送用户列表更新
	c.sendUserList()

	// 广播用户加入消息
	c.broadcastUserJoin()
}

func (c *WSClient) handleChat(msg WSMessage) {
	if c.grpcStream == nil {
		c.sendError("Not connected to chat server")
		return
	}

	grpcMsg := &pb.ChatMessage{
		User:          msg.User,
		Text:          msg.Text,
		RecipientUser: msg.RecipientUser,
	}

	if err := c.grpcStream.Send(grpcMsg); err != nil {
		log.Printf("Failed to send message to gRPC: %v", err)
		c.sendError("Failed to send message")
	}
}

func (c *WSClient) handleGRPCMessages() {
	for {
		if c.grpcStream == nil {
			break
		}

		msg, err := c.grpcStream.Recv()
		if err != nil {
			log.Printf("gRPC stream receive error: %v", err)
			break
		}

		// 转换为 WebSocket 消息
		wsMsg := WSMessage{
			Type:          "chat",
			User:          msg.User,
			Text:          msg.Text,
			RecipientUser: msg.RecipientUser,
			Timestamp:     time.Now().Format(time.RFC3339),
		}

		data, _ := json.Marshal(wsMsg)
		c.send <- data
	}
}

func (c *WSClient) sendUserList() {
	users := c.hub.getOnlineUsers()
	msg := map[string]interface{}{
		"type":  "userList",
		"users": users,
	}
	data, _ := json.Marshal(msg)
	c.send <- data
}

func (c *WSClient) broadcastUserJoin() {
	msg := map[string]interface{}{
		"type": "userJoin",
		"user": c.username,
	}
	data, _ := json.Marshal(msg)
	c.hub.broadcast <- data
}

func (c *WSClient) sendError(message string) {
	msg := map[string]interface{}{
		"type": "error",
		"text": message,
	}
	data, _ := json.Marshal(msg)
	c.send <- data
}
func main() {
	// 创建 WebSocket 中心管理器
	hub := newWSHub()
	go hub.run()

	// 创建路由器
	r := setupRouter(hub)

	// 启动 HTTP 服务器
	log.Println("Web server starting on :8080")
	log.Println("访问 http://localhost:8080 使用 Web 聊天客户端")

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
