package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

type Selection struct {
	Lock           uint32
	ThreadPtr      unsafe.Pointer
	referenceCount int64
	collectorPool  *sync.Pool
	Data           any
}

func (sel *Selection) IncrementReferenceCount() {
	atomic.AddInt64(&sel.referenceCount, 1)
}

func (sel *Selection) DecrementReferenceCount() {
	if atomic.AddInt64(&sel.referenceCount, -1) == 0 {
		sel.collectorPool.Put(sel)
	}
}

var selectionPool = &sync.Pool{}

func init() {
	selectionPool.New = func() any { return &Selection{collectorPool: selectionPool} }
}

// Selectable is an an interface for getting selected among many others
type Selectable interface {
	SelectRead(*Selection)
}

// Select selects a single element out of multiple ZenQs
// the second parameter tells if all ZenQs were closed or not before reading, in which case the data returned is nil
// If no ZenQ acquires this selector's lock then all selectable ZenQs are closed
func Select(streams ...Selectable) (data any, ok bool) {
	sel := selectionPool.Get().(*Selection)
	sel.ThreadPtr, sel.Data, sel.referenceCount, sel.Lock = GetG(), nil, 1, 0
	defer sel.DecrementReferenceCount()
	// race for reads
	for _, stream := range streams {
		go stream.SelectRead(sel)
	}
	// park and wait for notification
	mcall(fast_park)
	return sel.Data, atomic.LoadUint32(&sel.Lock) == 1 // lock == 0 means all queues were closed hence no read possible
}
