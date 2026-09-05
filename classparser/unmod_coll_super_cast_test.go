package javaclassparser

// 承重测试: UnmodifiableCollection 构造器 `super(Collection<? extends E>)` 喂给
// AbstractCollectionDecorator(Collection<E>) 时需 raw `(Collection)` 造型。
// javac 把通配符捕获为 CAP#1 ("Collection<CAP#1> cannot be converted to Collection<E>")。
// 镜像 commons-collections4 UnmodifiableCollection / UnmodifiableMap / KeySetIterator。
// kill-switch: JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestUnmodifiableCollectionSuperCastIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/UnmodifiableCollection.class")
	if err != nil {
		t.Fatalf("read UnmodifiableCollection: %v", err)
	}
	base, err := os.ReadFile("testdata/regression/AbstractCollectionDecorator.class")
	if err != nil {
		t.Fatalf("read AbstractCollectionDecorator: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		switch {
		case strings.HasSuffix(internalName, "UnmodifiableCollection"):
			return seed, true
		case strings.HasSuffix(internalName, "AbstractCollectionDecorator"):
			return base, true
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
