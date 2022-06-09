package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// global memory pool for storing and leasing parkeing spots for goroutines
var parkPool = sync.Pool{New: func() any { return new(parkSpot) }}

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by parking excess goroutines and pre-empt them when required with minimal latency overhead
// Uses the same lock-free linked list implementation as in `list.go`
type ThreadParker struct {
	head unsafe.Pointer
	tail unsafe.Pointer
}

// NewThreadParker returns a new thread parker.
func NewThreadParker() *ThreadParker {
	n := parkPool.Get().(*parkSpot)
	n.threadPtr, n.value, n.next = nil, nil, nil
	ptr := unsafe.Pointer(n)
	return &ThreadParker{head: ptr, tail: ptr}
}

// a single parked goroutine
type parkSpot struct {
	threadPtr unsafe.Pointer
	value     unsafe.Pointer
	next      unsafe.Pointer
}

// Park parks the current calling goroutine
// This keeps only one parked goroutine in state at all times
// the parked goroutine is called with minimal overhead via goready() due to both being in userland
// This ensures there is no thundering herd https://en.wikipedia.org/wiki/Thundering_herd_problem
func (tp *ThreadParker) Park() {
	n := parkPool.Get().(*parkSpot)
	n.threadPtr, n.value, n.next = GetG(), nil, nil
enqueue:
	for {
		tail := load_spot(&tp.tail)
		next := load_spot(&tail.next)
		if tail == load_spot(&tp.tail) {
			if next == nil {
				if cas_spot(&tail.next, next, n) {
					cas_spot(&tp.tail, tail, n)
					break enqueue
				}
			} else {
				cas_spot(&tp.tail, tail, next)
			}
		}
	}
	mcall(fast_park)
}

// Ready calls one parked goroutine from the queue if available
func (tp *ThreadParker) Ready() (readied bool) {
dequeue:
	for {
		head := load_spot(&tp.head)
		tail := load_spot(&tp.tail)
		next := load_spot(&head.next)
		if head == load_spot(&tp.head) {
			if head == tail {
				if next == nil {
					break dequeue
				}
				cas_spot(&tp.tail, tail, next)
			} else {
				safe_ready(next.threadPtr)
				if cas_spot(&tp.head, head, next) {
					head.threadPtr, head.value, head.next, readied = nil, nil, nil, true
					parkPool.Put(head)
					return
				}
			}
		}
	}
	return
}

func load_spot(p *unsafe.Pointer) (n *parkSpot) {
	n = (*parkSpot)(atomic.LoadPointer(p))
	return
}

func cas_spot(p *unsafe.Pointer, old, new *parkSpot) (ok bool) {
	ok = atomic.CompareAndSwapPointer(p, unsafe.Pointer(old), unsafe.Pointer(new))
	return
}
