// A minimalist thread-safe queue implemented using a lock-free ringbuffer which is even faster
// and more resource friendly than golang's native channels

// Known Limitations:-
//
// 1. Current max queue_size = 2^63
// 2. The size of the queue must be a power of 2

// Suggestions:-
//
// 1. Use runtime.LockOSThread() on the goroutine calling ZenQ.Read() for best performance provided you have > 1 cpu cores
// 2. Use large queue sizes (>= 2^14) in SPSC mode for best gains
//

package zenq

import (
	"fmt"
	"runtime"
	"sync/atomic"
)

// whether the system has multiple cores or a single core
var multicore = runtime.NumCPU() > 1

const (
	power = 12

	// The queue size, should be a power of 2
	queueSize = 1 << power

	// Masking is faster than division, only works with numbers which are powers of 2
	indexMask = queueSize - 1
)

// ZenQ global state enums
const (
	// Both reads and writes are possible
	StateOpen = iota
	// No further writes can be performed and you can only read upto the last committed write in this state
	StateClosedForWrites
	// Neither reads nor writes are possible, queue is fully exhausted
	StateFullyClosed
)

// ZenQ Slot state enums
const (
	SlotEmpty = iota
	SlotBusy
	SlotCommitted
	SlotClosed
)

type (
	// Most modern CPUs have cache line size of 64 bytes
	cacheLinePadding [8]uint64

	Slot[T any] struct {
		State        uint32
		WriteParker  *ThreadParker
		SelectParker *ThreadParker
		Item         T
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
		globalState uint32
		_p4         cacheLinePadding
		// arrays have faster access speed than slices for single elements
		contents [queueSize]Slot[T]
		_p5      cacheLinePadding
	}
)

// New returns a new queue given its payload type passed as a generic parameter
func New[T any]() *ZenQ[T] {
	var contents [queueSize]Slot[T]
	for idx := range contents {
		contents[idx].WriteParker, contents[idx].SelectParker = NewThreadParker(), NewThreadParker()
	}
	return &ZenQ[T]{contents: contents}
}

