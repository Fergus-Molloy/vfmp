package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"fergus.molloy.xyz/vfmp/core/messages"
	"fergus.molloy.xyz/vfmp/core/tcp"
)

func main() {
	addr := flag.String("address", "localhost:9090", "TCP address to connect to")
	topic := flag.String("topic", "test", "Topic to consume messages from")
	n := flag.Int("n", 1, "Number of messages to consume")
	sequence := flag.String("sequence", "", "Comma-separated list of responses (ACK,NCK,DLQ). Implies n = len(sequence)")
	flag.Parse()

	// Parse the sequence into a slice of message types
	var responseSequence []messages.MessageType
	if *sequence != "" {
		parts := strings.SplitSeq(*sequence, ",")
		for part := range parts {
			msgType := messages.MessageType(strings.ToUpper(strings.TrimSpace(part)))
			// Validate the message type
			switch msgType {
			case messages.ACK, messages.NCK, messages.DLQ:
				responseSequence = append(responseSequence, msgType)
			default:
				fmt.Fprintf(os.Stderr, "Invalid response type: %s. Valid types are: ACK, NCK, DLQ\n", part)
				os.Exit(1)
			}
		}
	} else {
		for range *n {
			responseSequence = append(responseSequence, messages.MessageType("ACK"))
		}
	}

	signal, shutdown := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer shutdown()

	tcpAddr, err := net.ResolveTCPAddr("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not resolve address: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not dial address: %v\n", err)
		os.Exit(1)
	}

	wg := new(sync.WaitGroup)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c, clientCtx := tcp.NewClient(conn, signal, wg, logger)

	// Subscribe to the topic
	msg := fmt.Appendf(nil, "RDY|%s", *topic)
	c.Write <- msg

	// Consume n messages
	for i, r := range responseSequence {
		ctx, cancel := context.WithTimeout(signal, 10*time.Second)
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "timeout waiting for message\n")
			shutdown()
			wg.Wait()
			os.Exit(1)
		case <-clientCtx.Done():
			fmt.Fprintf(os.Stderr, "client connection closed\n")
			shutdown()
			wg.Wait()
			os.Exit(1)
		case msg := <-c.Read:
			fmt.Printf("%s\n", msg)
			parsed, err := messages.Parse(msg)
			if err != nil {
				fmt.Printf("error parsing message from server")
				os.Exit(1)
			}
			m := parsed.(messages.MsgMessage)

			// Determine which response to send
			var response messages.Message
			// Use the response from the sequence, cycling if necessary
			switch r {
			case messages.ACK:
				response = messages.NewAckMessage(m.Topic, m.CorrelationID)
			case messages.NCK:
				response = messages.NewNckMessage(m.Topic, m.CorrelationID)
			case messages.DLQ:
				response = messages.NewDlqMessage(m.Topic, m.CorrelationID)
			default:
				response = messages.NewAckMessage(m.Topic, m.CorrelationID)
			}
			c.Write <- response.Bytes()

			// Send RDY for next message if not the last one
			if i < len(responseSequence)-1 {
				rdy := fmt.Appendf(nil, "RDY|%s", *topic)
				c.Write <- rdy
			} else {
				// Give the server time to process the last ACK/NCK/DLQ before disconnecting
				time.Sleep(100 * time.Millisecond)
			}
			cancel()
		}
	}

	shutdown()
	wg.Wait()
}
