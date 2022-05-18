package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

var nodePool = sync.Pool{
	New: func() any { return new(node) },
}

var dtoPool = sync.Pool{
	New: func() any { return new(parkCommit) },
}

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by parking excess goroutines and pre-empt them when required with minimal latency overhead
type ThreadParker struct {
	head unsafe.Pointer
	tail unsafe.Pointer
}

// NewThreadParker returns a new thread parker.
func NewThreadParker() *ThreadParker {
	n := unsafe.Pointer(new(node))
	return &ThreadParker{head: n, tail: n}
}

type node struct {
	value unsafe.Pointer
	next  unsafe.Pointer
}

type parkCommit struct {
	tp *ThreadParker
	n  *node
}

// used for storing the goroutine pointer *g
func zenqParkCommit(gp, tp unsafe.Pointer) bool {
	// obj := (*parkCommit)(n)
	// obj.n.value = gp
	// obj.tp.Enqueue(obj.n)
	t := (*ThreadParker)(tp)
	t.Enqueue(&node{value: gp})
	return true
}

// Park parks the current calling goroutine
// This keeps only one parked goroutine in state at all times
// the parked goroutine is called with minimal overhead via goready() due to both being in userland
// This ensures there is no thundering herd https://en.wikipedia.org/wiki/Thundering_herd_problem
func (tp *ThreadParker) Park() {
	// n := dtoPool.Get().(*parkCommit)
	// n.n = nodePool.Get().(*node)
	// n.tp = tp
	GoPark(zenqParkCommit, unsafe.Pointer(tp), waitReasonSleep, traceEvGoBlock, 1)
	// n.tp = nil
	// nodePool.Put(n)
	// dtoPool.Put(n)
}

// Ready calls the parked goroutine if any and moves other goroutines up the queue
func (tp *ThreadParker) Ready() {
	if node := tp.Dequeue(); node != nil {
		GoReady(node.value, 1)
	}
}

// Enqueue puts the given value v at the tail of the queue.
func (q *ThreadParker) Enqueue(n *node) {
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

// Dequeue removes and returns the value at the head of the queue.
// It returns nil if the queue is empty.
func (q *ThreadParker) Dequeue() *node {
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
				v := next
				if cas(&q.head, head, next) {
					return v // Dequeue is done.  return
				}
			}
		}
	}
}

func load(p *unsafe.Pointer) (n *node) {
	return (*node)(atomic.LoadPointer(p))
}

func cas(p *unsafe.Pointer, old, new *node) (ok bool) {
	return atomic.CompareAndSwapPointer(
		p, unsafe.Pointer(old), unsafe.Pointer(new))
}
