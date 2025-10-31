package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "realTimeChat/proto/chat"
)

func main() {
	// 1. read username
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter your username: ")
	userName, _ := reader.ReadString('\n') // read until newline
	userName = strings.TrimSpace(userName)
	if userName == "" {
		log.Fatalf("Username cannot be empty")
	}

	// 2. connect to grpc
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials())) // insecure for localhost
	if err != nil {
		log.Fatalf("Did not connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewChatServiceClient(conn) // set client

	// 3. RealtimeChat RPC，to get stream
	stream, err := c.RealtimeChat(context.Background())
	if err != nil {
		log.Fatalf("Could not start chat: %v", err)
	}

	// 4. send join message
	if err := stream.Send(&pb.ChatMessage{User: userName, Text: "has joined"}); err != nil {
		log.Fatalf("Failed to send join message: %v", err)
	}
	fmt.Printf("Connected as %s. Type 'exit' to quit.\n", userName)
	fmt.Println("---------------------------------------")

	// 5. start a goroutine to *receive* messages
	waitc := make(chan struct{}) // close signal
	go readRoutine(stream, waitc, userName)

	// 6. send message
	// for + scanner to read from stdin
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.ToLower(text) == "exit" {
			break
		}

		recipient := "" // empty means public message
		messageText := text

		// structure: /pm <username> <message>
		if strings.HasPrefix(text, "/pm ") {
			parts := strings.SplitN(text, " ", 3)
			if len(parts) < 3 || parts[1] == "" || parts[2] == "" {
				fmt.Println("Invalid PM format. Use: /pm <username> <message>")
				continue
			}
			recipient = parts[1]
			messageText = parts[2]
		}

		// create message
		msg := &pb.ChatMessage{
			User:          userName,
			Text:          messageText,
			RecipientUser: recipient,
		}

		if err := stream.Send(msg); err != nil {
			log.Printf("Failed to send message: %v", err)
			break
		}
	}

	// 7. close the send direction of the stream
	if err := stream.CloseSend(); err != nil {
		log.Printf("Failed to close send stream: %v", err)
	}

	// 8. wait for the read goroutine to finish
	<-waitc
	log.Println("Disconnected.")
}

func readRoutine(stream pb.ChatService_RealtimeChatClient, waitc chan struct{}, userName string) {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			log.Println("Server closed the connection")
			close(waitc)
			return
		}
		if err != nil {
			log.Printf("Failed to receive message: %v", err)
			close(waitc)
			return
		}
		if msg.RecipientUser != "" {
			// pm
			if msg.User == userName {
				fmt.Printf("[You to %s (PM)]: %s\n", msg.RecipientUser, msg.Text)
			} else {
				fmt.Printf("[%s (PM)]: %s\n", msg.User, msg.Text)
			}
		} else {
			fmt.Printf("[%s]: %s\n", msg.User, msg.Text)
		}
	}
}
