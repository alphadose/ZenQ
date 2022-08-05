package zenq

import (
	"sync/atomic"
	"unsafe"
)

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by parking excess goroutines and pre-empt them when required with minimal latency overhead
// Uses the same lock-free linked list implementation as in `list.go`
type ThreadParker[T any] struct {
	head atomic.Pointer[parkSpot[T]]
	tail atomic.Pointer[parkSpot[T]]
}

// NewThreadParker returns a new thread parker.
func NewThreadParker[T any](spot *parkSpot[T]) *ThreadParker[T] {
	var ptr atomic.Pointer[parkSpot[T]]
	ptr.Store(spot)
	return &ThreadParker[T]{head: ptr, tail: ptr}
}

// a single parked goroutine
type parkSpot[T any] struct {
	next      atomic.Pointer[parkSpot[T]]
	threadPtr unsafe.Pointer
	value     T
}

// Park parks the current calling goroutine
// This keeps only one parked goroutine in state at all times
// the parked goroutine is called with minimal overhead via goready() due to both being in userland
// This ensures there is no thundering herd https://en.wikipedia.org/wiki/Thundering_herd_problem
func (tp *ThreadParker[T]) Park(nextNode *parkSpot[T]) {
	var tail, next *parkSpot[T]
	for {
		tail = tp.tail.Load()
		next = tail.next.Load()
		if tail == tp.tail.Load() {
			if next == nil {
				if tail.next.CompareAndSwap(next, nextNode) {
					tp.tail.CompareAndSwap(tail, nextNode)
					return
				}
			} else {
				tp.tail.CompareAndSwap(tail, next)
			}
		}
	}
}

// Ready calls one parked goroutine from the queue if available
func (tp *ThreadParker[T]) Ready() (data T, ok bool, freeable *parkSpot[T]) {
	var head, tail, next *parkSpot[T]
	for {
		head = tp.head.Load()
		tail = tp.tail.Load()
		next = head.next.Load()
		if head == tp.head.Load() {
			if head == tail {
				if next == nil {
					return
				}
				tp.tail.CompareAndSwap(tail, next)
			} else {
				safe_ready(next.threadPtr)
				data, ok = next.value, true
				if tp.head.CompareAndSwap(head, next) {
					freeable = head
					freeable.threadPtr = nil
					freeable.next.Store(nil)
					return
				}
			}
		}
	}
}
