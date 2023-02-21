package types

import (
	"context"
	"sync"
)

type Queue[T any] interface {
	Enqueue(ctx context.Context, data T) error
	Dequeue(ctx context.Context) (T, error)
	IsEmpty() bool
}

type BlockingQueue[T any] struct {
	data     []T
	mutex    *sync.Mutex
	notEmpty chan struct{}
	notFull  chan struct{}
	maxSize  int
}

func NewBlockingQueue[T any](maxSize int) *BlockingQueue[T] {
	return &BlockingQueue[T]{
		data:     make([]T, 0, maxSize),
		mutex:    new(sync.Mutex),
		notFull:  make(chan struct{}, 1),
		notEmpty: make(chan struct{}, 1),
		maxSize:  maxSize,
	}
}

func (q *BlockingQueue[T]) Enqueue(ctx context.Context, item T) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	q.mutex.Lock()

	for q.IsFull() {
		q.mutex.Unlock()

		select {
		case <-q.notFull:
			q.mutex.Lock()
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	q.data = append(q.data, item)
	if len(q.data) == 1 {
		q.notEmpty <- struct{}{}
	}

	q.mutex.Unlock()

	return nil
}

func (q *BlockingQueue[T]) Dequeue(ctx context.Context) (T, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	for q.IsEmpty() {
		q.mutex.Unlock()
		select {
		case <-q.notEmpty:
			q.mutex.Lock()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	item := q.data[0]
	q.data = q.data[1:]

	if len(q.data) == q.maxSize-1 {
		q.notFull <- struct{}{}
	}

	q.mutex.Unlock()

	return item, nil
}

func (q *BlockingQueue[T]) IsEmpty() bool {
	return false
}

func (q *BlockingQueue[T]) IsFull() bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return len(q.data) == q.maxSize
}