// Write writes a value to the queue
func (self *ZenQ[T]) Write(value T) {
	if atomic.LoadUint32(&self.globalState) > StateOpen {
		return
	}
	idx := (atomic.AddUint64(&self.writerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	writeParker := self.contents[idx].WriteParker
	// selectParker := self.contents[idx].SelectParker

retry:
	// CAS -> change slot_state to busy if slot_state == empty
	if !atomic.CompareAndSwapUint32(slotState, SlotEmpty, SlotBusy) {
		// The body of this for loop will never be invoked in case of SPSC (Single-Producer-Single-Consumer) mode
		// guaranteening low latency unless the user's reader thread is blocked for some reason
		// selectParker.Ready()
		writeParker.Park()
		goto retry
	}
	self.contents[idx].Item = value
	atomic.StoreUint32(slotState, SlotCommitted)
	// Ready blocking selector if any
	// selectParker.Ready()
}

// Read reads a value from the queue, you can once read once per object
func (self *ZenQ[T]) Read() (data T, open bool) {
	idx := (atomic.AddUint64(&self.readerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	writeParker := self.contents[idx].WriteParker

	// change slot_state to empty after this function returns, via defer thereby preventing race conditions
	// Note:- Although defer adds around 50ns of latency, this is required for preventing race conditions
	// defer consume(slotState, writeParker)
	defer atomic.StoreUint32(slotState, SlotEmpty)

	shouldSpin := false

	// CAS -> change slot_state to busy if slot_state == committed
	for !atomic.CompareAndSwapUint32(slotState, SlotCommitted, SlotBusy) {
		switch atomic.LoadUint32(slotState) {
		case SlotBusy:
			wait()
		case SlotEmpty:
			if atomic.LoadUint32(&self.globalState) == StateFullyClosed {
				return
			}
			shouldSpin = shouldSpin || writeParker.Ready()
			if shouldSpin {
				wait()
			} else {
				runtime.Gosched()
			}
		case SlotClosed:
			if atomic.CompareAndSwapUint32(slotState, SlotClosed, SlotEmpty) {
				atomic.CompareAndSwapUint32(&self.globalState, StateClosedForWrites, StateFullyClosed)
				// Queue closed, released all parked select_readers
				// self.contents[idx].SelectParker.Release()
			}
			return getDefault[T](), false
		case SlotCommitted:
			continue
		}
	}
	return self.contents[idx].Item, true
}

// Close closes the ZenQ for further writes
// You can only read uptill the last committed write after closing
// This function will be blocking in case the queue is full
// ZenQ is closed from a writer goroutine by design, hence it should always be called
// from a writer goroutine and never from a reader goroutine which might cause the reader to get blocked and hence deadlock
func (self *ZenQ[T]) Close() {
	// This ensures a ZenQ is closed only once even if this function is called multiple times making this operation safe
	if !atomic.CompareAndSwapUint32(&self.globalState, StateOpen, StateClosedForWrites) {
		return
	}
	idx := (atomic.AddUint64(&self.writerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	writeParker := self.contents[idx].WriteParker
	selectParker := self.contents[idx].SelectParker

	// CAS -> change slot_state to busy if slot_state == empty
	for !atomic.CompareAndSwapUint32(slotState, SlotEmpty, SlotBusy) {
		selectParker.Ready()
		writeParker.Park()
	}
	// Closing commit
	atomic.StoreUint32(slotState, SlotClosed)
	selectParker.Ready()
}

// CloseAsync closes the channel asynchronously
// Useful when an user wants to close the channel from a reader end without blocking the thread
func (self *ZenQ[T]) CloseAsync() {
	go self.Close()
}

// Implementation of Selectable interface

// SelectRead is used for read selection among multiple ZenQs
// Contest for reads in a less aggressive manner to save resources
// There wont be any context switching between selection reader goroutines, they will be signalled via selectParker
func (self *ZenQ[T]) SelectRead(sel *Selection) {
	// return object to memory pool after all selectable goroutines have returned
	defer sel.DecrementReferenceCount()

	if atomic.LoadUint32(&self.globalState) == StateFullyClosed {
		return
	}

	// Get reader index
	idx := (atomic.AddUint64(&self.readerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	writeParker := self.contents[idx].WriteParker
	// selectParker := self.contents[idx].SelectParker

	for !atomic.CompareAndSwapUint32(slotState, SlotCommitted, SlotBusy) {
		switch atomic.LoadUint32(slotState) {
		case SlotBusy:
			runtime.Gosched()
		case SlotEmpty:
			writeParker.Ready()
			runtime_doSpin()
			// if writeParker.Ready() {
			// 	runtime.Gosched()
			// } else {
			// 	// selectParker.ParkBack()
			// 	runtime_doSpin()
			// }
		case SlotClosed:
			if atomic.CompareAndSwapUint32(slotState, SlotClosed, SlotEmpty) {
				atomic.CompareAndSwapUint32(&self.globalState, StateClosedForWrites, StateFullyClosed)
				// selectParker.Release()
			}
			return
		case SlotCommitted:
			continue
		}
		// Drop slot contention if the main selector thread no longer exists or if queue is closed
		if sel.Selected() || atomic.LoadUint32(&self.globalState) == StateFullyClosed {
			return
		}
	}

	data := self.contents[idx].Item

	defer atomic.StoreUint32(slotState, SlotEmpty)

	if sel.AcquireLock() {
		sel.WriteAndSchedule(data)
	} else {
		// go self.WriteWithHighPriority(data)
		go self.Write(data)
	}
}

// Reset resets the queue state
// This also releases all parked goroutines if any and drains all committed writes
func (self *ZenQ[T]) Reset() {
	// Close() is blocking when queue is full hence execute it asynchronously
	self.CloseAsync()
drain:
	for {
		if _, open := self.Read(); !open {
			break drain
		}
	}
	atomic.CompareAndSwapUint32(&self.globalState, StateFullyClosed, StateOpen)
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

// returns a default value of any type
func getDefault[T any]() T {
	return *new(T)
}
