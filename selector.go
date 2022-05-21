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
func Select(streams ...Selectable) any {
	sel := selectionPool.Get().(*Selection)
	defer selectionPool.Put(sel)
	g := GetG()
	sel.ThreadPtr = &g
	// race for reads
	for _, stream := range streams {
		go stream.SelectRead(sel)
	}
	// wait for notification
	mcall(fast_park)
	return sel.Data
}
