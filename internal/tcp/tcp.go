package tcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"fergus.molloy.xyz/vfmp/core/tcp"
	"fergus.molloy.xyz/vfmp/internal/broker"
	"fergus.molloy.xyz/vfmp/internal/config"
)

var (
	ErrMalformedPayload = errors.New("payload is malformed")
)

type MessageType string

const (
	RDY MessageType = "RDY"
	ACK MessageType = "ACK"
	NCK MessageType = "NCK"
	DLT MessageType = "DLQ"
)

func StartTCPServer(broker *broker.Broker, ctx context.Context, wg *sync.WaitGroup, config *config.Config) *net.Listener {
	srv := net.ListenConfig{}

	logger := slog.With("addr", config.TCPAddr)

	logger.Info("starting tcp server")
	listener, err := srv.Listen(ctx, "tcp", config.TCPAddr)
	if err != nil {
		logger.Error("unable to start tcp server", "err", err)
		wg.Done()
		return nil
	}

	go func() {
		defer wg.Done()

		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				logger.Error("error accepting new tcp client", "err", err)
			}
			wg.Add(1)
			c := tcp.NewClient(conn, ctx, wg, logger)

			for {
				select {
				case msg := <-c.Read:
					handleMessage(c, broker, logger, msg, ctx)
				case <-ctx.Done():
					logger.Warn("closing down client")
					return
				}
			}
		}
	}()

	return &listener
}

func handleMessage(c *tcp.TCPClient, broker *broker.Broker, logger *slog.Logger, msg []byte, ctx context.Context) {
	if len(msg) < 3 {
		logger.Error("received message is malformed", "err", ErrMalformedPayload)
		return
	}

	msgType := MessageType(msg[:3])
	switch msgType {
	case RDY:
		topic := strings.TrimSpace(string(msg[3:]))
		logger.Info("client ready for new message", "topic", topic)

		topicChan := broker.NotifyReady(ctx, topic)
		select {
		case m := <-topicChan:
			slog.Info("received message from queue", "topic", topic, "correlationID", m.CorrelationID)
			c.Write <- fmt.Appendf(nil, "MSG|%s|%s", m.CorrelationID, m.Data)
		case <-ctx.Done():
			return
		}

	case ACK, NCK, DLT:
		return
	default:
		logger.Error("unkown message type", "msgType", msgType, "err", ErrMalformedPayload)
		return
	}
}
