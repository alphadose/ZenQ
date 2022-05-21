package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

type Selection struct {
	Lock      uint32
	ThreadPtr unsafe.Pointer
	Data      any
}

var selectionPool = sync.Pool{New: func() any { return new(Selection) }}

// Selectable is an an interface for getting selected among many others
type Selectable interface {
	SelectRead(*Selection)
}

// Select selects a single element out of multiple ZenQs
// the second parameter tells if all ZenQs were closed or not before reading, in which case the data returned is nil
// If no ZenQ acquires this selector's lock then all selectable ZenQs are closed
func Select(streams ...Selectable) (data any, ok bool) {
	sel := selectionPool.Get().(*Selection)
	// defer selectionPool.Put(sel)
	sel.ThreadPtr, sel.Data, sel.Lock = GetG(), nil, 0
	// race for reads
	for _, stream := range streams {
		go stream.SelectRead(sel)
	}
	// park and wait for notification
	mcall(fast_park)
	return sel.Data, atomic.LoadUint32(&sel.Lock) == 1
}
