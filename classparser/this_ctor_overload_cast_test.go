package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestThisCtorOverloadCastIsLoadBearing pins thisCtorOverloadArgCast.
// An LRU-taking constructor forwards to a Lookup-taking overload via bytecode
// `invokespecial <init>(Lookup)`. Rendered as `this(var1)` javac binds the more
// specific LRU overload ("recursive constructor invocation"). Real hit: jackson
// TypeFactory(LRUMap) -> TypeFactory(LookupCache). Kill-switch:
// JDEC_THIS_CTOR_OVERLOAD_CAST_OFF.
func TestThisCtorOverloadCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TypeFactory.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_THIS_CTOR_OVERLOAD_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "this((LookupCache)(var1))") {
		t.Errorf("fix ON: expected this((LookupCache)(var1)), got snippet:\n%s", snippetThis(on))
	}

	t.Setenv("JDEC_THIS_CTOR_OVERLOAD_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "this((LookupCache)(var1))") {
		t.Errorf("fix OFF: expected recursive this(var1), got snippet:\n%s", snippetThis(off))
	}
}

func snippetThis(src string) string {
	var b strings.Builder
	for _, ln := range strings.Split(src, "\n") {
		if strings.Contains(ln, "this(") {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	s := b.String()
	if len(s) > 800 {
		return s[:800]
	}
	return s
}
