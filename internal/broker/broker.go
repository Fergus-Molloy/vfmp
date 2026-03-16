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
	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("queue not found")
)

var maxSendCount = 3

type Broker struct {
	mu      *sync.Mutex
	topics  map[string]*queue.Queue
	MsgChan chan model.Message
	leaseMu *sync.Mutex
	leases  map[string]lease
}

func (b *Broker) collectMetrics(ctx context.Context) {
	t := time.NewTicker(time.Second * 5)
	defer t.Stop()

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
		leaseMu: new(sync.Mutex),
		leases:  make(map[string]lease),
	}

	go b.collectMetrics(ctx)
	go b.awaitMessages(ctx, wg)

	return b
}

func (b *Broker) Dequeue(ctx context.Context, topic string) (model.Message, error) {
	q := b.getOrCreateTopic(ctx, topic)
	m, err := q.Dequeue(ctx)
	if err != nil {
		return m, err
	}

	b.newLease(10*time.Second, &m)
	return m, nil
}

func (b *Broker) DequeueN(ctx context.Context, topic string, n int) []model.Message {
	q := b.getOrCreateTopic(ctx, topic)
	msgs := q.DequeueN(n)
	toSend := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.SendCount >= maxSendCount {
			b.deadletter(m)
			continue
		}
		b.newLease(10*time.Second, &m)
		toSend = append(toSend, m)
	}

	return toSend
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

func (b *Broker) Ack(leaseID string) {
	b.leaseMu.Lock()
	defer b.leaseMu.Unlock()

	l, ok := b.leases[leaseID]
	if !ok {
		// lease resolved
		return
	}
	l.timer.Stop()

	delete(b.leases, leaseID)
	metrics.MsgAck.Inc()
}

func (b *Broker) Nack(leaseID string) {
	b.leaseMu.Lock()

	l, ok := b.leases[leaseID]
	if !ok {
		// lease resolved
		b.leaseMu.Unlock()
		return
	}
	l.timer.Stop()

	delete(b.leases, leaseID)
	b.leaseMu.Unlock()

	q := b.getOrCreateTopic(context.Background(), l.message.Topic)
	q.Prepend(*l.message)
	metrics.MsgNck.Inc()
}

func (b *Broker) deadletter(m model.Message) {
	q := b.getOrCreateTopic(context.Background(), "DEADLETTER/"+m.Topic)
	q.Append(m)
}

func (b *Broker) Dlq(leaseID string) {
	b.leaseMu.Lock()

	l, ok := b.leases[leaseID]
	if !ok {
		// lease resolved
		b.leaseMu.Unlock()
		return
	}

	delete(b.leases, leaseID)
	l.timer.Stop()
	b.leaseMu.Unlock()

	b.deadletter(*l.message)
	metrics.MsgDlq.Inc()
}

type lease struct {
	message *model.Message
	timer   *time.Timer
}

// newLease creates a new lease and updates the message with it's leaseID and send count
func (b *Broker) newLease(t time.Duration, msg *model.Message) {
	leaseID := uuid.New().String()
	msg.LeaseID = leaseID
	msg.SendCount += 1

	b.leaseMu.Lock()
	b.leases[leaseID] = lease{
		message: msg,
		timer:   time.AfterFunc(t, func() { b.Nack(leaseID) }),
	}
	b.leaseMu.Unlock()
}
