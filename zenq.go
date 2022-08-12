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
	// a single slot in the queue
	slot[T any] struct {
		atomic.Uint32
		writeParker *ThreadParker[T]
		item        T
	}

	// metadata of the queue
	metaQ struct {
		globalState uint8
		// NOTE->self: strideLength and indexMask can be further optimized to uint8 for specialized ZenQs
		// with known data types instead of generic type
		// using variables with lower sizes decreases memory bandwidth consumption and increases speed
		strideLength uint16
		indexMask    uint16
		contents     unsafe.Pointer
		// memory pool refs for storing and leasing parking spots for goroutines
		alloc func() any
		free  func(any)
	}

	// container for the selection events among multiple queues
	selectFactory[T any] struct {
		selectionState atomic.Uint32
		auxThread      unsafe.Pointer
		backlog        atomic.Pointer[T]
		waitList       List
	}

	// ZenQ is the CPU cache optimized ringbuffer implementation
	ZenQ[T any] struct {
		// The padding members 0 to 4 below are here to ensure each item is on a separate cache line.
		// This prevents false sharing and hence improves performance.
		_p0         cacheLinePadding
		writerIndex atomic.Uint32
		_p1         [constants.CacheLinePadSize - unsafe.Sizeof(atomic.Uint32{})]byte
		readerIndex atomic.Uint32
		_p2         [constants.CacheLinePadSize - unsafe.Sizeof(atomic.Uint32{})]byte
		metaQ
		_p3 [constants.CacheLinePadSize - unsafe.Sizeof(metaQ{})]byte
		selectFactory[T]
		_p4 [constants.CacheLinePadSize - unsafe.Sizeof(selectFactory[T]{})]byte
	}
)

// returns the next greater power of 2 relative to val
func nextGreaterPowerOf2(val uint32) uint32 {
	return 1 << uint32(math.Min(math.Ceil(Fastlog2(math.Max(float64(val), 1))), 16))
}

