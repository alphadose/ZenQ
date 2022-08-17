package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// global memory pool for storing and leasing node objects
var (
	nodePool = sync.Pool{New: func() any { return new(node) }}
	nodeGet  = nodePool.Get
	nodePut  = nodePool.Put
)

// List is a lock-free linked list
// theory -> https://www.cs.rochester.edu/u/scott/papers/1996_PODC_queues.pdf
// pseudocode -> https://www.cs.rochester.edu/research/synchronization/pseudocode/queues.html
type List struct {
	head atomic.Pointer[node]
	tail atomic.Pointer[node]
}

// NewList returns a new list
func NewList() List {
	n := nodeGet().(*node)
	n.threadPtr, n.dataOut = nil, nil
	n.next.Store(nil)
	var ptr atomic.Pointer[node]
	ptr.Store(n)
	return List{head: ptr, tail: ptr}
}

// a single node in the linked list
type node struct {
	next      atomic.Pointer[node]
	threadPtr *unsafe.Pointer
	dataOut   *any
}

// Enqueue inserts a value into the list
func (l *List) Enqueue(threadPtr *unsafe.Pointer, dataOut *any) {
	var (
		n          = nodeGet().(*node)
		tail, next *node
	)
	n.threadPtr, n.dataOut = threadPtr, dataOut
	for {
		tail = l.tail.Load()
		next = tail.next.Load()
		if tail == l.tail.Load() { // are tail and next consistent?
			if next == nil {
				if tail.next.CompareAndSwap(next, n) {
					l.tail.CompareAndSwap(tail, n) // Enqueue is done.  try to swing tail to the inserted node
					return
				}
			} else { // tail was not pointing to the last node
				// try to swing Tail to the next node
				l.tail.CompareAndSwap(tail, next)
			}
		}
	}
}

// Dequeue removes and returns the value at the head of the queue to the memory pool
// It returns nil if the list is empty
func (l *List) Dequeue() (threadPtr *unsafe.Pointer, dataOut *any) {
	var head, tail, next *node
	for {
		head = l.head.Load()
		tail = l.tail.Load()
		next = head.next.Load()
		if head == l.head.Load() { // are head, tail, and next consistent?
			if head == tail { // is list empty or tail falling behind?
				if next == nil { // is list empty?
					return nil, nil
				}
				// tail is falling behind.  try to advance it
				l.tail.CompareAndSwap(tail, next)
			} else {
				// read value before CAS_node otherwise another dequeue might free the next node
				threadPtr, dataOut = next.threadPtr, next.dataOut
				if l.head.CompareAndSwap(head, next) {
					// sysFreeOS(unsafe.Pointer(head), nodeSize)
					head.threadPtr, head.dataOut = nil, nil
					head.next.Store(nil)
					nodePut(head)
					return // Dequeue is done.  return
				}
			}
		}
	}
}
