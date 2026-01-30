package tcp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"fergus.molloy.xyz/vfmp/internal/broker"
	"fergus.molloy.xyz/vfmp/internal/config"
)

var (
	ErrMalformedPayload = fmt.Errorf("payload was malformed")
)

type MessageType string

const (
	Rdy MessageType = "RDY"
	Ack MessageType = "ACK"
	Nck MessageType = "NCK"
	DLT MessageType = "DLT"
)

type tcpClient struct {
	read, write chan []byte
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

		for {
			client, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				logger.Error("error accepting new tcp client", "err", err)
			}
			wg.Add(1)
			c := startClient(client, ctx, wg, logger)

			for {
				select {
				case msg := <-c.read:
					c.handleMessage(broker, logger, msg, ctx)
				case <-ctx.Done():
					logger.Warn("closing down client")
					close(c.read)
					close(c.write)
					return
				}
			}
		}
	}()

	return &listener
}

func startClient(client net.Conn, ctx context.Context, wg *sync.WaitGroup, logger *slog.Logger) *tcpClient {
	defer wg.Done()

	logger = logger.With("client_addr", client.RemoteAddr())

	write := make(chan []byte, 1)

	go startClientWriter(client, write, ctx, logger)
	read := make(chan []byte, 1)
	go startClientReader(client, read, ctx, logger)

	return &tcpClient{
		read:  read,
		write: write,
	}

}
func startClientReader(client net.Conn, read chan []byte, ctx context.Context, logger *slog.Logger) {
	defer client.Close()

	for {
		err := client.SetReadDeadline(time.Now().Add(time.Second))
		if err != nil {
			logger.Error("error setting deadline for client", "err", err)
			return
		}
		size, err := readN(client, 8)
		if err != nil {
			switch {
			case os.IsTimeout(err) && contextIsDone(ctx), err == io.EOF, errors.Is(err, net.ErrClosed):
				logger.Warn("closing client", "err", err)
				return
			case os.IsTimeout(err):
				continue
			default:
				logger.Error("error reading from tcp client", "err", err)
			}
		}
		messageSize := binary.BigEndian.Uint64(size)
		msg, err := readN(client, int(messageSize))
		if err != nil {
			switch {
			case os.IsTimeout(err) && contextIsDone(ctx), err == io.EOF, errors.Is(err, net.ErrClosed):
				logger.Warn("closing client", "err", err)
				return
			case os.IsTimeout(err):
				continue
			default:
				logger.Error("error reading from tcp client", "err", err)
			}
		}

		logger.Info("read message from tcp client", "bytes", len(msg))

		read <- msg
	}
}

func (c *tcpClient) handleMessage(broker *broker.Broker, logger *slog.Logger, msg []byte, ctx context.Context) {
	if len(msg) < 3 {
		logger.Error("received message is malformed", "err", ErrMalformedPayload)
		return
	}

	msgType := MessageType(msg[:3])
	switch msgType {
	case Rdy:
		topic := strings.TrimSpace(string(msg[3:]))
		logger.Info("client ready for new message", "topic", topic)

		topicChan := broker.NotifyReady(ctx, topic)
		select {
		case m := <-topicChan:
			slog.Info("received message from queue", "topic", topic, "correlationID", m.CorrelationID)
			c.write <- fmt.Appendf(nil, "MSG|%s|%s", m.CorrelationID, m.Data)
		case <-ctx.Done():
			return
		}

	case Ack, Nck, DLT:
		return
	default:
		logger.Error("unkown message type", "msgType", msgType, "err", ErrMalformedPayload)
		return
	}
}

func startClientWriter(client net.Conn, write chan []byte, ctx context.Context, logger *slog.Logger) {
	defer client.Close()

	for {
		select {
		case data := <-write:
			d := prependSize(data)
			_, err := client.Write(d)
			if err != nil {
				logger.Error("failed to write to tcp client", "err", err, "data", d)
			}
		case <-ctx.Done():
			return
		}
	}
}

func prependSize(data []byte) []byte {
	d := make([]byte, len(data)+8)

	binary.BigEndian.PutUint64(d, uint64(len(data)))

	copy(d[8:], data)
	return d
}

func readN(client net.Conn, n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadAtLeast(client, buf, n)
	if err != nil {
		return nil, err
	}

	if len(buf) == 0 {
		return nil, io.EOF
	}

	return buf, nil
}

func contextIsDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
