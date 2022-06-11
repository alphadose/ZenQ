package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// global memory pool for storing and leasing node objects
var nodePool = sync.Pool{New: func() any { return new(node) }}

// List is a lock-free linked list
// theory -> https://www.cs.rochester.edu/u/scott/papers/1996_PODC_queues.pdf
// pseudocode -> https://www.cs.rochester.edu/research/synchronization/pseudocode/queues.html
type List struct {
	head unsafe.Pointer
	tail unsafe.Pointer
}

// NewList returns a new list
func NewList() List {
	n := nodePool.Get().(*node)
	n.value, n.next = nil, nil
	ptr := unsafe.Pointer(n)
	return List{head: ptr, tail: ptr}
}

// a single node in the linked list
type node struct {
	value unsafe.Pointer
	next  unsafe.Pointer
}

// Enqueue inserts a value into the list
func (l *List) Enqueue(value unsafe.Pointer) {
	n := nodePool.Get().(*node)
	n.value, n.next = value, nil
	for {
		tail := load_node(&l.tail)
		next := load_node(&tail.next)
		if tail == load_node(&l.tail) { // are tail and next consistent?
			if next == nil {
				if cas_node(&tail.next, next, n) {
					cas_node(&l.tail, tail, n) // Enqueue is done.  try to swing tail to the inserted node
					return
				}
			} else { // tail was not pointing to the last node
				// try to swing Tail to the next node
				cas_node(&l.tail, tail, next)
			}
		}
	}
}

// Dequeue removes and returns the value at the head of the queue to the memory pool
// It returns nil if the list is empty
func (l *List) Dequeue() (value unsafe.Pointer) {
	for {
		head := load_node(&l.head)
		tail := load_node(&l.tail)
		next := load_node(&head.next)
		if head == load_node(&l.head) { // are head, tail, and next consistent?
			if head == tail { // is list empty or tail falling behind?
				if next == nil { // is list empty?
					return nil
				}
				// tail is falling behind.  try to advance it
				cas_node(&l.tail, tail, next)
			} else {
				// read value before CAS_node otherwise another dequeue might free the next node
				value = next.value
				if cas_node(&l.head, head, next) {
					// sysFreeOS(unsafe.Pointer(head), nodeSize)
					head.value, head.next = nil, nil
					nodePool.Put(head)
					return // Dequeue is done.  return
				}
			}
		}
	}
}

func load_node(p *unsafe.Pointer) (n *node) {
	n = (*node)(atomic.LoadPointer(p))
	return
}

func cas_node(p *unsafe.Pointer, old, new *node) (ok bool) {
	ok = atomic.CompareAndSwapPointer(p, unsafe.Pointer(old), unsafe.Pointer(new))
	return
}
