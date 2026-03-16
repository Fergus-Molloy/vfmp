package broker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"fergus.molloy.xyz/vfmp/internal/metrics"
	"fergus.molloy.xyz/vfmp/internal/model"
	"fergus.molloy.xyz/vfmp/internal/queue"
)

var (
	ErrNotFound = errors.New("queue not found")
)

type Broker struct {
	mu      *sync.Mutex
	topics  map[string]*queue.Queue
	MsgChan chan model.Message
}

func (b *Broker) collectMetrics(ctx context.Context) {
	t := time.NewTicker(time.Second * 5)
	for {
		select {
		case <-t.C:
			b.mu.Lock()
			for _, q := range b.topics {
				metrics.QueueLen.Observe(float64(q.Len()))
			}
			b.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func (b *Broker) awaitMessages(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case m := <-b.MsgChan:
			metrics.MsgIn.Inc()
			q := b.getOrCreateTopic(ctx, m.Topic)
			q.Append(m)
			slog.Info("new message added to queue", "topic", m.Topic, "correlationID", m.CorrelationID)
		case <-ctx.Done():
			slog.Info("broker finished")
			return
		}
	}
}

func (b *Broker) getOrCreateTopic(ctx context.Context, topic string) *queue.Queue {
	b.mu.Lock()
	defer b.mu.Unlock()

	q, ok := b.topics[topic]
	if !ok {
		slog.Info("creating new queue", "topic", topic)
		metrics.TopicCount.Inc()
		q = queue.New(ctx, topic)
		b.topics[topic] = q
	}
	return q
}

func StartBroker(ctx context.Context, wg *sync.WaitGroup) *Broker {
	b := &Broker{
		mu:      new(sync.Mutex),
		topics:  make(map[string]*queue.Queue),
		MsgChan: make(chan model.Message, 1),
	}

	go b.collectMetrics(ctx)
	go b.awaitMessages(ctx, wg)

	return b
}

func (b *Broker) Dequeue(ctx context.Context, topic string) (model.Message, error) {
	q := b.getOrCreateTopic(ctx, topic)
	return q.Dequeue(ctx)
}

func (b *Broker) DequeueN(ctx context.Context, topic string, n int) []model.Message {
	q := b.getOrCreateTopic(ctx, topic)
	return q.DequeueN(n)
}

func (b *Broker) GetCount(topic string) int {
	q, ok := b.topics[topic]
	if !ok {
		return 0
	}

	return q.Len()
}

func (b *Broker) Peek(topic string) (model.Message, error) {
	q, ok := b.topics[topic]
	if !ok {
		return model.Message{}, ErrNotFound
	}

	return q.Peek()
}
