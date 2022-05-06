// A minimalist thread-safe queue implemented using a lock-free ringbuffer which is even faster
// and more resource friendly than golang's native channels

// Known Limitations:-
//
// 1. Current max queue_size = 2^63
// 2. The size of the queue must be a power of 2

// Suggestions:-
//
// 1. If you have enough cores you can change from runtime.Gosched() to a busy loop
// 2. Use runtime.LockOSThread() on the goroutine calling ZenQ.Read() for best performance provided you have > 1 cpu cores
//

package zenq

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

const (
	power uint64 = 12

	// The queue size, should be a power of 2
	queueSize uint64 = 1 << power

	// Masking is faster than division, only works with numbers which are powers of 2
	indexMask uint64 = queueSize - 1
)

// ZenQ Slot state enums
const (
	SlotEmpty = iota
	SlotBusy
	SlotCommitted
)

type (
	// Most modern CPUs have cache line size of 64 bytes
	cacheLinePadding [8]uint64

	// Slot represents a single slot in ZenQ each one having its own state
	Slot[T any] struct {
		State  uint32
		Parker ThreadParker
		Item   T
	}

	// ZenQ is the CPU cache optimized ringbuffer implementation
	ZenQ[T any] struct {
		// The padding members 1 to 5 below are here to ensure each item is on a separate cache line.
		// This prevents false sharing and hence improves performance.
		_p1         cacheLinePadding
		writerIndex uint64
		_p2         cacheLinePadding
		readerIndex uint64
		_p3         cacheLinePadding
		subscriber  *ZenQ[any]
		_p4         cacheLinePadding
		// arrays have faster access speed than slices for single elements
		contents [queueSize]Slot[T]
		_p5      cacheLinePadding
	}
)

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by putting excess goroutines to sleep and pre-empt them when required with minimal latency overhead
type ThreadParker struct {
	semaCount int64
	sync.Mutex
}

// Park parks the current calling goroutine
// Edge Case:- when semaCount is 0, the first calling goroutine needs to call this twice to be parked
func (tp *ThreadParker) Park() {
	tp.Lock()
	atomic.AddInt64(&tp.semaCount, 1)
}

// Ready wakes up all sleeping goroutines associated with this ThreadParker object
// Underlying implementation depends on the OS, for linux its futex, for BSD/MacOS its sema_wakeup etc
func (tp *ThreadParker) Ready() {
start:
	ctr := atomic.LoadInt64(&tp.semaCount)
	if ctr > 0 {
		// this prevents race condition arising from multiple concurrent reader goroutines case
		if atomic.CompareAndSwapInt64(&tp.semaCount, ctr, ctr-1) {
			tp.Unlock()
		} else {
			goto start
		}
	}
}

// New returns a new queue given its payload type passed as a generic parameter
func New[T any]() *ZenQ[T] {
	return new(ZenQ[T])
}

// Write writes a value to the queue
func (self *ZenQ[T]) Write(value T) {
	// if a selector has subscribed to this queue, send data to the selector directly which is another ZenQ
	if self.subscriber != nil {
		self.subscriber.Send(value)
		return
	}

	// Get writer slot index
	idx := (atomic.AddUint64(&self.writerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	parker := &self.contents[idx].Parker

	// CAS -> change slot_state to busy if slot_state == empty
	for !atomic.CompareAndSwapUint32(slotState, SlotEmpty, SlotBusy) {
		// The body of this for loop will never be invoked in case of SPSC (Single-Producer-Single-Consumer) mode
		// guaranteening low latency unless the user's reader thread is blocked for some reason
		parker.Park()
	}
	self.contents[idx].Item = value
	// commit write into the slot
	atomic.StoreUint32(slotState, SlotCommitted)

}

// consume consumes a slot and marks it ready for writing
func consume(slotState *uint32, parker *ThreadParker) {
	atomic.StoreUint32(slotState, SlotEmpty)
	parker.Ready()
}

// Read reads a value from the queue, you can once read once per object
func (self *ZenQ[T]) Read() T {
	// Get reader slot index
	idx := (atomic.AddUint64(&self.readerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	parker := &self.contents[idx].Parker

	// change slot_state to empty after this function returns, via defer thereby preventing race conditions
	// Note:- Although defer adds around 50ns of latency, this is required for preventing race conditions
	defer consume(slotState, parker)

	// CAS -> change slot_state to busy if slot_state == committed
	for !atomic.CompareAndSwapUint32(slotState, SlotCommitted, SlotBusy) {
		runtime.Gosched()
	}
	return self.contents[idx].Item
}

// Dump dumps the current queue state
// Unsafe to be called from multiple goroutines
func (self *ZenQ[T]) Dump() {
	fmt.Printf("writerIndex: %3d, readerIndex: %3d, content:", self.writerIndex, self.readerIndex)
	for index := range self.contents {
		fmt.Printf("%5v : State -> %5v, Item -> %5v", index, self.contents[index].State, self.contents[index].Item)
	}
	fmt.Print("\n")
}

// Reset resets the queue state
// Unsafe to be called from multiple goroutines
func (self *ZenQ[T]) Reset() {
	atomic.StoreUint64(&self.writerIndex, 0)
	atomic.StoreUint64(&self.readerIndex, 0)
	for idx := range self.contents {
		atomic.StoreUint32(&self.contents[idx].State, SlotEmpty)
	}
}
