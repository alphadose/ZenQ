package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by parking excess goroutines and pre-empt them when required with minimal latency overhead
// Uses the same lock-free linked list implementation as in `list.go`
type ThreadParker[T any] struct {
	head unsafe.Pointer
	tail unsafe.Pointer
	// memory pool for storing and leasing parkeing spots for goroutines
	poolRef *sync.Pool
}

// NewThreadParker returns a new thread parker.
func NewThreadParker[T any](poolRef *sync.Pool) *ThreadParker[T] {
	tp := new(ThreadParker[T])
	n := poolRef.Get().(*parkSpot[T])
	n.threadPtr, n.next = nil, nil
	tp.head, tp.tail, tp.poolRef = unsafe.Pointer(n), unsafe.Pointer(n), poolRef
	return tp
}

// a single parked goroutine
type parkSpot[T any] struct {
	threadPtr unsafe.Pointer
	next      unsafe.Pointer
	value     T
}

// Park parks the current calling goroutine
// This keeps only one parked goroutine in state at all times
// the parked goroutine is called with minimal overhead via goready() due to both being in userland
// This ensures there is no thundering herd https://en.wikipedia.org/wiki/Thundering_herd_problem
func (tp *ThreadParker[T]) Park(value T) {
	n := tp.poolRef.Get().(*parkSpot[T])
	n.threadPtr, n.next, n.value = GetG(), nil, value
	for {
		tail := (*parkSpot[T])(atomic.LoadPointer(&tp.tail))
		next := (*parkSpot[T])(atomic.LoadPointer(&tail.next))
		if tail == (*parkSpot[T])(atomic.LoadPointer(&tp.tail)) {
			if next == nil {
				if atomic.CompareAndSwapPointer(&tail.next, unsafe.Pointer(next), unsafe.Pointer(n)) {
					atomic.CompareAndSwapPointer(&tp.tail, unsafe.Pointer(tail), unsafe.Pointer(n))
					mcall(fast_park)
					return
				}
			} else {
				atomic.CompareAndSwapPointer(&tp.tail, unsafe.Pointer(tail), unsafe.Pointer(next))
			}
		}
	}
}

// Ready calls one parked goroutine from the queue if available
func (tp *ThreadParker[T]) Ready() (data T, ok bool) {
	for {
		head := (*parkSpot[T])(atomic.LoadPointer(&tp.head))
		tail := (*parkSpot[T])(atomic.LoadPointer(&tp.tail))
		next := (*parkSpot[T])(atomic.LoadPointer(&head.next))
		if head == (*parkSpot[T])(atomic.LoadPointer(&tp.head)) {
			if head == tail {
				if next == nil {
					return
				}
				atomic.CompareAndSwapPointer(&tp.tail, unsafe.Pointer(tail), unsafe.Pointer(next))
			} else {
				safe_ready(next.threadPtr)
				data, ok = next.value, true
				if atomic.CompareAndSwapPointer(&tp.head, unsafe.Pointer(head), unsafe.Pointer(next)) {
					head.threadPtr, head.next = nil, nil
					tp.poolRef.Put(head)
					return
				}
			}
		}
	}
}
