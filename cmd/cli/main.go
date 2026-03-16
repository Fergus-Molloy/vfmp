package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	vfmpv1 "fergus.molloy.xyz/vfmp/gen/vfmp/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := flag.String("address", "localhost:50051", "gRPC server address")
	topic := flag.String("topic", "test", "Topic to consume messages from")
	n := flag.Int("n", 1, "Number of messages to consume")
	sequence := flag.String("sequence", "", "Comma-separated ACK/NCK/DLQ responses, one per message. Overrides -n")
	flag.Parse()

	responseSequence := parseSequence(*sequence, *n)

	ctx, shutdown := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer shutdown()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := vfmpv1.NewMessageServiceClient(conn)

	stream, err := client.Consume(ctx, &vfmpv1.ConsumeRequest{
		Topic:       *topic,
		MaxMessages: int32(len(responseSequence)),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start consume stream: %v\n", err)
		os.Exit(1)
	}

	for i, resp := range responseSequence {
		msg, err := stream.Recv()
		if err == io.EOF {
			fmt.Fprintf(os.Stderr, "stream ended after %d/%d messages\n", i, len(responseSequence))
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error receiving message: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("topic=%s correlation_id=%s delivery_count=%d body=%s\n",
			msg.Topic, msg.CorrelationId, msg.DeliveryCount, msg.Body)

		if err := respond(ctx, client, msg, resp); err != nil {
			fmt.Fprintf(os.Stderr, "failed to send %s: %v\n", resp, err)
			os.Exit(1)
		}
	}
}

func respond(ctx context.Context, client vfmpv1.MessageServiceClient, msg *vfmpv1.Message, resp string) error {
	switch resp {
	case "ACK":
		_, err := client.Ack(ctx, &vfmpv1.AckRequest{Topic: msg.Topic, LeaseToken: msg.LeaseToken})
		return err
	case "NCK":
		_, err := client.Nck(ctx, &vfmpv1.NckRequest{Topic: msg.Topic, LeaseToken: msg.LeaseToken})
		return err
	case "DLQ":
		_, err := client.Dlq(ctx, &vfmpv1.DlqRequest{Topic: msg.Topic, LeaseToken: msg.LeaseToken})
		return err
	default:
		return fmt.Errorf("unknown response type: %s", resp)
	}
}

func parseSequence(sequence string, n int) []string {
	if sequence != "" {
		var result []string
		for _, part := range strings.Split(sequence, ",") {
			resp := strings.ToUpper(strings.TrimSpace(part))
			switch resp {
			case "ACK", "NCK", "DLQ":
				result = append(result, resp)
			default:
				fmt.Fprintf(os.Stderr, "invalid response type %q — valid types: ACK, NCK, DLQ\n", part)
				os.Exit(1)
			}
		}
		return result
	}

	result := make([]string, n)
	for i := range result {
		result[i] = "ACK"
	}
	return result
}
