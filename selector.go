package zenq

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

// global memory pool for holding selection objects
var selectionPool = &sync.Pool{}

func init() {
	selectionPool.New = func() any { return &Selection{collectorPool: selectionPool} }
}

// Selection is an object shared by a selector and its children ZenQs
// This object is used for selection notification
type Selection struct {
	ThreadPtr      *unsafe.Pointer
	Data           any
	numQueues      int64
	referenceCount int64
	collectorPool  *sync.Pool
}

// SignalQueueClosure signals the closure of one ZenQ to the selector thread
// it returns if all queues were closed or not in which case the calling thread must goready() the selector thread
func (sel *Selection) SignalQueueClosure() (allQueuesClosed bool) {
	return atomic.AddInt64(&sel.numQueues, -1) == 0
}

// AllQueuesClosed returns whether all the queues present in selection are closed or not
func (sel *Selection) AllQueuesClosed() bool {
	return atomic.LoadInt64(&sel.numQueues) == 0
}

// IncrementReferenceCount does exactly what it says
func (sel *Selection) IncrementReferenceCount() {
	atomic.AddInt64(&sel.referenceCount, 1)
}

// DecrementReferenceCount decrements the reference count by 1 and puts the object back into the pool if it reaches 0
func (sel *Selection) DecrementReferenceCount() {
	if atomic.AddInt64(&sel.referenceCount, -1) == 0 {
		sel.ThreadPtr, sel.Data, sel.numQueues = nil, nil, 0
		// reuse this object in another selection event thereby saving memory
		sel.collectorPool.Put(sel)
	}
}

// NewSelectionObject returns a selection object from the memory pool
func NewSelectionObject() *Selection {
	return selectionPool.Get().(*Selection)
}

// Selectable is an interface for getting selected among many others
type Selectable interface {
	IsClosed() bool
	EnqueueSelector(*Selection)
	Signal() uint8
}

// Select selects a single element out of multiple ZenQs
// the second parameter tells if all ZenQs were closed or not before reading, in which case the data returned is nil
func Select(streams ...Selectable) (data any, ok bool) {
	var waitq []Selectable
	for idx := range streams {
		if streams[idx] == nil || streams[idx].IsClosed() {
			continue
		}
		waitq = append(waitq, streams[idx])
	}
	numStreams := int64(len(waitq))
	if numStreams == 0 {
		return nil, false
	}
	sel := NewSelectionObject()
	g := GetG()

	sel.ThreadPtr, sel.Data, sel.numQueues, sel.referenceCount = &g, nil, numStreams, numStreams+1
	// defer sel.DecrementReferenceCount()

	var numSignals uint8 = 0

	for idx := range waitq {
		waitq[idx].EnqueueSelector(sel)
	}

	iter := 0
retry:
	for idx := range waitq {
		numSignals += waitq[idx].Signal()
	}

	// might cause deadlock without this case
	if numSignals == 0 && atomic.LoadPointer(&g) != nil {
		if runtime_canSpin(iter) && multicore {
			iter++
			runtime_doSpin()
		} else {
			runtime.Gosched()
		}
		goto retry
	}

	// park and wait for notification
	mcall(fast_park)

	data, ok = sel.Data, !sel.AllQueuesClosed()
	sel.DecrementReferenceCount()
	return
}
