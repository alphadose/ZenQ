package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// global memory pool for storing and leasing node objects
var nodePool = sync.Pool{New: func() any { return new(node) }}

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by parking excess goroutines and pre-empt them when required with minimal latency overhead
// Uses a thread safe linked list for storing goroutine references
// theory from https://www.cs.rochester.edu/research/synchronization/pseudocode/queues.html
type ThreadParker struct {
	head unsafe.Pointer
	tail unsafe.Pointer
}

// NewThreadParker returns a new thread parker.
func NewThreadParker() *ThreadParker {
	n := nodePool.Get().(*node)
	n.value, n.next = nil, nil
	ptr := unsafe.Pointer(n)
	return &ThreadParker{head: ptr, tail: ptr}
}

// a single node in the linked list
type node struct {
	value unsafe.Pointer
	next  unsafe.Pointer
}

// Park parks the current calling goroutine
// This keeps only one parked goroutine in state at all times
// the parked goroutine is called with minimal overhead via goready() due to both being in userland
// This ensures there is no thundering herd https://en.wikipedia.org/wiki/Thundering_herd_problem
func (tp *ThreadParker) Park() {
	tp.Enqueue(GetG())
	mcall(fast_park)
}

// Ready calls one parked goroutine from the queue if available
func (tp *ThreadParker) Ready() (readied bool) {
	if gp := tp.Dequeue(); gp != nil {
		safe_ready(gp)
		readied = true
	}
	return
}

// Enqueue inserts a value into the queue
func (q *ThreadParker) Enqueue(value unsafe.Pointer) {
	n := nodePool.Get().(*node)
	n.value, n.next = value, nil
	for {
		tail := load(&q.tail)
		next := load(&tail.next)
		if tail == load(&q.tail) { // are tail and next consistent?
			if next == nil {
				if cas(&tail.next, next, n) {
					cas(&q.tail, tail, n) // Enqueue is done.  try to swing tail to the inserted node
					return
				}
			} else { // tail was not pointing to the last node
				// try to swing Tail to the next node
				cas(&q.tail, tail, next)
			}
		}
	}
}

// Dequeue removes and returns the value at the head of the queue
// It returns nil if the queue is empty
func (q *ThreadParker) Dequeue() (value unsafe.Pointer) {
	for {
		head := load(&q.head)
		tail := load(&q.tail)
		next := load(&head.next)
		if head == load(&q.head) { // are head, tail, and next consistent?
			if head == tail { // is queue empty or tail falling behind?
				if next == nil { // is queue empty?
					return nil
				}
				// tail is falling behind.  try to advance it
				cas(&q.tail, tail, next)
			} else {
				// read value before CAS otherwise another dequeue might free the next node
				value = next.value
				if cas(&q.head, head, next) {
					// sysFreeOS(unsafe.Pointer(head), nodeSize)
					head.value, head.next = nil, nil
					nodePool.Put(head)
					return // Dequeue is done.  return
				}
			}
		}
	}
}

func load(p *unsafe.Pointer) (n *node) {
	return (*node)(atomic.LoadPointer(p))
}

func cas(p *unsafe.Pointer, old, new *node) (ok bool) {
	return atomic.CompareAndSwapPointer(p, unsafe.Pointer(old), unsafe.Pointer(new))
}
