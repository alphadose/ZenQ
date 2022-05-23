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
	"unsafe"
)

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

// ZenQ selector state enums
const (
	// Open for being selected
	SelectionOpen = iota
	// Running state
	SelectionRunning
)

// ZenQ Slot state enums
const (
	SlotEmpty = iota
	SlotBusy
	SlotCommitted
	SlotClosed
)

type (
	Slot[T any] struct {
		State       uint32
		WriteParker *ThreadParker
		Item        T
	}

	SelectFactory struct {
		auxThread unsafe.Pointer
		state     uint32
		// ThreadParker used as a linked list for queueing selector read requests
		waitList *ThreadParker
	}

	meow [8]uint64

	// ZenQ is the CPU cache optimized ringbuffer implementation
	ZenQ[T any] struct {
		// The padding members 1 to 5 below are here to ensure each item is on a separate cache line.
		// This prevents false sharing and hence improves performance.
		writerIndex   uint64
		_p1           [cacheLinePadSize - unsafe.Sizeof(uint64(0))]byte
		readerIndex   uint64
		_p2           [cacheLinePadSize - unsafe.Sizeof(uint64(0))]byte
		globalState   uint32
		_p3           [cacheLinePadSize - unsafe.Sizeof(uint32(0))]byte
		selectFactory SelectFactory
		_p4           [cacheLinePadSize - unsafe.Sizeof(SelectFactory{})]byte
		// arrays have faster access speed than slices for single elements
		contents [queueSize]Slot[T]
		_p5      cacheLinePadding
	}
)

// New returns a new queue given its payload type passed as a generic parameter
func New[T any]() *ZenQ[T] {
	var contents [queueSize]Slot[T]
	for idx := range contents {
		contents[idx].WriteParker = NewThreadParker()
	}
	println(unsafe.Sizeof(contents))
	zenq := &ZenQ[T]{contents: contents}
	zenq.selectFactory.waitList = NewThreadParker()
	go zenq.selectSender()
	// allow the above auxillary thread to manifest
	runtime.Gosched()
	return zenq
}

// Write writes a value to the queue
func (self *ZenQ[T]) Write(value T) {
	if atomic.LoadUint32(&self.globalState) > StateOpen {
		return
	}

	idx := (atomic.AddUint64(&self.writerIndex, 1) - 1) & indexMask
	slotState := &self.contents[idx].State
	writeParker := self.contents[idx].WriteParker

	// CAS -> change slot_state to busy if slot_state == empty
	for !atomic.CompareAndSwapUint32(slotState, SlotEmpty, SlotBusy) {
		switch atomic.LoadUint32(slotState) {
		case SlotBusy:
			runtime.Gosched()
		case SlotCommitted:
			writeParker.Park()
		case SlotEmpty:
			continue
		case SlotClosed:
			return
		}
	}
	self.contents[idx].Item = value
	atomic.StoreUint32(slotState, SlotCommitted)
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

	// CAS -> change slot_state to busy if slot_state == empty
	for !atomic.CompareAndSwapUint32(slotState, SlotEmpty, SlotBusy) {
		switch atomic.LoadUint32(slotState) {
		case SlotBusy:
			runtime.Gosched()
		case SlotCommitted:
			writeParker.Park()
		case SlotEmpty:
			continue
		case SlotClosed:
			return
		}
	}
	// Closing commit
	atomic.StoreUint32(slotState, SlotClosed)
}

// CloseAsync closes the channel asynchronously
// Useful when an user wants to close the channel from a reader end without blocking the thread
func (self *ZenQ[T]) CloseAsync() {
	go self.Close()
}

// Signal is the mechanism by which a selector notifies this ZenQ's auxillary thread to contest for the selection
func (self *ZenQ[T]) Signal() uint8 {
	if !atomic.CompareAndSwapUint32(&self.selectFactory.state, SelectionOpen, SelectionRunning) {
		return 0
	}
	safe_ready(self.selectFactory.auxThread)
	return 1
}

// EnqueueSelector pushes a calling selector to this ZenQ's selector waitlist
func (self *ZenQ[T]) EnqueueSelector(sel *Selection) {
	self.selectFactory.waitList.Enqueue(unsafe.Pointer(sel))
}

// IsClosed returns whether the zenq is closed for both reads and writes
func (self *ZenQ[T]) IsClosed() bool {
	return atomic.LoadUint32(&self.globalState) == StateFullyClosed
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
	fmt.Printf("writerIndex: %3d, readerIndex: %3d\n contents:-\n\n", self.writerIndex, self.readerIndex)
	for index := range self.contents {
		fmt.Printf("%5v : State -> %5v, Item -> %5v\n", index, self.contents[index].State, self.contents[index].Item)
	}
}

// selectSender is an auxillary thread which remains parked by default
// only when a selector sends a signal, it is notified and tries to send back to the selector
// if it fails, then it parks again and waits for another signal from another selection process
// since it is parked most of the times, it consumes minimal cpu time making the selection process efficient
func (self *ZenQ[T]) selectSender() {
	atomic.StorePointer(&self.selectFactory.auxThread, GetG())
	readState := false
	var data T
	var queueOpen bool

	for {
		// park by default and wait for Signal() notification from a selection process
		mcall(fast_park)
		if !readState {
			data, queueOpen = self.Read()
			readState = true
		}

	selector_dequeue:
		for {
			// keep dequeuing selectors from waitlist and try to acquire one
			// if acquired write to selector, ready it and go back to parking state
			if s := self.selectFactory.waitList.Dequeue(); s != nil {
				sel := (*Selection)(s)
				if selThread := atomic.SwapPointer(sel.ThreadPtr, nil); selThread != nil {
					// implementaion of sending from closed channel to selector mechanism
					if !queueOpen {
						// Signal to the selector that this queue is closed
						if sel.SignalQueueClosure() {
							// unblock the selector thread if all queues are closed so that it returns nil, false
							safe_ready(selThread)
						}
						// lazily decrement reference count until it is finally collected by the memory pool
						sel.DecrementReferenceCount()
						continue
					}
					// write to the selector
					sel.Data = data
					// notify selector
					safe_ready(selThread)
					readState = false
					sel.DecrementReferenceCount()
					break selector_dequeue
				} else {
					sel.DecrementReferenceCount()
					continue
				}
			} else {
				break selector_dequeue
			}
		}
		atomic.StoreUint32(&self.selectFactory.state, SelectionOpen)
	}
}

// returns a default value of any type
func getDefault[T any]() T {
	return *new(T)
}
