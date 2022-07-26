// A minimalist thread-safe queue implemented using a lock-free ringbuffer which is faster
// and has lower memory allocations than golang's native channels
// Based on the LMAX disruptor pattern https://lmax-exchange.github.io/disruptor/disruptor.html

// Known Limitations:-
//
// 1. Max queue_size = 2^31
// 2. The queue_size is a power of 2, in case a different size is provided then queue_size is rounded up to the next greater power of 2

// Suggestions:-
//
// 1. Use runtime.LockOSThread() on the goroutine calling ZenQ.Read() for lowest latency provided you have > 1 cpu cores
//

package zenq

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/alphadose/zenq/v2/constants"
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
	slot[T any] struct {
		State       uint32
		WriteParker *ThreadParker[T]
		Item        T
	}

	selectFactory struct {
		state     uint32
		auxThread unsafe.Pointer
		backlog   unsafe.Pointer
		waitList  List
	}

	// ZenQ is the CPU cache optimized ringbuffer implementation
	ZenQ[T any] struct {
		// The padding members 0 to 4 below are here to ensure each item is on a separate cache line.
		// This prevents false sharing and hence improves performance.
		_p0          cacheLinePadding
		writerIndex  uint32
		_p1          [constants.CacheLinePadSize - unsafe.Sizeof(uint32(0))]byte
		readerIndex  uint32
		_p2          [constants.CacheLinePadSize - unsafe.Sizeof(uint32(0))]byte
		globalState  uint32
		indexMask    uint32
		strideLength uintptr
		contents     unsafe.Pointer
		// memory pool refs for storing and leasing parking spots for goroutines
		alloc func() any
		free  func(any)
		_p3   [constants.CacheLinePadSize - 2*unsafe.Sizeof(uint32(0)) - 2*unsafe.Sizeof(func() {}) - unsafe.Sizeof(unsafe.Pointer(nil)) - unsafe.Sizeof(uintptr(0))]byte
		selectFactory
		_p4 [constants.CacheLinePadSize - unsafe.Sizeof(selectFactory{})]byte
	}
)

// returns the next greater power of 2 relative to val
func nextGreaterPowerOf2(val uint32) uint32 {
	return 1 << uint32(math.Min(math.Ceil(Fastlog2(math.Max(float64(val), 1))), 31))
}

