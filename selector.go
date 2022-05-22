package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	SelectionOpen = iota
	Selected
)

var selectionPool = &sync.Pool{}

func init() {
	selectionPool.New = func() any { return &Selection{collectorPool: selectionPool} }
}

type Selection struct {
	lock           uint32
	threadPtr      unsafe.Pointer
	referenceCount int64
	collectorPool  *sync.Pool
	data           any
}

func (sel *Selection) WriteAndSchedule(data any) {
	sel.data = data
	wait_until_parked(sel.threadPtr)
	goready(sel.threadPtr, 1)
}

func (sel *Selection) AcquireLock() bool {
	return atomic.CompareAndSwapUint32(&sel.lock, SelectionOpen, Selected)
}

func (sel *Selection) Selected() bool {
	return atomic.LoadUint32(&sel.lock) == Selected
}

func (sel *Selection) IncrementReferenceCount() {
	atomic.AddInt64(&sel.referenceCount, 1)
}

func (sel *Selection) DecrementReferenceCount() {
	if atomic.AddInt64(&sel.referenceCount, -1) == 0 {
		// println("kekraw")
		// sel.collectorPool.Put(sel)
	}
}

func NewSelectionObject() *Selection {
	return selectionPool.Get().(*Selection)
}

// Selectable is an an interface for getting selected among many others
type Selectable interface {
	SelectRead(*Selection)
}

// Select selects a single element out of multiple ZenQs
// the second parameter tells if all ZenQs were closed or not before reading, in which case the data returned is nil
// If no ZenQ acquires this selector's lock then all selectable ZenQs are closed
func Select(streams ...Selectable) (data any, ok bool) {
	if len(streams) == 0 {
		return nil, false
	}
	sel := NewSelectionObject()
	sel.threadPtr, sel.data, sel.referenceCount, sel.lock = GetG(), nil, int64(len(streams)), SelectionOpen
	// race for reads
	for _, stream := range streams {
		go stream.SelectRead(sel)
	}
	// park and wait for notification
	mcall(fast_park)
	return sel.data, sel.Selected() // lock == SelectionOpen means all queues were closed hence no read possible
}
