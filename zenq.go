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
	"sync"
	"sync/atomic"
)

const (
	// The queue size, should be a power of 2
	queueSize uint64 = 1 << 16

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
		State   uint32
		Sleeper sync.Mutex
		Item    T
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

// commit write into a slot
func (self *ZenQ[T]) commit(index uint64, slotState *uint32, value T) {
	self.contents[index].Item = value
	// commit write into the slot
	atomic.StoreUint32(slotState, SlotCommitted)
}

// Write writes a value to the queue
func (self *ZenQ[T]) Write(value T) {
	// Get writer slot index
	idx := (atomic.AddUint64(&self.writerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	// CAS -> change slot_state to busy if slot_state == empty
	if atomic.CompareAndSwapUint32(slotState, SlotEmpty, SlotBusy) {
		self.commit(idx, slotState, value)
	} else {
		// case where large number of writer goroutines are contending for the same slot
		// hence making them sleep via mutex.Lock() is more efficient resource wise
		sleeper := &self.contents[idx].Sleeper
		sleeper.Lock()
		// this ensures only 1 goroutine is contending for a particular slot
		// at all times and other goroutines(if any) are sleeping
		for !atomic.CompareAndSwapUint32(slotState, SlotEmpty, SlotBusy) {
			runtime.Gosched()
			// need access to goready() function from runtime internals to pre-empt this goroutine after parking
			// unfortunatly the struct `g` to be used in goready() cannot be replicated in this plane
			// goparkunlock(sleeper, "test-parking", traceEvGoBlockSend, 2)
		}
		self.commit(idx, slotState, value)
		// unlock(sleeper)
		sleeper.Unlock()
	}
}

// Read reads a value from the queue, you can once read once per object
func (self *ZenQ[T]) Read() T {
	// Get reader slot index
	idx := (atomic.AddUint64(&self.readerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	// change slot_state to empty after this function returns, via defer thereby preventing race conditions
	// Note:- Although defer adds around 50ns of latency, this is required for preventing race conditions
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
