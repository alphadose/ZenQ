// A minimalist thread-safe queue implemented using a lock-free ringbuffer which is faster
// and has lower memory allocations than golang's native channels
// Based on the LMAX disruptor pattern https://lmax-exchange.github.io/disruptor/disruptor.html

// Known Limitations:-
//
// 1. Max queue_size = 2^16
// 2. The queue_size is a power of 2, in case a different size is provided then queue_size is rounded up to the next greater power of 2 upto a max of 2^16

// Suggestions:-
//
// 1. Use runtime.LockOSThread() on the goroutine calling ZenQ.Read() for lowest latency provided you have > 1 cpu cores

package zenq

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/alphadose/zenq/constants"
)

// ZenQ Slot state enums
const (
	SlotEmpty = iota
	SlotBusy
	SlotCommitted
)

const (
	// queue size must be a power of 2, max size-> 2^31
	size      uint32 = 1 << 14
	indexMask uint32 = size - 1
)

type (
	// a single slot in the queue
	slot[T any] struct {
		atomic.Uint32
		item T
	}

	// ZenQ ringbuffer
	ZenQ[T any] struct {
		_           [constants.CacheLinePadSize]byte
		writerIndex atomic.Uint32
		_           [constants.CacheLinePadSize - unsafe.Sizeof(atomic.Uint32{})]byte
		readerIndex atomic.Uint32
		_           [constants.CacheLinePadSize - unsafe.Sizeof(atomic.Uint32{})]byte
		contents    [size]slot[T]
		_           [constants.CacheLinePadSize]byte
	}
)

// New returns a new queue given its payload type passed as a generic parameter
func New[T any]() *ZenQ[T] {
	return &ZenQ[T]{}
}

// Write a value to the queue
func (self *ZenQ[T]) Write(value T) {
	slot := &self.contents[self.writerIndex.Add(1)&indexMask]
	for !slot.CompareAndSwap(SlotEmpty, SlotBusy) {
		switch slot.Load() {
		case SlotBusy, SlotCommitted:
			runtime.Gosched()
		case SlotEmpty:
			continue
		}
	}
	slot.item = value
	slot.Store(SlotCommitted)
}

// Read a value from the queue
func (self *ZenQ[T]) Read() (data T) {
	slot := &self.contents[self.readerIndex.Add(1)&indexMask]
	for !slot.CompareAndSwap(SlotCommitted, SlotBusy) {
		switch slot.Load() {
		case SlotBusy, SlotEmpty:
			runtime.Gosched()
		case SlotCommitted:
			continue
		}
	}
	data = slot.item
	slot.Store(SlotEmpty)
	return
}

// Dump dumps the current queue state
// Unsafe to be called from multiple goroutines
func (self *ZenQ[T]) Dump() {
	fmt.Printf("writerIndex: %3d, readerIndex: %3d\n contents:-\n\n", self.writerIndex, self.readerIndex)
	for idx := range self.contents {
		fmt.Printf("Slot -> %#v\n", self.contents[idx])
	}
}
