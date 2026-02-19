package tcp

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"fergus.molloy.xyz/vfmp/core/messages"
	"fergus.molloy.xyz/vfmp/core/tcp"
	"fergus.molloy.xyz/vfmp/internal/broker"
	"fergus.molloy.xyz/vfmp/internal/config"
	"fergus.molloy.xyz/vfmp/internal/model"
	"github.com/google/uuid"
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
	messages map[uuid.UUID]model.Message
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

			// clean-up on client disconnect
			context.AfterFunc(clientCtx, func() {
				logger.Warn("client disconnected", "client_addr", client.RemoteAddr())
				for _, m := range c.messages {
					c.nack(ctx, logger, broker, m.CorrelationID)
				}
			})

			wg.Add(1)
			go handleClient(c, broker, ctx, clientCtx, wg, logger.With("client_addr", client.RemoteAddr()))
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
		messages: make(map[uuid.UUID]model.Message),
	}
}

func (c *tcpClient) handleMessage(broker *broker.Broker, logger *slog.Logger, bytes []byte, ctx context.Context) {
	m, err := messages.Parse(bytes)
	if err != nil {
		logger.Error("could not parse message", "err", err, "raw", string(bytes))
		return
	}

	switch msg := m.(type) {
	case messages.RdyMessage:
		l := logger.With("msgType", msg.MsgType).With("topic", msg.Topic)

		topicChan := broker.NotifyReady(ctx, msg.Topic)
		select {
		case m := <-topicChan:
			c.mu.Lock()
			c.messages[m.CorrelationID] = m
			go c.awaitAck(ctx, logger, broker, m.CorrelationID)

			l.Info("received message from queue", "correlationID", m.CorrelationID)

			select {
			case c.inner.Write <- m.ToMsgMessage().Bytes():
			case <-ctx.Done():
			}
			c.mu.Unlock()
		case <-ctx.Done():
			return
		}

	case messages.AckMessage:
		l := logger.With("msgType", msg.MsgType).With("topic", msg.Topic)
		c.ack(l, msg.CorrelationID)

	case messages.NckMessage:
		l := logger.With("msgType", msg.MsgType).With("topic", msg.Topic)
		c.nack(ctx, l, broker, msg.CorrelationID)

	case messages.DlqMessage:
		l := logger.With("msgType", msg.MsgType).With("topic", msg.Topic)
		l.Warn("dead lettering message", "correlationID", msg.CorrelationID)

		// for now just delete the message
		c.mu.Lock()
		delete(c.messages, msg.CorrelationID)
		c.mu.Unlock()
	default:
		logger.Error("unknown message type")
		return
	}
}

func (c *tcpClient) nack(ctx context.Context, l *slog.Logger, broker *broker.Broker, correlationID uuid.UUID) {
	l.Info("message was nacked", "correlationID", correlationID)
	c.mu.Lock()
	m, ok := c.messages[correlationID]
	if !ok {
		l.Error("could not find message in map", "correlationID", correlationID)
		return
	}
	delete(c.messages, correlationID)
	// return message to end of queue
	select {
	case broker.MsgChan <- m:
		c.mu.Unlock()
	case <-ctx.Done():
		c.mu.Unlock()
		return
	}
}

func (c *tcpClient) ack(l *slog.Logger, correlationID uuid.UUID) {
	c.mu.Lock()
	delete(c.messages, correlationID)
	l.Debug("acked message", "correlationID", correlationID)
	c.mu.Unlock()
}

func (c *tcpClient) awaitAck(ctx context.Context, l *slog.Logger, broker *broker.Broker, id uuid.UUID) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Second * 10):
		l.Debug("timed out waiting for ack")
		c.mu.Lock()
		defer c.mu.Unlock()

		_, ok := c.messages[id]
		if ok {
			// nack
			c.nack(ctx, l, broker, id)
		}
	}
}
