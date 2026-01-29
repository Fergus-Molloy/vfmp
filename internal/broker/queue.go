package broker

import (
	"context"
	"sync"

	"fergus.molloy.xyz/vfmp/internal/model"
	"fergus.molloy.xyz/vfmp/internal/model/list"
)

type Queue struct {
	mu      *sync.Mutex
	list    *list.List[model.Message]
	MsgChan chan model.Message
}

func NewQueue(ctx context.Context) *Queue {
	q := &Queue{
		mu:      new(sync.Mutex),
		list:    list.New[model.Message](),
		MsgChan: make(chan model.Message, 1),
	}
	go q.sendMessages(ctx)

	return q
}

func (q *Queue) sendMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			elem := q.list.PopFront()
			if elem != nil {
				q.MsgChan <- elem.Value
			}
		}
	}
}

func (q *Queue) Append(msg model.Message) {
	q.list.PushBack(msg)
}
