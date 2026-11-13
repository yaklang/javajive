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
	"strings"
	"sync"
	"sync/atomic"
)

type envFrame struct {
	snap map[string]string
	// keys are the JDEC_* entries this snapshot overrides. They are extracted
	// once on push so Get can answer the common "no override active" case
	// without parsing the goroutine stack.
	keys []string
}

type stack []envFrame

var byGid sync.Map // uint64 -> stack
var liveBinds int32

// activeKeys is the union of JDEC_* keys present in every live snapshot. When
// a queried key is absent from this set and the process environment has no
// value for it, every possible frame (closed snapshot, live sentinel, or no
// bind) returns the empty string, so Get can skip gid()/runtime.Stack.
//
// Updates happen only on Run/EnterLive push/pop, not on each Get. The map is
// published copy-on-write under activeMu and read lock-free via atomic.Value.
var activeMu sync.Mutex
var activeKeySet atomic.Value // map[string]struct{}

func init() {
	activeKeySet.Store(map[string]struct{}{})
}

func jdecKeys(snap map[string]string) []string {
	if len(snap) == 0 {
		return nil
	}
	var keys []string
	for key := range snap {
		if strings.HasPrefix(key, "JDEC_") {
			keys = append(keys, key)
		}
	}
	return keys
}

func addActiveKeys(keys []string) {
	if len(keys) == 0 {
		return
	}
	activeMu.Lock()
	defer activeMu.Unlock()
	set := activeKeySet.Load().(map[string]struct{})
	next := make(map[string]struct{}, len(set)+len(keys))
	for key := range set {
		next[key] = struct{}{}
	}
	for _, key := range keys {
		next[key] = struct{}{}
	}
	activeKeySet.Store(next)
}

// refreshActiveKeys recomputes the union of override keys from the live stacks
// after a pop. Pops happen at request boundaries, and the number of frames and
// override keys is small, so an exact rebuild is cheaper and simpler than
// refcounting keys across nested frames.
func refreshActiveKeys() {
	activeMu.Lock()
	defer activeMu.Unlock()
	surviving := map[string]struct{}{}
	byGid.Range(func(_, value any) bool {
		for _, frame := range value.(stack) {
			for _, key := range frame.keys {
				surviving[key] = struct{}{}
			}
		}
		return true
	})
	activeKeySet.Store(surviving)
}

func activeKey(key string) bool {
	set := activeKeySet.Load().(map[string]struct{})
	_, ok := set[key]
	return ok
}

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
	return st[len(st)-1].snap, true
}

// Get returns the innermost snapshot value. Missing keys are unset (closed).
// A live-env sentinel or no bind reads os.Getenv. Child goroutines do not
// inherit a bind; use Go or Run(Current()) explicitly.
func Get(key string) string {
	if !hasBind() {
		return os.Getenv(key)
	}
	// Fast path for the overwhelmingly common case: no active snapshot
	// overrides this key and the process environment does not define it.
	// Every possible frame (closed snapshot, live sentinel, or no binding)
	// then yields "", so gid()/runtime.Stack parsing is unnecessary. A key
	// that IS overridden still takes the full goroutine-local path.
	if strings.HasPrefix(key, "JDEC_") && !activeKey(key) && os.Getenv(key) == "" {
		return ""
	}
	if snap, ok := Current(); ok {
		if snap == nil {
			return os.Getenv(key)
		}
		return snap[key]
	}
	return os.Getenv(key)
}

// CaptureProcess copies the process environment once. A decompile worker
// passes it to Run so later Type() lookups read the map instead of os.Getenv.
// Unbound Lookup() stays live; this snapshot is taken at the start of one class.
func CaptureProcess() map[string]string {
	env := os.Environ()
	snap := make(map[string]string, len(env))
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		snap[kv[:i]] = kv[i+1:]
	}
	return snap
}

// Lookup captures the policy at a synchronous rendering/analysis boundary.
// Snapshot lookups stay closed to the host and to nested requests; an unbound
// or live-sentinel lookup continues to read the live process environment. Hot
// visitors can pass this function explicitly instead of rediscovering the
// goroutine binding by formatting its complete stack for each expression.
func Lookup() func(string) string {
	if snap, bound := Current(); bound && snap != nil {
		return func(key string) string { return snap[key] }
	}
	return os.Getenv
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
	frame := envFrame{snap: snap, keys: jdecKeys(snap)}
	n[len(st)] = frame
	byGid.Store(id, n)
	atomic.AddInt32(&liveBinds, 1)
	addActiveKeys(frame.keys)
}

func pop(id uint64) {
	v, ok := byGid.Load(id)
	if !ok {
		return
	}
	st := v.(stack)
	frame := st[len(st)-1]
	if len(st) <= 1 {
		byGid.Delete(id)
	} else {
		n := make(stack, len(st)-1)
		copy(n, st[:len(st)-1])
		byGid.Store(id, n)
	}
	atomic.AddInt32(&liveBinds, -1)
	if len(frame.keys) > 0 {
		refreshActiveKeys()
	}
}

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