// New returns a new queue given its payload type passed as a generic parameter
func New[T any](size uint32) *ZenQ[T] {
	var (
		queueSize uint32 = nextGreaterPowerOf2(size)
		contents         = make([]slot[T], queueSize, queueSize)
		parkPool         = sync.Pool{New: func() any { return new(parkSpot[T]) }}
	)
	for idx := uint32(0); idx < queueSize; idx++ {
		n := parkPool.Get().(*parkSpot[T])
		n.threadPtr, n.next = nil, nil
		contents[idx].WriteParker = NewThreadParker[T](unsafe.Pointer(n))
	}
	zenq := &ZenQ[T]{
		strideLength:  unsafe.Sizeof(slot[T]{}),
		contents:      unsafe.Pointer(&contents[0]),
		alloc:         parkPool.Get,
		free:          parkPool.Put,
		selectFactory: selectFactory{waitList: NewList()},
		indexMask:     queueSize - 1,
	}
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
	if s := self.waitList.Dequeue(); s != nil {
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

	slot := (*slot[T])(unsafe.Pointer(uintptr(atomic.AddUint32(&self.writerIndex, 1)&self.indexMask)*self.strideLength + uintptr(self.contents)))

	// CAS -> change slot_state to busy if slot_state == empty
	for !atomic.CompareAndSwapUint32(&slot.State, SlotEmpty, SlotBusy) {
		switch atomic.LoadUint32(&slot.State) {
		case SlotBusy:
			wait()
		case SlotCommitted:
			n := self.alloc().(*parkSpot[T])
			n.threadPtr, n.next, n.value = GetG(), nil, value
			slot.WriteParker.Park(unsafe.Pointer(n))
			mcall(fast_park)
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
	slot := (*slot[T])(unsafe.Pointer(uintptr(atomic.AddUint32(&self.readerIndex, 1)&self.indexMask)*self.strideLength + uintptr(self.contents)))

	// CAS -> change slot_state to busy if slot_state == committed
	for !atomic.CompareAndSwapUint32(&slot.State, SlotCommitted, SlotBusy) {
		switch atomic.LoadUint32(&slot.State) {
		case SlotBusy:
			wait()
		case SlotEmpty:
			var freeable *parkSpot[T]
			if data, queueOpen, freeable = slot.WriteParker.Ready(); queueOpen {
				if freeable != nil {
					self.free(freeable)
				}
				return
			} else if atomic.LoadUint32(&self.globalState) != StateFullyClosed {
				mcall(gosched_m)
			} else {
				// queue is closed, decrement the reader index by 1
				atomic.AddUint32(&self.readerIndex, math.MaxUint32)
				queueOpen = false
				return
			}
		case SlotClosed:
			if atomic.CompareAndSwapUint32(&slot.State, SlotClosed, SlotEmpty) {
				atomic.StoreUint32(&self.globalState, StateFullyClosed)
			}
			queueOpen = false
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
	slot := (*slot[T])(unsafe.Pointer(uintptr(atomic.AddUint32(&self.writerIndex, 1)&self.indexMask)*self.strideLength + uintptr(self.contents)))

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
	if d := atomic.SwapPointer(&self.backlog, nil); d != nil {
		data, ok = *((*T)(d)), true
	}
	return
}

// Signal is the mechanism by which a selector notifies this ZenQ's auxillary thread to contest for the selection
func (self *ZenQ[T]) Signal() uint8 {
	if !atomic.CompareAndSwapUint32(&self.state, SelectionOpen, SelectionRunning) {
		return 0
	}
	safe_ready(self.auxThread)
	return 1
}

// EnqueueSelector pushes a calling selector to this ZenQ's selector waitlist
func (self *ZenQ[T]) EnqueueSelector(sel *Selection) {
	self.waitList.Enqueue(sel)
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
	// drain entire queue
	for open := true; open; _, open = self.Read() {
	}
	atomic.StoreUint32(&self.globalState, StateOpen)
}

// Dump dumps the current queue state
// Unsafe to be called from multiple goroutines
func (self *ZenQ[T]) Dump() {
	fmt.Printf("writerIndex: %3d, readerIndex: %3d\n contents:-\n\n", self.writerIndex, self.readerIndex)
	// for idx := range self.contents {
	// 	fmt.Printf("%5v : State -> %5v, Item -> %5v\n", idx, self.contents[idx].State, self.contents[idx].Item)
	// }
}

// selectSender is an auxillary thread which remains parked by default
// only when a selector sends a signal, it is notified and tries to send back to the selector
// if it fails, then it parks again and waits for another signal from another selection process
// since it is parked most of the times, it consumes minimal cpu time making the selection process efficient
func (self *ZenQ[T]) selectSender() {
	atomic.StorePointer(&self.auxThread, GetG())
	var (
		data                 T
		sel                  *Selection
		threadPtr            unsafe.Pointer
		readState, queueOpen bool = false, true
	)

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
			if sel = self.waitList.Dequeue(); sel != nil {
				if threadPtr = atomic.SwapPointer(sel.ThreadPtr, nil); threadPtr != nil {
					// implementaion of sending from closed channel to selector mechanism
					if !queueOpen {
						// Signal to the selector that this queue is closed
						if sel.SignalQueueClosure() {
							// unblock the selector thread if all queues are closed so that it returns nil, false
							safe_ready(threadPtr)
						}
						// lazily decrement reference count until it is finally collected by the memory pool
						sel.DecrementReferenceCount()
						continue
					}
					// write to the selector
					sel.Data = data
					// notify selector
					safe_ready(threadPtr)
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
			atomic.StorePointer(&self.backlog, unsafe.Pointer(&i))
		}
		atomic.StoreUint32(&self.state, SelectionOpen)
	}
}
