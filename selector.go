package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// global memory pool for holding selection objects
var (
	selectionPool = sync.Pool{}
	selectionGet  = selectionPool.Get
)

func init() {
	selectionPool.New = func() any { return &Selection{free: selectionPool.Put} }
}

// Selection is an object shared by a selector and its children ZenQs
// This object is used for selection notification
type Selection struct {
	ThreadPtr      *unsafe.Pointer
	Data           any
	numQueues      int32
	referenceCount int32
	free           func(any)
}

// SignalQueueClosure signals the closure of one ZenQ to the selector thread
// it returns if all queues were closed or not in which case the calling thread must goready() the selector thread
func (sel *Selection) SignalQueueClosure() bool {
	return atomic.AddInt32(&sel.numQueues, -1) == 0
}

// AllQueuesClosed returns whether all the queues present in selection are closed or not
func (sel *Selection) AllQueuesClosed() bool {
	return atomic.LoadInt32(&sel.numQueues) == 0
}

// IncrementReferenceCount does exactly what it says
func (sel *Selection) IncrementReferenceCount() {
	atomic.AddInt32(&sel.referenceCount, 1)
}

// DecrementReferenceCount decrements the reference count by 1 and puts the object back into the pool if it reaches 0
func (sel *Selection) DecrementReferenceCount() {
	if atomic.AddInt32(&sel.referenceCount, -1) == 0 {
		sel.ThreadPtr, sel.Data = nil, nil
		// reuse this object in another selection event thereby saving memory
		sel.free(sel)
	}
}

// Selectable is an interface for getting selected among many others
type Selectable interface {
	IsClosed() bool
	EnqueueSelector(*Selection)
	ReadFromBackLog() (data any, ok bool)
	Signal() uint8
}

// Select selects a single element out of multiple ZenQs
// the second parameter tells if all ZenQs were closed or not before reading, in which case the data returned is nil
func Select(streams ...Selectable) (data any, ok bool) {
	var idx, numStreams int32 = 0, int32(len(streams) - 1)
filter_shuffle:
	for ; idx < numStreams; idx++ {
		if streams[idx] == nil || streams[idx].IsClosed() {
			for ; numStreams >= 0 && (streams[numStreams] == nil || streams[numStreams].IsClosed()); numStreams-- {
			}
			if idx >= numStreams {
				break filter_shuffle
			}
			streams[idx], streams[numStreams] = streams[numStreams], streams[idx]
			numStreams--
		}
	}
	if numStreams < 0 {
		ok = false
		return
	}

	for idx = 0; idx <= numStreams; idx++ {
		if data, ok = streams[idx].ReadFromBackLog(); ok {
			return
		}
	}

	sel, g, numSignals, iter := selectionGet().(*Selection), GetG(), uint8(0), 0

	sel.ThreadPtr, sel.Data, sel.numQueues, sel.referenceCount = &g, nil, numStreams+1, numStreams+2

	for idx = 0; idx <= numStreams; idx++ {
		streams[idx].EnqueueSelector(sel)
	}

retry:
	for idx = 0; idx <= numStreams; idx++ {
		numSignals += streams[idx].Signal()
	}

	// might cause deadlock without this case
	if numSignals == 0 && atomic.LoadPointer(&g) != nil {
		// wait for some ZenQ to acquire this selector's thread
		if runtime_canSpin(iter) {
			iter++
			runtime_doSpin()
		} else {
			mcall(gosched_m)
		}
		// if still no one has acquired this thread's reference then its dangerous to park
		// retry and signal all queues
		if atomic.LoadPointer(&g) != nil {
			goto retry
		}
	}

	// park and wait for notification
	mcall(fast_park)

	data, ok = sel.Data, !sel.AllQueuesClosed()
	sel.DecrementReferenceCount()
	return
}
