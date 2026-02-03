package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"

	"fergus.molloy.xyz/vfmp/core/tcp"
)

func main() {
	args := os.Args[1:]
	if len(args) != 2 {
		fmt.Printf("Usage: client <addr> <topic>\n")
		os.Exit(1)
	}

	addr := args[0]
	topic := args[1]

	fmt.Printf("connecting to server %s for topic %s\n", addr, topic)
	signal, exitFunc := signal.NotifyContext(context.Background(), os.Interrupt)
	defer exitFunc()

	input := make(chan string, 1)

	go getUserInput(input, exitFunc)

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		fmt.Printf("could not resolve address: %v\n", err)
		return
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		fmt.Printf("could not dial address: %v\n", err)
		return
	}

	wg := new(sync.WaitGroup)
	wg.Add(1)
	c, clientCtx := tcp.NewClient(conn, signal, wg, slog.Default())

	for {
		select {
		case <-signal.Done():
			wg.Wait()
			return
		case <-clientCtx.Done():
			exitFunc()
			wg.Wait()
			return
		case i := <-input:
			c.Write <- []byte(i)
		case msg := <-c.Read:
			fmt.Printf("> %s\n", msg)
		}
	}
}

func getUserInput(input chan string, exitFunc context.CancelFunc) {
	reader := bufio.NewReader(os.Stdin)
	for {
		text, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("error reading from stdin: %v\n", err)
			continue
		}
		text = strings.TrimSpace(text)
		if text == "exit" {
			exitFunc()
			return
		}
		input <- text
	}
}
