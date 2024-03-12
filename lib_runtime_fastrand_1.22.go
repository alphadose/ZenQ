//go:build go1.22

package zenq

import (
	_ "unsafe"
)

//go:linkname Fastrand runtime.cheaprand
func Fastrand() uint32
