package queue

import (
	"context"
	"testing"
	"time"

	"fergus.molloy.xyz/vfmp/internal/model"
)

// TestAppendDoesntBlock asserts that we can always append to the queue, even
// when no messages are being consumed
func TestAppendDoesntBlock(t *testing.T) {
	ctx := t.Context()
	q := New(ctx, "topic")

	q.Append(model.Message{Data: []byte("data"), Topic: "topic"})

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for range 10 {
			q.Append(model.Message{Data: []byte("data 2"), Topic: "topic"})
			time.Sleep(time.Millisecond * 10)
		}
		done <- struct{}{}
	}()

	select {
	case <-done:
		return
	case <-timeoutCtx.Done():
		t.Fatalf("timedout waiting to append new message to queue")
	}
}

func TestLengthIncrementsCorrectly(t *testing.T) {
	ctx := t.Context()
	q := New(ctx, "topic")

	if q.Len() != 0 {
		t.Fatalf("queue has incorrect length, got: %d, want: %d", q.Len(), 0)
	}

	q.Append(model.Message{})
	time.Sleep(10 * time.Millisecond)
	if q.Len() != 1 {
		t.Fatalf("queue has incorrect length, got: %d, want: %d", q.Len(), 1)
	}

	q.Append(model.Message{})
	time.Sleep(10 * time.Millisecond)
	if q.Len() != 2 {
		t.Fatalf("queue has incorrect length, got: %d, want: %d", q.Len(), 2)
	}

	_, _ = q.Dequeue(ctx)
	time.Sleep(10 * time.Millisecond)
	if q.Len() != 1 {
		t.Fatalf("queue has incorrect length, got: %d, want: %d", q.Len(), 1)
	}

	_, _ = q.Dequeue(ctx)
	time.Sleep(10 * time.Millisecond)
	if q.Len() != 0 {
		t.Fatalf("queue has incorrect length, got: %d, want: %d", q.Len(), 0)
	}
}
