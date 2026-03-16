package grpc

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"

	vfmpv1 "fergus.molloy.xyz/vfmp/gen/vfmp/v1"
	"fergus.molloy.xyz/vfmp/internal/broker"
	"fergus.molloy.xyz/vfmp/internal/config"
	"fergus.molloy.xyz/vfmp/internal/model"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

func StartGRPCServer(broker *broker.Broker, ctx context.Context, wg *sync.WaitGroup, config *config.Config) *grpc.Server {
	logger := slog.With("addr", config.TCPAddr)
	logger.Info("starting tcp server")

	listener, err := net.Listen("tcp", config.TCPAddr)
	if err != nil {
		logger.Error("error starting listener for grpc server", "err", err)
		return nil
	}

	srv := grpc.NewServer()
	vfmpv1.RegisterMessageServiceServer(srv, newServer(broker))

	go func() {
		defer wg.Done()

		err := srv.Serve(listener)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			logger.Error("error serving grpc server", "err", err)
		}
	}()

	return srv
}

// server implements \[vfmpv1.Server\]
type server struct {
	vfmpv1.UnimplementedMessageServiceServer

	broker   *broker.Broker
	sendMu   *sync.Mutex
	mu       *sync.Mutex
	messages map[uuid.UUID]model.Message
}

func newServer(b *broker.Broker) *server {
	return &server{
		broker:   b,
		sendMu:   new(sync.Mutex),
		mu:       new(sync.Mutex),
		messages: make(map[uuid.UUID]model.Message),
	}
}

func (s *server) Consume(r *vfmpv1.ConsumeRequest, stream grpc.ServerStreamingServer[vfmpv1.Message]) error {
	logger := slog.With("topic", r.Topic)

	msgs := s.broker.DequeueN(context.Background(), r.Topic, int(r.MaxMessages))
	var c int
	for _, msg := range msgs {
		c += 1
		s.sendMu.Lock()
		err := stream.Send(&vfmpv1.Message{
			Topic:         r.Topic,
			DeliveryCount: 0, //TODO
			CorrelationId: msg.CorrelationID.String(),
			LeaseToken:    "token", //TODO
			Body:          msg.Data,
		})
		s.sendMu.Unlock()
		if err != nil {
			logger.Error("error sending message to client", "count", c, "err", err)
			return err
		}
	}

	logger.Info("sent messages to client", "count", c)
	return nil
}

// Ack confirms successful processing. The message is permanently removed
// from the queue.
func (s *server) Ack(context.Context, *vfmpv1.AckRequest) (*vfmpv1.AckResponse, error) {
	panic("todo")
}

// Nck signals failed processing. The message is immediately requeued and
// becomes available for redelivery to any consumer.
func (s *server) Nck(context.Context, *vfmpv1.NckRequest) (*vfmpv1.NckResponse, error) {
	panic("todo")
}

// Dlq dead-letters the message, permanently discarding it without requeue.
func (s *server) Dlq(context.Context, *vfmpv1.DlqRequest) (*vfmpv1.DlqResponse, error) {
	panic("todo")
}
