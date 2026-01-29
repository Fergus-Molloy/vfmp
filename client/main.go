package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
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
	read := make(chan string, 1)
	write := make(chan string, 1)
	go dialServer(addr, read, write, signal)

	for {
		select {
		case <-signal.Done():
			return
		case i := <-input:
			write <- i
		case msg := <-read:
			fmt.Printf("> %s\n", msg)
		}
	}
}

func dialServer(addr string, read, write chan string, signal context.Context) {
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

	go startWriter(conn, write, signal)

	for {
		size, err := readN(conn, 8)
		if err != nil {
			if err == io.EOF {
				fmt.Printf("server closed connection\n")
				os.Exit(1)
			}

			fmt.Printf("error reading from tcp client: %v\n", err)
			continue
		}

		messageSize := binary.BigEndian.Uint64(size)
		msg, err := readN(conn, int(messageSize))
		if err != nil {
			if err == io.EOF {
				fmt.Printf("server closed connection\n")
				os.Exit(1)
			}

			fmt.Printf("error reading from tcp client: %v\n", err)
			continue
		}

		read <- string(msg)
	}
}

func startWriter(conn *net.TCPConn, write chan string, signal context.Context) {
	for {
		select {
		case <-signal.Done():
			return
		case msg := <-write:
			size := len(msg)
			data := make([]byte, 0, size+8)
			data = binary.BigEndian.AppendUint64(data, uint64(size))
			data = append(data, []byte(msg)...)
			_, err := conn.Write(data)
			if err != nil {
				fmt.Printf("failed to write data to tcp connection: %v\n", err)
			}
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
func readN(client net.Conn, n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadAtLeast(client, buf, n)
	if err != nil {
		return nil, err
	}
	return buf, nil
}
