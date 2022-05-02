// A minimalist thread-safe queue implemented using a lock-free ringbuffer which is even faster than golang's native channels

// Known Limitations:-
//
// 1. At most (2^64)-2 items can be written to the queue
// 2. The size of the queue must be a power of 2

// Suggestions:-
//
// 1. If you have enough cores you can change from runtime.Gosched() to a busy loop
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

// Most modern CPUs have cache line size of 64 bytes
type cacheLinePadding [8]uint64

// ZenQ is the CPU cache optimized ringbuffer implementation
type ZenQ[T any] struct {
	// The padding members 1 to 5 below are here to ensure each item is on a separate cache line.
	// This prevents false sharing and hence improves performance.
	_p1                cacheLinePadding
	lastCommittedIndex uint64
	_p2                cacheLinePadding
	nextFreeIndex      uint64
	_p3                cacheLinePadding
	readerIndex        uint64
	_p4                cacheLinePadding
	// arrays have faster access speed than slices for single elements
	contents [queueSize]T
	_p5      cacheLinePadding
}

// New returns a new queue given its payload type
func New[T any]() *ZenQ[T] {
	return &ZenQ[T]{lastCommittedIndex: 0, nextFreeIndex: 1, readerIndex: 1}
}

// Write writes a value to the queue
func (self *ZenQ[T]) Write(value T) {
	myIndex := atomic.AddUint64(&self.nextFreeIndex, 1) - 1
	//Wait for reader to catch up, so we don't clobber a slot which it is (or will be) reading
	for myIndex > atomic.LoadUint64(&self.readerIndex)+queueSize-2 {
		runtime.Gosched()
	}
	//Write the item into it's slot
	self.contents[myIndex&indexMask] = value

	//Increment the lastCommittedIndex so the item is available for reading
	for !atomic.CompareAndSwapUint64(&self.lastCommittedIndex, myIndex-1, myIndex) {
		runtime.Gosched()
	}
}

// Read reads a value from the queue, you can once read once per object
func (self *ZenQ[T]) Read() T {
	myIndex := atomic.AddUint64(&self.readerIndex, 1) - 1
	//If reader has out-run writer, wait for a value to be committed
	for myIndex > atomic.LoadUint64(&self.lastCommittedIndex) {
		runtime.Gosched()
	}
	return self.contents[myIndex&indexMask]
}

// Dump dumps the current queue state
func (self *ZenQ[T]) Dump() {
	fmt.Printf("lastCommitted: %3d, nextFree: %3d, readerIndex: %3d, content:", self.lastCommittedIndex, self.nextFreeIndex, self.readerIndex)
	for index, value := range self.contents {
		fmt.Printf("%5v : %5v", index, value)
	}
	fmt.Print("\n")
}

// Reset resets the queue state
func (self *ZenQ[T]) Reset() {
	atomic.StoreUint64(&self.lastCommittedIndex, 0)
	atomic.StoreUint64(&self.nextFreeIndex, 1)
	atomic.StoreUint64(&self.readerIndex, 1)
}
