// Package jdecenv provides request-local JDEC_* lookup for package-level
// reconstructors that have no receiver.
//
// Nested Run calls push/pop a stack so an inner request cannot erase the outer
// snapshot (including on panic). Prefer ClassContext.Getenv / Decompiler.getenv
// (explicit policy pointers) on hot paths. Get is a last-resort ambient lookup.
//
// A new goroutine has an empty stack and must call Run with a captured snapshot
// (see Go and Current). wrapResolve re-enters Run with the request snapshot so
// resolver callbacks that nest DecompileWithOptions restore the caller.
package jdecenv

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
)

type stack []map[string]string

var byGid sync.Map // uint64 -> stack
var liveBinds int32

func hasBind() bool { return atomic.LoadInt32(&liveBinds) != 0 }

// Run pushes snap for this goroutine, runs fn, then pops (panic-safe).
// A nil snap does not push: the existing outer binding is left unchanged.
func Run(snap map[string]string, fn func() error) (err error) {
	if snap == nil {
		return fn()
	}
	id := gid()
	push(id, snap)
	defer pop(id)
	return fn()
}

// RunLive pushes a live-env sentinel (Get reads os.Getenv) for the duration of
// fn, then restores the previous frame. Used by nested legacy Dump/Decompile
// so they do not permanently clear an outer options request.
func RunLive(fn func() error) error {
	undo := EnterLive()
	defer undo()
	return fn()
}

// EnterLive pushes a live-env sentinel. The returned function pops it
// (panic-safe when deferred).
func EnterLive() func() {
	id := gid()
	push(id, nil)
	return func() { pop(id) }
}

// Current returns the innermost bound snapshot and whether a frame exists.
// A live-env sentinel is reported as (nil, true).
func Current() (map[string]string, bool) {
	if !hasBind() {
		return nil, false
	}
	id := gid()
	v, ok := byGid.Load(id)
	if !ok {
		return nil, false
	}
	st := v.(stack)
	if len(st) == 0 {
		return nil, false
	}
	return st[len(st)-1], true
}

// Get returns the innermost snapshot value. Missing keys are unset (closed).
// A live-env sentinel or no bind reads os.Getenv. Child goroutines do not
// inherit a bind; use Go or Run(Current()) explicitly.
func Get(key string) string {
	if !hasBind() {
		return os.Getenv(key)
	}
	if snap, ok := Current(); ok {
		if snap == nil {
			return os.Getenv(key)
		}
		return snap[key]
	}
	return os.Getenv(key)
}

// Go starts fn on a new goroutine with the current snapshot rebound.
func Go(fn func()) {
	snap, ok := Current()
	go func() {
		if !ok {
			fn()
			return
		}
		if snap == nil {
			_ = RunLive(func() error {
				fn()
				return nil
			})
			return
		}
		_ = Run(snap, func() error {
			fn()
			return nil
		})
	}()
}

func push(id uint64, snap map[string]string) {
	var st stack
	if v, ok := byGid.Load(id); ok {
		st = v.(stack)
	}
	n := make(stack, len(st)+1)
	copy(n, st)
	n[len(st)] = snap
	byGid.Store(id, n)
	atomic.AddInt32(&liveBinds, 1)
}

func pop(id uint64) {
	v, ok := byGid.Load(id)
	if !ok {
		return
	}
	st := v.(stack)
	if len(st) <= 1 {
		byGid.Delete(id)
		atomic.AddInt32(&liveBinds, -1)
		return
	}
	n := make(stack, len(st)-1)
	copy(n, st[:len(st)-1])
	byGid.Store(id, n)
	atomic.AddInt32(&liveBinds, -1)
}

// gid identifies this goroutine for the ambient snapshot stack. It parses
// runtime.Stack once per Get when a frame is bound. Hot reconstructors must
// use ClassContext.Getenv / Decompiler.getenv (explicit policy), not Get.
func gid() uint64 { return gidFromStack() }

func gidFromStack() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	i := 0
	for i < n && buf[i] != ' ' {
		i++
	}
	i++
	var id uint64
	for i < n {
		c := buf[i]
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
		i++
	}
	return id
}
