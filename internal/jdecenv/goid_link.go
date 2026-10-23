package jdecenv

import (
	"sync"
	"unsafe"
)

var (
	goidOnce   sync.Once
	goidOffset uintptr
	goidFastOK bool
)

// gid identifies this goroutine. A bound jdecenv frame used to call
// runtime.Stack on every Get; that traceback was most of ekp decompile CPU.
// The goid field offset is measured once, then each lookup is a pointer read.
// Goid is the current goroutine id. Callers use it to keep per-goroutine
// walk depth without formatting a stack trace.
func Goid() uint64 { return gid() }

func gid() uint64 {
	goidOnce.Do(calibrateGoidOffset)
	if goidFastOK {
		if g := currentG(); g != nil {
			return *(*uint64)(unsafe.Add(g, goidOffset))
		}
	}
	return gidFromStack()
}

func calibrateGoidOffset() {
	for attempt := 0; attempt < 8; attempt++ {
		found := make(chan uintptr, 1)
		go func() {
			want := gidFromStack()
			g := currentG()
			var off uintptr
			matches := 0
			if g != nil && want > 1 {
				for cand := uintptr(0); cand < 2048; cand += unsafe.Sizeof(uint64(0)) {
					if *(*uint64)(unsafe.Add(g, cand)) == want {
						off = cand
						matches++
					}
				}
			}
			if matches == 1 {
				found <- off
				return
			}
			found <- ^uintptr(0)
		}()
		off := <-found
		if off != ^uintptr(0) {
			goidOffset = off
			goidFastOK = true
			return
		}
	}
}
