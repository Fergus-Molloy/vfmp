package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"fergus.molloy.xyz/vfmp/internal/model"
	"fergus.molloy.xyz/vfmp/internal/model/list"
)

var (
	ErrEmpty = errors.New("list has no elements")
)

type Queue struct {
	mu     *sync.Mutex
	list   *list.List[model.Message]
	topic  string
	notify chan struct{}
	len    atomic.Int64
}

// New returns a new Queue
func New(ctx context.Context, topic string) *Queue {
	return &Queue{
		mu:     new(sync.Mutex),
		list:   list.New[model.Message](),
		topic:  topic,
		notify: make(chan struct{}, 1),
	}
}

// Append adds message to back of queue.
func (q *Queue) Append(msg model.Message) {
	q.mu.Lock()
	q.list.PushBack(msg)
	q.len.Add(1)
	q.mu.Unlock()

	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Prepend adds message to front of queue.
func (q *Queue) Prepend(msg model.Message) {
	q.mu.Lock()
	q.list.PushFront(msg)
	q.len.Add(1)
	q.mu.Unlock()

	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Dequeue removes and returns the next message, blocking until one is available
// or ctx is cancelled.
func (q *Queue) Dequeue(ctx context.Context) (model.Message, error) {
	for {
		q.mu.Lock()
		elem := q.list.Front()
		if elem != nil {
			val := elem.Value
			q.list.Remove(elem)
			q.len.Add(-1)
			q.mu.Unlock()
			return val, nil
		}
		q.mu.Unlock()

		select {
		case <-q.notify:
		case <-ctx.Done():
			return model.Message{}, ctx.Err()
		}
	}
}

// DequeueN removes and returns up to n messages from the front of the queue.
// Returns fewer than n messages if the queue is exhausted.
func (q *Queue) DequeueN(n int) []model.Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	result := make([]model.Message, 0, n)
	for range n {
		elem := q.list.Front()
		if elem == nil {
			break
		}
		result = append(result, elem.Value)
		q.list.Remove(elem)
		q.len.Add(-1)
	}
	return result
}

// Peek returns the first message in the queue without removing it.
// Will return ErrEmpty if queue is empty.
func (q *Queue) Peek() (model.Message, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	e := q.list.Front()
	if e == nil {
		return model.Message{}, ErrEmpty
	}
	return e.Value, nil
}

// Len returns the current length of the queue
func (q *Queue) Len() int {
	return int(q.len.Load())
}
