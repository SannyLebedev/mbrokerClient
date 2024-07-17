package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	pb "github.com/SannyLebedev/mbrokerClient/proto"
	uuid "github.com/satori/go.uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var clientID = uuid.NewV4().String()

type CustomMessage struct {
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

func main() {
	for {
		err := run()
		if err != nil {
			log.Printf("Failed to run client: %v", err)
		}
		log.Println("Reconnecting...")
		time.Sleep(3 * time.Second)
	}
}

func run() error {
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer conn.Close()

	log.Printf("Current client id: %s \n", clientID)
	log.Println("Connecting...")
	client := pb.NewMessageBrokerClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	projectStream, err := client.Subscribe(ctx, &pb.SubscribeRequest{
		Project:       "A",
		ClientId:      clientID,
		RecipientType: pb.ServiceType_PROJECT,
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to project: %v", err)
	}

	// mgwStream, err := client.Subscribe(ctx, &pb.SubscribeRequest{
	// 	Project:       "",
	// 	ClientId:      clientID,
	// 	RecipientType: pb.ServiceType_MGW,
	// })
	// if err != nil {
	// 	return fmt.Errorf("failed to subscribe to mgw: %v", err)
	// }
	log.Println("Connected...")

	errCh := make(chan error)
	doneCh := make(chan struct{})

	go sendMessages(client, errCh, doneCh)
	go receiveMessages(projectStream, errCh, doneCh)
	// go receiveMessages(mgwStream, errCh, doneCh)

	for {
		select {
		case err := <-errCh:
			log.Printf("Error from goroutine: %v", err)
			close(doneCh)
			return err
		case <-ctx.Done():
			log.Println("Client disconnected, attempting to reconnect...")
			close(doneCh)
			return ctx.Err()
		}
	}

}

func sendMessages(client pb.MessageBrokerClient, errCh chan<- error, doneCh <-chan struct{}) {

	for {

		select {
		case <-doneCh:
			log.Println("sendMessages: done channel closed, exiting...")
			return
		default:

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			msg := &CustomMessage{
				Content:   "Hello from client!",
				Timestamp: time.Now().Unix(),
			}
			msgBytes, err := json.Marshal(msg)
			if err != nil {
				errCh <- fmt.Errorf("failed to marshal message: %v", err)
				return
			}

			req := &pb.SendMessageRequest{
				Project:       "A",
				RecipientType: pb.ServiceType_PROJECT,
				SenderType:    pb.ServiceType_PROJECT,
				Message:       msgBytes,
				ClientId:      clientID,
			}

			if _, err = client.SendMessage(ctx, req); err != nil {
				errCh <- fmt.Errorf("error sending message to project: %v", err)
				return
			}

			mgwReq := &pb.SendMessageRequest{
				Project:       "",
				ClientId:      clientID,
				Message:       msgBytes,
				RecipientType: pb.ServiceType_MGW,
				SenderType:    pb.ServiceType_PROJECT,
			}
			if _, err = client.SendMessage(context.Background(), mgwReq); err != nil {
				errCh <- fmt.Errorf("error sending message to mgw: %v", err)
				return
			}

			time.Sleep(10 * time.Second)
		}
	}
}

func receiveMessages(stream pb.MessageBroker_SubscribeClient, errCh chan<- error, doneCh <-chan struct{}) {
	for {
		select {
		case <-doneCh:
			log.Println("receiveMessages: done channel closed, exiting...")
			return
		default:
			resp, err := stream.Recv()
			if err != nil {
				errCh <- fmt.Errorf("error receiving message from server: %v", err)
				return
			}
			log.Printf("sender_type: %+v \n", resp.SenderType)
			var msg CustomMessage
			err = json.Unmarshal(resp.Message, &msg)
			if err != nil {
				log.Printf("Failed to unmarshal message: %v", err)
				continue
			}

			log.Printf("Received message from server: %v", msg)
		}
	}
}
