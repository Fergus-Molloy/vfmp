package broker

import (
	"context"
	"log/slog"
	"sync"

	"fergus.molloy.xyz/vfmp/internal/model"
)

type Broker struct {
	mu      *sync.Mutex
	topics  map[string]*Queue
	MsgChan chan model.Message
}

func (b *Broker) awaitMessages(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case m := <-b.MsgChan:
			q := b.getOrCreateTopic(ctx, m.Topic)
			q.Append(m)
		case <-ctx.Done():
			slog.Info("broker finished")
			return
		}
	}
}

func (b *Broker) getOrCreateTopic(ctx context.Context, topic string) *Queue {
	b.mu.Lock()
	defer b.mu.Unlock()

	q, ok := b.topics[topic]
	if !ok {
		slog.Info("creating new queue", "topic", topic)
		q = NewQueue(ctx)
		b.topics[topic] = q
	}
	return q
}

func StartBroker(ctx context.Context, wg *sync.WaitGroup) *Broker {
	b := &Broker{
		mu:      new(sync.Mutex),
		topics:  make(map[string]*Queue),
		MsgChan: make(chan model.Message, 1),
	}

	go b.awaitMessages(ctx, wg)

	return b
}

func (b *Broker) NotifyReady(ctx context.Context, topic string) chan model.Message {
	q := b.getOrCreateTopic(ctx, topic)
	return q.MsgChan
}
