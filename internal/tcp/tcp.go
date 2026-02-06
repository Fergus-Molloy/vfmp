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
	"fergus.molloy.xyz/vfmp/internal/model"
	"github.com/google/uuid"
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

// tcpClient wraps a [tcp.TCPClient] with an async-safe map of messages being handled by this client
type tcpClient struct {
	inner    *tcp.TCPClient
	mu       *sync.Mutex
	messages map[string]model.Message
}

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
		defer listener.Close()

		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				logger.Error("error accepting new tcp client", "err", err)
			}
			client, clientCtx := tcp.NewClient(conn, ctx, wg, logger)
			c := newTCPClient(client)

			wg.Add(1)
			go handleClient(c, broker, ctx, clientCtx, wg, logger)
		}
	}()

	return &listener
}

func handleClient(c *tcpClient, broker *broker.Broker, ctx context.Context, clientCtx context.Context, wg *sync.WaitGroup, logger *slog.Logger) {
	defer wg.Done()
	for {
		select {
		case msg := <-c.inner.Read:
			c.handleMessage(broker, logger, msg, clientCtx)
		case <-clientCtx.Done():
			logger.Warn("client disconnected", "client_addr", c.inner.RemoteAddr())
			return
		case <-ctx.Done():
			return
		}
	}
}

func newTCPClient(c *tcp.TCPClient) *tcpClient {
	return &tcpClient{
		inner:    c,
		mu:       new(sync.Mutex),
		messages: make(map[string]model.Message),
	}
}

func (c *tcpClient) handleMessage(broker *broker.Broker, logger *slog.Logger, msg []byte, ctx context.Context) {
	if len(msg) < 3 {
		logger.Error("received message is malformed", "err", ErrMalformedPayload)
		return
	}

	msgType := MessageType(msg[:3])
	l := logger.With("msgType", msgType)
	switch msgType {
	case RDY:
		topic := strings.TrimSpace(string(msg[4:]))
		l.Info("client ready for new message", "topic", topic)

		topicChan := broker.NotifyReady(ctx, topic)
		select {
		case m := <-topicChan:
			c.mu.Lock()
			c.messages[m.CorrelationID] = m

			l.Info("received message from queue", "topic", topic, "correlationID", m.CorrelationID)

			select {
			case c.inner.Write <- fmt.Appendf(nil, "MSG|%s|%s", m.CorrelationID, m.Data):
				c.mu.Unlock()
			case <-ctx.Done():
				c.mu.Unlock()
				return
			}
		case <-ctx.Done():
			return
		}

	case ACK:
		stringID := strings.TrimSpace(string(msg[4:]))
		_, err := uuid.Parse(stringID)
		if err != nil {
			l.Error("could not parse correlation id", "err", err, "data", stringID)
			return
		}

		c.mu.Lock()
		delete(c.messages, stringID)
		l.Debug("acked message", "correlationID", stringID)
		c.mu.Unlock()

	case NCK:
		stringID := strings.TrimSpace(string(msg[4:]))
		_, err := uuid.Parse(stringID)
		if err != nil {
			l.Error("could not parse correlation id", "err", err, "data", stringID)
			return
		}
		l.Info("message was nacked", "correlationID", stringID)
		c.mu.Lock()
		m, ok := c.messages[stringID]
		if !ok {
			l.Error("could not find message in map", "correlationID", stringID)
			return
		}
		delete(c.messages, stringID)
		// return message to end of queue
		select {
		case broker.MsgChan <- m:
			c.mu.Unlock()
		case <-ctx.Done():
			c.mu.Unlock()
			return
		}

	case DLT:
		stringID := strings.TrimSpace(string(msg[4:]))
		_, err := uuid.Parse(stringID)
		if err != nil {
			l.Error("could not parse correlation id", "err", err, "data", stringID)
			return
		}
		l.Warn("dead lettering message", "correlationID", stringID)

		// for now just delete the message
		c.mu.Lock()
		delete(c.messages, stringID)
		c.mu.Unlock()
	default:
		l.Error("unknown message type", "err", ErrMalformedPayload)
		return
	}
}
