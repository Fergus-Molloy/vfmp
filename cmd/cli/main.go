package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"fergus.molloy.xyz/vfmp/core/tcp"
)

func main() {
	args := os.Args[1:]
	if len(args) != 2 {
		fmt.Printf("Usage: cli <addr> <topic>\n")
		os.Exit(1)
	}

	addr := args[0]
	topic := args[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
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
	c, clientCtx := tcp.NewClient(conn, ctx, wg, logger)

	// Subscribe to the topic
	msg := fmt.Appendf(nil, "RDY|%s", topic)
	c.Write <- msg

	// Wait for exactly one message
	select {
	case <-ctx.Done():
		fmt.Fprintf(os.Stderr, "timeout waiting for message\n")
		cancel()
		wg.Wait()
		os.Exit(1)
	case <-clientCtx.Done():
		fmt.Fprintf(os.Stderr, "client connection closed\n")
		cancel()
		wg.Wait()
		os.Exit(1)
	case msg := <-c.Read:
		fmt.Printf("%s\n", msg)
		cancel()
		wg.Wait()
		os.Exit(0)
	}
}
