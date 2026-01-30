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
}

func NewClient(conn net.Conn, ctx context.Context, wg *sync.WaitGroup, logger *slog.Logger) *TCPClient {
	defer wg.Done()

	logger = logger.With("client_addr", conn.RemoteAddr())

	write := make(chan []byte, 1)

	go startClientWriter(conn, write, ctx, logger)
	read := make(chan []byte, 1)
	go startClientReader(conn, read, ctx, logger)

	go awaitShutdown(ctx, read, write)

	return &TCPClient{
		Read:  read,
		Write: write,
		read:  read,
		write: write,
	}

}

func awaitShutdown(ctx context.Context, read chan []byte, write chan []byte) {
	<-ctx.Done()
	close(read)
	close(write)
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

		select {
		case read <- msg:
		// do nothing
		case <-ctx.Done():
			return
		}
	}
}

func startClientWriter(client net.Conn, write chan []byte, ctx context.Context, logger *slog.Logger) {
	defer client.Close()

	for {
		select {
		case data := <-write:
			d := encodeData(data)
			_, err := client.Write(d)
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
