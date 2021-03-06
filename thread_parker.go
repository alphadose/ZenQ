package zenq

import (
	"sync/atomic"
	"unsafe"
)

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by parking excess goroutines and pre-empt them when required with minimal latency overhead
// Uses the same lock-free linked list implementation as in `list.go`
type ThreadParker[T any] struct {
	head unsafe.Pointer
	tail unsafe.Pointer
}

// NewThreadParker returns a new thread parker.
func NewThreadParker[T any](n unsafe.Pointer) *ThreadParker[T] {
	return &ThreadParker[T]{head: n, tail: n}
}

// a single parked goroutine
type parkSpot[T any] struct {
	next      unsafe.Pointer
	threadPtr unsafe.Pointer
	value     T
}

// Park parks the current calling goroutine
// This keeps only one parked goroutine in state at all times
// the parked goroutine is called with minimal overhead via goready() due to both being in userland
// This ensures there is no thundering herd https://en.wikipedia.org/wiki/Thundering_herd_problem
func (tp *ThreadParker[T]) Park(nextNode unsafe.Pointer) {
	var tail, next unsafe.Pointer
	for {
		tail = atomic.LoadPointer(&tp.tail)
		next = atomic.LoadPointer(&((*parkSpot[T])(tail)).next)
		if tail == atomic.LoadPointer(&tp.tail) {
			if next == nil {
				if atomic.CompareAndSwapPointer(&((*parkSpot[T])(tail)).next, next, nextNode) {
					atomic.CompareAndSwapPointer(&tp.tail, tail, nextNode)
					return
				}
			} else {
				atomic.CompareAndSwapPointer(&tp.tail, tail, next)
			}
		}
	}
}

// Ready calls one parked goroutine from the queue if available
func (tp *ThreadParker[T]) Ready() (data T, ok bool, freeable *parkSpot[T]) {
	var head, tail, next unsafe.Pointer
	for {
		head = atomic.LoadPointer(&tp.head)
		tail = atomic.LoadPointer(&tp.tail)
		next = atomic.LoadPointer(&((*parkSpot[T])(head)).next)
		if head == atomic.LoadPointer(&tp.head) {
			if head == tail {
				if next == nil {
					return
				}
				atomic.CompareAndSwapPointer(&tp.tail, tail, next)
			} else {
				safe_ready((*parkSpot[T])(next).threadPtr)
				data, ok = (*parkSpot[T])(next).value, true
				if atomic.CompareAndSwapPointer(&tp.head, head, next) {
					freeable = (*parkSpot[T])(head)
					freeable.next, freeable.threadPtr = nil, nil
					return
				}
			}
		}
	}
}
