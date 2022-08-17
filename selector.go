package zenq

import (
	"sync/atomic"
	"unsafe"
)

// Selection is an object shared by a selector and its children ZenQs
// This object is used for selection notification
type Selection struct {
	ThreadPtr *unsafe.Pointer
	Data      *any
}

// Selectable is an interface for getting selected among many others
type Selectable interface {
	IsClosed() bool
	EnqueueSelector(*Selection)
	ReadFromBackLog() (data any)
	Signal() uint8
}

// Select selects a single element out of multiple ZenQs
// A maximum of 127 ZenQs can be selected from at a time owing to the size of int8 type
// `nil` is returned if all streams are closed or if a stream gets closed during the selection process
func Select(streams ...Selectable) (data any) {
	numStreams := int8(len(streams) - 1)
filter:
	for idx := int8(0); idx < numStreams; idx++ {
		if streams[idx] == nil || streams[idx].IsClosed() {
			for ; numStreams >= 0 && (streams[numStreams] == nil || streams[numStreams].IsClosed()); numStreams-- {
			}
			if idx >= numStreams {
				break filter
			}
			streams[idx], streams[numStreams] = streams[numStreams], streams[idx]
			numStreams--
		}
	}
	if numStreams < 0 {
		data = nil
		return
	}

	for idx := int8(0); idx <= numStreams; idx++ {
		if data = streams[idx].ReadFromBackLog(); data != nil {
			return
		}
	}

	g, numSignals, iter := GetG(), uint8(0), int8(0)

	sel := &Selection{ThreadPtr: &g, Data: &data}

	for idx := int8(0); idx <= numStreams; idx++ {
		streams[idx].EnqueueSelector(sel)
	}

retry:
	for idx := int8(0); idx <= numStreams; idx++ {
		numSignals += streams[idx].Signal()
	}

	// might cause deadlock without this case
	if numSignals == 0 && atomic.LoadPointer(&g) != nil {
		// wait for some ZenQ to acquire this selector's thread
		if runtime_canSpin(int(iter)) {
			iter++
			spin(30)
		} else {
			mcall(gosched_m)
		}
		goto retry
	}

	// park and wait for notification
	mcall(fast_park)
	return
}
