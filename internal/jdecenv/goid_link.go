package jdecenv

// Goid identifies the current goroutine. On amd64 the runtime g pointer is a
// stable opaque identity for the lifetime of the goroutine; other platforms
// fall back to parsing runtime.Stack. Callers only compare the value and must
// not treat it as the runtime's numeric goroutine id.
func Goid() uint64 { return gid() }

func gid() uint64 {
	if g := currentG(); g != nil {
		return uint64(uintptr(g))
	}
	return gidFromStack()
}
