package blocking

import (
	"sync"
)

// CondQueue 使用"sync.Cond"和"双向链表"实现的阻塞队列
// 当队列空的时候阻塞消费者，队列满时阻塞生产者，其它情况不阻塞
type CondQueue[T any] struct {
	len     uint32
	cap     uint32
	head    *condQueueNode[T]
	tail    *condQueueNode[T]
	lock    *sync.Mutex
	inCond  *sync.Cond
	outCond *sync.Cond
}

type condQueueNode[T any] struct {
	data T
	next *condQueueNode[T]
	prev *condQueueNode[T]
}

// NewCondQueue 初始化队列时需要传入容量
func NewCondQueue[T any](cap uint32) *CondQueue[T] {
	var lock sync.Mutex
	inCond := sync.NewCond(&lock)
	outCond := sync.NewCond(&lock)
	return &CondQueue[T]{
		len:     0,
		cap:     cap,
		head:    nil,
		tail:    nil,
		lock:    &lock,
		inCond:  inCond,
		outCond: outCond,
	}
}

func (q *CondQueue[T]) Produce(data T) {
	q.lock.Lock()
	for q.len >= q.cap {
		q.inCond.Wait()
	}
	node := &condQueueNode[T]{
		data: data,
	}
	if q.head == nil {
		q.tail = node
		q.head = node
	} else {
		q.tail.next = node
		node.prev = q.tail
		q.tail = node
	}
	q.len++

	q.lock.Unlock()
	q.outCond.Broadcast()
}

func (q *CondQueue[T]) Consume() T {
	q.lock.Lock()
	for q.len <= 0 {
		q.outCond.Wait()
	}
	res := q.head
	q.head = q.head.next
	if q.head != nil {
		q.head.prev = nil
	}
	q.len--

	q.lock.Unlock()
	q.inCond.Broadcast()

	return res.data
}
