package javaclassparser

// 承重测试: UnmodifiableBoundedCollection 构造器 `super(BoundedCollection<? extends E>)`
// 喂给 AbstractCollectionDecorator(Collection<E>) 时擦除不同, 仍需 raw `(Collection)` 造型。
// kill-switch: JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestUnmodifiableBoundedCollectionSuperCastIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/UnmodifiableBoundedCollection.class")
	if err != nil {
		t.Fatalf("read UnmodifiableBoundedCollection: %v", err)
	}
	base, err := os.ReadFile("testdata/regression/AbstractCollectionDecorator.class")
	if err != nil {
		t.Fatalf("read AbstractCollectionDecorator: %v", err)
	}
	bc, err := os.ReadFile("testdata/regression/BoundedCollection.class")
	if err != nil {
		t.Fatalf("read BoundedCollection: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		switch {
		case strings.HasSuffix(internalName, "UnmodifiableBoundedCollection"):
			return seed, true
		case strings.HasSuffix(internalName, "AbstractCollectionDecorator"):
			return base, true
		case strings.HasSuffix(internalName, "BoundedCollection"):
			return bc, true
		default:
			return nil, false
		}
	}

	os.Unsetenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF")
	on, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "super((Collection)(var1))") && !strings.Contains(on, "super((Collection) (var1))") {
		t.Errorf("fix ON: expected super((Collection)(var1)), got:\n%s", on)
	}

	t.Setenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF", "1")
	off, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "super((Collection)(var1))") || strings.Contains(off, "super((Collection) (var1))") {
		t.Errorf("fix OFF: expected bare super(var1), got:\n%s", off)
	}
}
