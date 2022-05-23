package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

var selectionPool = &sync.Pool{}

// func init() {
// 	selectionPool.New = func() any { return &Selection{collectorPool: selectionPool} }
// }

type Selection struct {
	ThreadPtr *unsafe.Pointer
	Data      any
	numQueues int64
}

// SignalQueueClosure signals the closure of one ZenQ to the selector thread
// it returns if all queues were closed or not in which case the calling thread must goready the selector thread
func (sel *Selection) SignalQueueClosure() (allQueuesClosed bool) {
	return atomic.AddInt64(&sel.numQueues, -1) == 0
}

func (sel *Selection) AllQueuesClosed() bool {
	return atomic.LoadInt64(&sel.numQueues) == 0
}

// func (sel *Selection) WriteAndSchedule(data any) {
// 	sel.data = data
// 	wait_until_parked(sel.threadPtr)
// 	goready(sel.threadPtr, 1)
// }

// func (sel *Selection) IncrementReferenceCount() {
// 	atomic.AddInt64(&sel.referenceCount, 1)
// }

// func (sel *Selection) DecrementReferenceCount() {
// 	if atomic.AddInt64(&sel.referenceCount, -1) == 0 {
// 		// println("kekraw")
// 		// sel.collectorPool.Put(sel)
// 	}
// }

func NewSelectionObject() *Selection {
	return new(Selection)
	// return selectionPool.Get().(*Selection)
}

// Selectable is an an interface for getting selected among many others
type Selectable interface {
	OpenSelection()
	IsClosed() bool
	EnqueueSelector(*Selection)
	Signal()
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
	for idx := range waitq {
		waitq[idx].OpenSelection()
	}
	sel := NewSelectionObject()
	g := GetG()
	sel.ThreadPtr, sel.Data, sel.numQueues = &g, nil, int64(len(waitq))
	for idx := range waitq {
		waitq[idx].EnqueueSelector(sel)
	}
	for idx := range waitq {
		waitq[idx].Signal()
	}
	// park and wait for notification
	mcall(fast_park)
	return sel.Data, !sel.AllQueuesClosed()
}
