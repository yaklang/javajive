//go:build (amd64 || arm64) && !tinygo

package jdecenv

import (
	"sync"
	"unsafe"
)

// getg is implemented in getg_*.s (Go TLS g pointer).
func getg() unsafe.Pointer

var (
	goidOnce   sync.Once
	goidOffset uintptr
)

func gid() uint64 {
	goidOnce.Do(func() {
		goidOffset = discoverGoidOffset()
	})
	if goidOffset == 0 {
		return gidFromStack()
	}
	return *(*uint64)(unsafe.Pointer(uintptr(getg()) + goidOffset))
}

func discoverGoidOffset() uintptr {
	want := gidFromStack()
	if want == 0 {
		return 0
	}
	gp := uintptr(getg())
	var hit uintptr
	n := 0
	for off := uintptr(8); off < 256; off += 8 {
		if *(*uint64)(unsafe.Pointer(gp + off)) == want {
			n++
			hit = off
		}
	}
	if n != 1 {
		return 0
	}
	ch := make(chan bool, 1)
	go func() {
		w := gidFromStack()
		got := *(*uint64)(unsafe.Pointer(uintptr(getg()) + hit))
		ch <- w != 0 && got == w && w != want
	}()
	if !<-ch {
		return 0
	}
	return hit
}
