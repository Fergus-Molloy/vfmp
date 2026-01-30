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
	mu         *sync.Mutex
	list       *list.List[model.Message]
	topic      string
	msgOutChan chan model.Message
	msgInChan  chan model.Message
	len        atomic.Int64
}

// New returns a new Queue
func New(ctx context.Context, topic string) *Queue {
	q := &Queue{
		mu:         new(sync.Mutex),
		list:       list.New[model.Message](),
		topic:      topic,
		msgOutChan: make(chan model.Message),
		msgInChan:  make(chan model.Message),
		len:        atomic.Int64{},
	}
	go q.handleMessageChans(ctx)

	return q
}

// GetMsgChan returns chan that new messages will be published on
func (q *Queue) GetMsgChan() <-chan model.Message {
	return q.msgOutChan
}

// Append adds message to back of queue.
func (q *Queue) Append(msg model.Message) {
	q.msgInChan <- msg
}

// Peek returns the first message in the queue without removing it.
// Will return ErrEmpty if queue is empty.
func (q *Queue) Peek() (model.Message, error) {
	// does not mutate state so can ignore the lock
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

func (q *Queue) handleMessageChans(ctx context.Context) {
	for {
		q.mu.Lock()
		elem := q.list.Front()
		if elem != nil {
			q.awaitMsgOp(ctx, elem)
		} else {
			q.awaitNewMsg(ctx)
		}
		q.mu.Unlock()
	}
}

func (q *Queue) awaitMsgOp(ctx context.Context, elem *list.Element[model.Message]) {
	select {
	case q.msgOutChan <- elem.Value:
		q.list.Remove(elem)
		q.len.Add(-1)
	case m := <-q.msgInChan:
		q.list.PushBack(m)
		q.len.Add(1)
	case <-ctx.Done():
		return
	}
}

func (q *Queue) awaitNewMsg(ctx context.Context) {
	select {
	case m := <-q.msgInChan:
		q.list.PushBack(m)
		q.len.Add(1)
	case <-ctx.Done():
		return
	}
}