// New returns a new queue given its payload type passed as a generic parameter
func New[T any](size uint32) *ZenQ[T] {
	var (
		queueSize = nextGreaterPowerOf2(size)
		contents  = make([]slot[T], queueSize, queueSize)
		parkPool  = sync.Pool{New: func() any { return new(parkSpot[T]) }}
	)
	for idx := uint32(0); idx < queueSize; idx++ {
		spot := parkPool.Get().(*parkSpot[T])
		spot.threadPtr = nil
		contents[idx].writeParker = NewThreadParker(spot)
	}
	zenq := &ZenQ[T]{
		metaQ: metaQ{
			strideLength: uint16(unsafe.Sizeof(slot[T]{})),
			contents:     unsafe.Pointer(&contents[0]),
			alloc:        parkPool.Get,
			free:         parkPool.Put,
			indexMask:    uint16(queueSize - 1),
		},
		selectFactory: selectFactory[T]{waitList: NewList()},
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
	if Load8(&self.globalState) != StateOpen {
		queueClosedForWrites = true
		return
	}

	// Try to send directly to selector when possible or else just dequeue unselected references
	// in order to reduce the burden on the auxillary thread and save cpu time
direct_send:
	if s := self.waitList.Dequeue(); s != nil {
		sel := (*Selection)(s)
		if selThread := atomic.SwapPointer(sel.ThreadPtr, nil); selThread != nil {
			if !self.IsClosed() {
				// direct send to selector
				*sel.Data = value
			} else {
				*sel.Data = nil
			}
			// notify selector
			safe_ready(selThread)
			sel.DecrementReferenceCount()
			return
		}
		sel.DecrementReferenceCount()
		goto direct_send
	}

	slot := (*slot[T])(unsafe.Pointer(uintptr(self.strideLength)*(uintptr(self.indexMask)&uintptr(self.writerIndex.Add(1))) + uintptr(self.contents)))

	// CAS -> change slot_state to busy if slot_state == empty
	for !slot.CompareAndSwap(SlotEmpty, SlotBusy) {
		switch slot.Load() {
		case SlotBusy:
			wait()
		case SlotCommitted:
			n := self.alloc().(*parkSpot[T])
			n.threadPtr, n.value = GetG(), value
			n.next.Store(nil)
			slot.writeParker.Park(n)
			mcall(fast_park)
			return
		case SlotEmpty:
			continue
		case SlotClosed:
			return
		}
	}
	slot.item = value
	slot.Store(SlotCommitted)
	return
}

// Read reads a value from the queue, you can once read once per object
func (self *ZenQ[T]) Read() (data T, queueOpen bool) {
	slot := (*slot[T])(unsafe.Pointer(uintptr(self.strideLength)*(uintptr(self.indexMask)&uintptr(self.readerIndex.Add(1))) + uintptr(self.contents)))

	// CAS -> change slot_state to busy if slot_state == committed
	for !slot.CompareAndSwap(SlotCommitted, SlotBusy) {
		switch slot.Load() {
		case SlotBusy:
			wait()
		case SlotEmpty:
			var freeable *parkSpot[T]
			if data, queueOpen, freeable = slot.writeParker.Ready(); queueOpen {
				if freeable != nil {
					self.free(freeable)
				}
				return
			} else if Load8(&self.globalState) != StateFullyClosed {
				mcall(gosched_m)
			} else {
				// queue is closed, decrement the reader index by 1
				self.readerIndex.Add(math.MaxUint32)
				queueOpen = false
				return
			}
		case SlotClosed:
			if slot.CompareAndSwap(SlotClosed, SlotEmpty) {
				Store8(&self.globalState, StateFullyClosed)
			}
			queueOpen = false
			return
		case SlotCommitted:
			continue
		}
	}
	data, queueOpen = slot.item, true
	slot.Store(SlotEmpty)
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
	if Load8(&self.globalState) != StateOpen {
		alreadyClosedForWrites = true
		return
	}
	Store8(&self.globalState, StateClosedForWrites)
	slot := (*slot[T])(unsafe.Pointer(uintptr(self.strideLength)*(uintptr(self.indexMask)&uintptr(self.writerIndex.Add(1))) + uintptr(self.contents)))

	// CAS -> change slot_state to busy if slot_state == empty
	for !slot.CompareAndSwap(SlotEmpty, SlotBusy) {
		switch slot.Load() {
		case SlotBusy, SlotCommitted:
			mcall(gosched_m)
		case SlotEmpty:
			continue
		case SlotClosed:
			return
		}
	}
	// Closing commit
	slot.Store(SlotClosed)
	return
}

// CloseAsync closes the channel asynchronously
// Useful when an user wants to close the channel from a reader end without blocking the thread
func (self *ZenQ[T]) CloseAsync() {
	go self.Close()
}

// The following 4 functions below implement the Selectable interface

// ReadFromBackLog tries to read a data from backlog if available
func (self *ZenQ[T]) ReadFromBackLog() (data any) {
	if d := self.backlog.Swap(nil); d != nil {
		data = *((*T)(d))
	}
	return
}

// Signal is the mechanism by which a selector notifies this ZenQ's auxillary thread to contest for the selection
func (self *ZenQ[T]) Signal() uint8 {
	if !self.selectionState.CompareAndSwap(SelectionOpen, SelectionRunning) {
		return 0
	} else {
		safe_ready(self.auxThread)
		return 1
	}
}

// EnqueueSelector pushes a calling selector to this ZenQ's selector waitlist
func (self *ZenQ[T]) EnqueueSelector(sel *Selection) {
	self.waitList.Enqueue(sel)
}

// IsClosed returns whether the zenq is closed for both reads and writes
func (self *ZenQ[T]) IsClosed() bool {
	return Load8(&self.globalState) == StateFullyClosed
}

// Reset resets the queue state
// This also releases all parked goroutines if any and drains all committed writes
func (self *ZenQ[T]) Reset() {
	// Close() is blocking when queue is full hence execute it asynchronously
	self.CloseAsync()
	// drain entire queue
	for open := true; open; _, open = self.Read() {
	}
	Store8(&self.globalState, StateOpen)
}

// Dump dumps the current queue state
// Unsafe to be called from multiple goroutines
func (self *ZenQ[T]) Dump() {
	fmt.Printf("writerIndex: %3d, readerIndex: %3d\n contents:-\n\n", self.writerIndex, self.readerIndex)
	for idx := uintptr(0); idx <= uintptr(self.indexMask); idx++ {
		slot := (*slot[T])(unsafe.Pointer(uintptr(self.contents) + idx*unsafe.Sizeof(slot[T]{})))
		fmt.Printf("Slot -> %#v\n", *slot)
	}
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
					if queueOpen {
						// write to the selector
						*sel.Data = data
					} else {
						*sel.Data = nil
					}
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
			self.backlog.Store(&i)
		}
		self.selectionState.Store(SelectionOpen)
	}
}
