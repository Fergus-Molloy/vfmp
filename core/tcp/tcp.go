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
	"sync"
	"time"
)

var (
	ErrMalformedPayload = fmt.Errorf("payload was malformed")
)

type TCPClient struct {
	Read        <-chan []byte
	Write       chan<- []byte
	read, write chan []byte
	conn        net.Conn
}

// NewClient creates a tcp client that communicates via read and write channels, returns the client and a context that will be "Done" if the client exits
func NewClient(conn net.Conn, ctx context.Context, wg *sync.WaitGroup, logger *slog.Logger) (*TCPClient, context.Context) {
	ctx, cancel := context.WithCancel(ctx)

	logger = logger.With("client_addr", conn.RemoteAddr())

	read := make(chan []byte, 1)
	write := make(chan []byte, 1)

	client := &TCPClient{
		Read:  read,
		Write: write,
		read:  read,
		write: write,
		conn:  conn,
	}

	wg.Add(3)
	go client.startClientWriter(ctx, cancel, wg, logger)
	go client.startClientReader(ctx, cancel, wg, logger)
	go client.awaitShutdown(ctx, cancel, wg)

	return client, ctx
}

func (c *TCPClient) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *TCPClient) awaitShutdown(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done()

	<-ctx.Done()
	cancel()
	close(c.read)
	close(c.write)
}

func (c *TCPClient) startClientReader(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, logger *slog.Logger) {
	defer wg.Done()
	defer cancel()
	defer c.conn.Close()

	for {
		err := c.conn.SetReadDeadline(time.Now().Add(time.Second))
		if err != nil {
			logger.Error("error setting deadline for client", "err", err)
			return
		}
		size, err := readN(c.conn, 8)
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
		msg, err := readN(c.conn, int(messageSize))
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

		logger.Debug("read message from tcp client", "bytes", len(msg))

		select {
		case c.read <- msg:
		// do nothing
		case <-ctx.Done():
			return
		}
	}
}

func (c *TCPClient) startClientWriter(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, logger *slog.Logger) {
	defer wg.Done()
	defer cancel()
	defer c.conn.Close()

	for {
		select {
		case data := <-c.write:
			d := encodeData(data)
			_, err := c.conn.Write(d)
			if err != nil {
				logger.Error("failed to write to tcp client", "err", err, "data", d)
			}
		case <-ctx.Done():
			return
		}
	}
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

func encodeData(data []byte) []byte {
	d := make([]byte, len(data)+8)
	binary.BigEndian.PutUint64(d, uint64(len(data)))
	copy(d[8:], data)
	return d
}
