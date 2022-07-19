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
	numQueues      int64
	referenceCount int64
	free           func(any)
}

// SignalQueueClosure signals the closure of one ZenQ to the selector thread
// it returns if all queues were closed or not in which case the calling thread must goready() the selector thread
func (sel *Selection) SignalQueueClosure() (allQueuesClosed bool) {
	allQueuesClosed = atomic.AddInt64(&sel.numQueues, -1) == 0
	return
}

// AllQueuesClosed returns whether all the queues present in selection are closed or not
func (sel *Selection) AllQueuesClosed() (allQueuesClosed bool) {
	allQueuesClosed = atomic.LoadInt64(&sel.numQueues) == 0
	return
}

// IncrementReferenceCount does exactly what it says
func (sel *Selection) IncrementReferenceCount() {
	atomic.AddInt64(&sel.referenceCount, 1)
}

// DecrementReferenceCount decrements the reference count by 1 and puts the object back into the pool if it reaches 0
func (sel *Selection) DecrementReferenceCount() {
	if atomic.AddInt64(&sel.referenceCount, -1) == 0 {
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
	waitq, numStreams := make([]Selectable, len(streams), len(streams)), uint32(0)
	for idx := 0; idx < len(streams); idx++ {
		if streams[idx] == nil || streams[idx].IsClosed() {
			continue
		}
		waitq[numStreams] = streams[idx]
		numStreams++
	}
	if numStreams == 0 {
		return
	}

	// best case - optimistic first pass
	for idx := uint32(0); idx < numStreams; idx++ {
		if data, ok = waitq[idx].ReadFromBackLog(); ok {
			return
		}
	}

	// shuffle the queue to avoid deterministic starvation
	for i, j := uint32(0), uint32(0); i < numStreams; i, j = i+1, fastrandn(i+1) {
		waitq[i], waitq[j] = waitq[j], waitq[i]
	}

	sel, g, numSignals, iter := selectionGet().(*Selection), GetG(), uint8(0), 0

	sel.ThreadPtr, sel.Data, sel.numQueues, sel.referenceCount = &g, nil, int64(numStreams), int64(numStreams+1)

	for idx := uint32(0); idx < numStreams; idx++ {
		waitq[idx].EnqueueSelector(sel)
	}

retry:
	for idx := uint32(0); idx < numStreams; idx++ {
		numSignals += waitq[idx].Signal()
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
