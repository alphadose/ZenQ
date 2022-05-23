package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

var selectionPool = &sync.Pool{}

func init() {
	selectionPool.New = func() any { return &Selection{collectorPool: selectionPool} }
}

type Selection struct {
	ThreadPtr      *unsafe.Pointer
	Data           any
	numQueues      int64
	referenceCount int64
	collectorPool  *sync.Pool
}

// SignalQueueClosure signals the closure of one ZenQ to the selector thread
// it returns if all queues were closed or not in which case the calling thread must goready the selector thread
func (sel *Selection) SignalQueueClosure() (allQueuesClosed bool) {
	return atomic.AddInt64(&sel.numQueues, -1) == 0
}

func (sel *Selection) AllQueuesClosed() bool {
	return atomic.LoadInt64(&sel.numQueues) == 0
}

func (sel *Selection) IncrementReferenceCount() {
	atomic.AddInt64(&sel.referenceCount, 1)
}

func (sel *Selection) DecrementReferenceCount() {
	if atomic.AddInt64(&sel.referenceCount, -1) == 0 {
		sel.ThreadPtr, sel.Data, sel.numQueues = nil, nil, 0
		sel.collectorPool.Put(sel)
	}
}

func NewSelectionObject() *Selection {
	return selectionPool.Get().(*Selection)
}

// Selectable is an an interface for getting selected among many others
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
	if len(waitq) == 0 {
		return nil, false
	}
	sel := NewSelectionObject()
	g := GetG()

	sel.ThreadPtr, sel.Data, sel.numQueues, sel.referenceCount = &g, nil, int64(len(waitq)), int64(len(waitq))+1
	defer sel.DecrementReferenceCount()

	var numSignals uint8 = 0

	for idx := range waitq {
		waitq[idx].EnqueueSelector(sel)
	}

retry:
	for idx := range waitq {
		numSignals += waitq[idx].Signal()
	}

	// might cause deadlock without this case
	if numSignals == 0 && atomic.LoadPointer(&g) != nil {
		wait()
		goto retry
	}

	// park and wait for notification
	mcall(fast_park)
	return sel.Data, !sel.AllQueuesClosed()
}
