// A minimalist thread-safe queue implemented using a lock-free ringbuffer which is even faster
// and more resource than golang's native channels

// Known Limitations:-
//
// 1. At most (2^64)-2 items can be written to the queue
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
	"sync/atomic"
)

const (
	// The queue size, should be a power of 2
	queueSize uint64 = 1 << 12

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
		State uint32
		Item  T
	}

	// ZenQ is the CPU cache optimized ringbuffer implementation
	ZenQ[T any] struct {
		// The padding members 1 to 4 below are here to ensure each item is on a separate cache line.
		// This prevents false sharing and hence improves performance.
		_p1         cacheLinePadding
		writerIndex uint64
		_p2         cacheLinePadding
		readerIndex uint64
		_p3         cacheLinePadding
		// arrays have faster access speed than slices for single elements
		contents [queueSize]Slot[T]
		_p4      cacheLinePadding
	}
)

// New returns a new queue given its payload type passed as a generic parameter
func New[T any]() *ZenQ[T] {
	return new(ZenQ[T])
}

// Write writes a value to the queue
func (self *ZenQ[T]) Write(value T) {
	// Get writer slot index
	idx := (atomic.AddUint64(&self.writerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	// CAS -> change slot_state to busy if slot_state == empty
	for !atomic.CompareAndSwapUint32(slotState, SlotEmpty, SlotBusy) {
		runtime.Gosched()
	}
	self.contents[idx].Item = value
	// commit write into the slot
	atomic.StoreUint32(slotState, SlotCommitted)
}

// Read reads a value from the queue, you can once read once per object
func (self *ZenQ[T]) Read() T {
	// Get reader slot index
	idx := (atomic.AddUint64(&self.readerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	// change slot_state to empty after this function returns, via defer thereby preventing race conditions
	// Note:- Although defer adds around 200ns of latency, this is required for preventing race conditions
	defer atomic.StoreUint32(slotState, SlotEmpty)
	// CAS -> change slot_state to busy if slot_state == committed
	for !atomic.CompareAndSwapUint32(slotState, SlotCommitted, SlotBusy) {
		runtime.Gosched()
	}
	return self.contents[idx].Item
}

// Dump dumps the current queue state
// Unsafe to be called from multiple goroutines
func (self *ZenQ[T]) Dump() {
	fmt.Printf("nextFree: %3d, readerIndex: %3d, content:", self.writerIndex, self.readerIndex)
	for index, value := range self.contents {
		fmt.Printf("%5v : %5v", index, value)
	}
	fmt.Print("\n")
}

// Reset resets the queue state
// Unsafe to be called from multiple goroutines
func (self *ZenQ[T]) Reset() {
	atomic.StoreUint64(&self.writerIndex, 0)
	atomic.StoreUint64(&self.readerIndex, 0)
	for _, s := range self.contents {
		atomic.StoreUint32(&s.State, SlotEmpty)
	}
}
