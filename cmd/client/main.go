package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"time"

	vfmpv1 "fergus.molloy.xyz/vfmp/gen/vfmp/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	maxMessages     = 10
	pollInterval    = 5 * time.Second
	backoffInterval = 10 * time.Second
)

func main() {
	addr := flag.String("address", "localhost:50051", "gRPC server address")
	topic := flag.String("topic", "test", "Topic to consume messages from")
	flag.Parse()

	slog.Info("connecting to server", "addr", *addr, "topic", *topic)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("failed to connect", "addr", *addr, "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := vfmpv1.NewMessageServiceClient(conn)

	consume(ctx, client, *topic)
}

func consume(ctx context.Context, client vfmpv1.MessageServiceClient, topic string) {
	for {
		if ctx.Err() != nil {
			return
		}

		received, err := consumeBatch(ctx, client, topic)
		if err != nil {
			slog.Error("consume error", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoffInterval):
				continue
			}
		}

		slog.Info("received batch of messages", "count", received)
		// If the queue was empty, wait before polling again.
		if received < maxMessages {
			select {
			case <-ctx.Done():
				return
			case <-time.After(pollInterval):
			}
		}
	}
}

// consumeBatch opens a single Consume stream, drains it, and ACKs each message.
// Returns the number of messages received.
func consumeBatch(ctx context.Context, client vfmpv1.MessageServiceClient, topic string) (int, error) {
	stream, err := client.Consume(ctx, &vfmpv1.ConsumeRequest{
		Topic:       topic,
		MaxMessages: maxMessages,
	})
	if err != nil {
		return 0, fmt.Errorf("opening stream: %w", err)
	}

	received := 0
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return received, nil
		}
		if err != nil {
			if status.Code(err) == codes.Canceled {
				return received, nil
			}
			return received, fmt.Errorf("receiving message: %w", err)
		}

		received++
		slog.Info("received message",
			"topic", msg.Topic,
			"correlation_id", msg.CorrelationId,
			"lease_token", msg.LeaseToken,
			"delivery_count", msg.DeliveryCount,
			"body", string(msg.Body),
		)

		if err := ack(ctx, client, msg); err != nil {
			slog.Error("failed to ack message", "lease_token", msg.LeaseToken, "err", err)
		}
	}
}

func ack(ctx context.Context, client vfmpv1.MessageServiceClient, msg *vfmpv1.Message) error {
	_, err := client.Ack(ctx, &vfmpv1.AckRequest{
		Topic:      msg.Topic,
		LeaseToken: msg.LeaseToken,
	})
	return err
}
