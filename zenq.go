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
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	power = 12

	// The queue size, should be a power of 2
	queueSize = 1 << power

	// Masking is faster than division, only works with numbers which are powers of 2
	indexMask = queueSize - 1

	// add this to uint64 to achieve the same thing as -1 to int64
	uint64SubtractionConstant = 1<<64 - 1
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
		WriteParker *ThreadParker[T]
		Item        T
	}

	SelectFactory struct {
		auxThread unsafe.Pointer
		state     uint32
		waitList  *List
		backlog   unsafe.Pointer
	}

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
	// memory pool for storing and leasing parking spots for goroutines
	parkPool := sync.Pool{New: func() any { return new(parkSpot[T]) }}
	for idx := range contents {
		contents[idx].WriteParker = NewThreadParker[T](&parkPool)
	}
	zenq := &ZenQ[T]{contents: contents}
	zenq.selectFactory.waitList = NewList()
	go zenq.selectSender()
	// allow the above auxillary thread to manifest
	mcall(gosched_m)
	return zenq
}

// Write writes a value to the queue
// It returns whether the queue is currently open for writes or not
// If not then it might be still open for reads, which can be checked by calling zenq.IsClosed()
func (self *ZenQ[T]) Write(value T) (queueClosedForWrites bool) {
	if atomic.LoadUint32(&self.globalState) > StateOpen {
		queueClosedForWrites = true
		return
	}

	// Try to send directly to selector when possible or else just dequeue unselected references
	// in order to reduce the burden on the auxillary thread and save cpu time
direct_send:
	if s := self.selectFactory.waitList.Dequeue(); s != nil {
		sel := (*Selection)(s)
		if selThread := atomic.SwapPointer(sel.ThreadPtr, nil); selThread != nil {
			if self.IsClosed() {
				if sel.SignalQueueClosure() {
					safe_ready(selThread)
				}
				sel.DecrementReferenceCount()
				return
			}
			// direct send to selector
			sel.Data = value
			// notify selector
			safe_ready(selThread)
			sel.DecrementReferenceCount()
			return
		}
		sel.DecrementReferenceCount()
		goto direct_send
	}

	slot := &self.contents[(atomic.AddUint64(&self.writerIndex, 1)-1)&indexMask]

	// CAS -> change slot_state to busy if slot_state == empty
	for !atomic.CompareAndSwapUint32(&slot.State, SlotEmpty, SlotBusy) {
		switch atomic.LoadUint32(&slot.State) {
		case SlotBusy:
			wait()
		case SlotCommitted:
			slot.WriteParker.Park(value)
			return
		case SlotEmpty:
			continue
		case SlotClosed:
			return
		}
	}
	slot.Item = value
	atomic.StoreUint32(&slot.State, SlotCommitted)
	return
}

// Read reads a value from the queue, you can once read once per object
func (self *ZenQ[T]) Read() (data T, queueOpen bool) {
	slot := &self.contents[(atomic.AddUint64(&self.readerIndex, 1)-1)&indexMask]

	// CAS -> change slot_state to busy if slot_state == committed
	for !atomic.CompareAndSwapUint32(&slot.State, SlotCommitted, SlotBusy) {
		switch atomic.LoadUint32(&slot.State) {
		case SlotBusy:
			wait()
		case SlotEmpty:
			if data, queueOpen = slot.WriteParker.Ready(); queueOpen {
				return
			} else if atomic.LoadUint32(&self.globalState) != StateFullyClosed {
				wait()
			} else {
				// queue is closed, rollback the reader index by 1
				atomic.AddUint64(&self.readerIndex, uint64SubtractionConstant)
				return
			}
		case SlotClosed:
			if atomic.CompareAndSwapUint32(&slot.State, SlotClosed, SlotEmpty) {
				atomic.StoreUint32(&self.globalState, StateFullyClosed)
			}
			return
		case SlotCommitted:
			continue
		}
	}
	data, queueOpen = slot.Item, true
	atomic.StoreUint32(&slot.State, SlotEmpty)
	return
}

// Close closes the ZenQ for further writes
// You can only read uptill the last committed write after closing
// This function will be blocking in case the queue is full
// ZenQ is closed from a writer goroutine by design, hence it should always be called
// from a writer goroutine and never from a reader goroutine which might cause the reader to get blocked and hence deadlock
// It returns if the queue was already closed for writes or not
func (self *ZenQ[T]) Close() (alreadyClosedForWrites bool) {
	// This ensures a ZenQ is closed only once even if this function is called multiple times making this operation safe
	if !atomic.CompareAndSwapUint32(&self.globalState, StateOpen, StateClosedForWrites) {
		alreadyClosedForWrites = true
		return
	}
	slot := &self.contents[(atomic.AddUint64(&self.writerIndex, 1)-1)&indexMask]

	// CAS -> change slot_state to busy if slot_state == empty
	for !atomic.CompareAndSwapUint32(&slot.State, SlotEmpty, SlotBusy) {
		switch atomic.LoadUint32(&slot.State) {
		case SlotBusy, SlotCommitted:
			mcall(gosched_m)
		case SlotEmpty:
			continue
		case SlotClosed:
			return
		}
	}
	// Closing commit
	atomic.StoreUint32(&slot.State, SlotClosed)
	return
}

// CloseAsync closes the channel asynchronously
// Useful when an user wants to close the channel from a reader end without blocking the thread
func (self *ZenQ[T]) CloseAsync() {
	go self.Close()
}

// The following 4 functions below implement the Selectable interface

// ReadFromBackLog tries to read a data from backlog if available
func (self *ZenQ[T]) ReadFromBackLog() (data any, ok bool) {
	if d := atomic.SwapPointer(&self.selectFactory.backlog, nil); d != nil {
		data, ok = *((*T)(d)), true
	}
	return
}

// Signal is the mechanism by which a selector notifies this ZenQ's auxillary thread to contest for the selection
func (self *ZenQ[T]) Signal() (sig uint8) {
	if !atomic.CompareAndSwapUint32(&self.selectFactory.state, SelectionOpen, SelectionRunning) {
		return
	}
	safe_ready(self.selectFactory.auxThread)
	sig = 1
	return
}

// EnqueueSelector pushes a calling selector to this ZenQ's selector waitlist
func (self *ZenQ[T]) EnqueueSelector(sel *Selection) {
	self.selectFactory.waitList.Enqueue(unsafe.Pointer(sel))
}

// IsClosed returns whether the zenq is closed for both reads and writes
func (self *ZenQ[T]) IsClosed() (closed bool) {
	closed = atomic.LoadUint32(&self.globalState) == StateFullyClosed
	return
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
	atomic.StoreUint32(&self.globalState, StateOpen)
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
	var data T
	var sel *Selection
	var s unsafe.Pointer
	readState, queueOpen := false, true

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
			if s = self.selectFactory.waitList.Dequeue(); s != nil {
				sel = (*Selection)(s)
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
		// if not selected by any selector, commit data to backlog and wait for next signal
		// saves a lot of cpu time
		if readState && queueOpen {
			var i T = data
			atomic.StorePointer(&self.selectFactory.backlog, unsafe.Pointer(&i))
		}
		atomic.StoreUint32(&self.selectFactory.state, SelectionOpen)
	}
}
