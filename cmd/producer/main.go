package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	serverAddr = flag.String("addr", "http://localhost:8080", "VFMP server address")
	numTopics  = flag.Int("topics", 1, "Number of topics to populate")
	numMsgs    = flag.Int("messages", 0, "Total number of messages to generate (0 for infinite)")
	rate       = flag.Int("rate", 0, "Messages per second to generate (0 for unlimited, implies infinite)")
	msgSize    = flag.Int("size", 1024, "Size of each message in bytes")
)

// Human-readable topic name components
var (
	adjectives = []string{
		"small",
		// "fast", "slow", "bright", "dark", "happy", "sad", "large", "small",
		// "hot", "cold", "new", "old", "good", "bad", "high", "low",
		// "quick", "loud", "quiet", "clean", "dirty", "empty", "full", "heavy",
	}
	nouns = []string{
		"buffer",
		// "queue", "stream", "pipe", "channel", "buffer", "cache", "store", "pool",
		// "stack", "heap", "tree", "graph", "list", "array", "table", "index",
		// "bucket", "shard", "partition", "segment", "block", "page", "frame", "slot",
	}
)

type Stats struct {
	sent      atomic.Uint64
	errors    atomic.Uint64
	startTime time.Time
}

func (s *Stats) report() {
	sent := s.sent.Load()
	errors := s.errors.Load()
	elapsed := time.Since(s.startTime)
	rate := float64(sent) / elapsed.Seconds()

	slog.Info("producer stats",
		"sent", sent,
		"errors", errors,
		"elapsed", elapsed.Round(time.Millisecond),
		"rate", fmt.Sprintf("%.2f msg/s", rate),
	)
}

func generateTopicName() string {
	adj := adjectives[rand.IntN(len(adjectives))]
	noun := nouns[rand.IntN(len(nouns))]
	return fmt.Sprintf("%s-%s", adj, noun)
}

func generateMessage(size int) []byte {
	// Generate random ASCII printable characters (32-126)
	msg := make([]byte, size)
	for i := range size {
		msg[i] = byte(32 + rand.IntN(95)) // ASCII printable range
	}
	return msg
}

func publishMessage(ctx context.Context, client *http.Client, serverAddr, topic string, data []byte) error {
	url := fmt.Sprintf("%s/messages/%s", serverAddr, topic)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func produceWorker(ctx context.Context, wg *sync.WaitGroup, client *http.Client, serverAddr string, topics []string, msgSize int, stats *Stats, rateLimiter <-chan time.Time) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rateLimiter:
			// Rate limiter allows us to proceed
		}

		// Pick a random topic
		topic := topics[rand.IntN(len(topics))]

		// Generate message
		msg := generateMessage(msgSize)

		// Publish message
		if err := publishMessage(ctx, client, serverAddr, topic, msg); err != nil {
			slog.Error("failed to publish message", "topic", topic, "err", err)
			stats.errors.Add(1)
		} else {
			stats.sent.Add(1)
		}
	}
}

func main() {
	flag.Parse()

	// Setup logging
	slog.SetLogLoggerLevel(slog.LevelInfo)

	// Validate flags
	if *numTopics < 1 {
		slog.Error("number of topics must be at least 1")
		os.Exit(1)
	}

	if *numMsgs < 0 {
		slog.Error("number of messages cannot be negative")
		os.Exit(1)
	}

	if *rate < 0 {
		slog.Error("rate cannot be negative")
		os.Exit(1)
	}

	if *rate == 0 && *numMsgs == 0 {
		slog.Error("must set either rate or number of messages")
		os.Exit(1)
	}

	if *msgSize < 1 {
		slog.Error("message size must be at least 1 byte")
		os.Exit(1)
	}

	// Generate topic names
	topics := make([]string, *numTopics)
	for i := range *numTopics {
		topics[i] = generateTopicName()
	}

	slog.Info("starting producer",
		"server", *serverAddr,
		"topics", topics,
		"messages", *numMsgs,
		"rate", *rate,
		"msg_size", *msgSize,
	)

	// Setup signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Setup HTTP client
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Setup stats
	stats := &Stats{
		startTime: time.Now(),
	}

	// Setup rate limiter
	var rateLimiter <-chan time.Time
	if *rate > 0 {
		ticker := time.NewTicker(time.Second / time.Duration(*rate))
		defer ticker.Stop()
		rateLimiter = ticker.C
	} else {
		// Unlimited rate - create a channel that's always ready
		unlimitedChan := make(chan time.Time)
		close(unlimitedChan)
		rateLimiter = unlimitedChan
	}

	// Start stats reporter
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats.report()
			}
		}
	}()

	wg := new(sync.WaitGroup)

	if *rate > 0 {
		// Continuous production mode
		slog.Info("starting continuous production (press Ctrl+C to stop)")

		wg.Add(1)
		go produceWorker(ctx, wg, client, *serverAddr, topics, *msgSize, stats, rateLimiter)
	} else {
		// Finite message mode - calculate workers needed
		numWorkers := min(10, *numMsgs) // Max 10 workers
		messagesPerWorker := *numMsgs / numWorkers
		remainder := *numMsgs % numWorkers

		slog.Info("starting finite production", "workers", numWorkers, "total_messages", *numMsgs)

		for i := range numWorkers {
			workerMessages := messagesPerWorker
			if i < remainder {
				workerMessages++
			}

			wg.Add(1)
			go func(count int) {
				defer wg.Done()

				for range count {
					select {
					case <-ctx.Done():
						return
					case <-rateLimiter:
						// Rate limiter allows us to proceed
					}

					topic := topics[rand.IntN(len(topics))]
					msg := generateMessage(*msgSize)

					if err := publishMessage(ctx, client, *serverAddr, topic, msg); err != nil {
						slog.Error("failed to publish message", "topic", topic, "err", err)
						stats.errors.Add(1)
					} else {
						stats.sent.Add(1)
					}
				}
			}(workerMessages)
		}
	}

	// Wait for shutdown signal or completion
	if *rate > 0 {
		<-ctx.Done()
		slog.Warn("shutdown signal received, stopping producer")
	} else {
		// Wait for all workers to complete
		doneChan := make(chan struct{})
		go func() {
			wg.Wait()
			close(doneChan)
		}()

		select {
		case <-ctx.Done():
			slog.Warn("shutdown signal received, stopping producer")
		case <-doneChan:
			slog.Info("all messages sent, shutting down")
		}
	}

	// Cancel context to stop all workers
	cancel()

	// Wait for all goroutines to finish
	wg.Wait()

	// Final stats report
	stats.report()
	slog.Info("producer stopped cleanly")
}
