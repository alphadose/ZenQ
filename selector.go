package zenq

import (
	"sync"
	"unsafe"
)

type Selection struct {
	ThreadPtr *unsafe.Pointer
	Data      any
}

var selectionPool = sync.Pool{New: func() any { return new(Selection) }}

// Selectable is an an interface for getting selected among many others
type Selectable interface {
	SelectRead(*Selection)
}

// Select selects a single element out of multiple ZenQs
// the second parameter tells if all ZenQs were closed or not before reading, in which case the data returned is nil
func Select(streams ...Selectable) (data any, ok bool) {
	sel := selectionPool.Get().(*Selection)
	defer selectionPool.Put(sel)
	g := GetG()
	sel.ThreadPtr = &g
	sel.Data = nil
	// race for reads
	for _, stream := range streams {
		go stream.SelectRead(sel)
	}
	// park and wait for notification
	mcall(fast_park)
	return sel.Data, sel.Data != nil
}
