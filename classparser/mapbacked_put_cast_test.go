package javaclassparser

// 承重测试: Map<E, ? super V>.put 的 KEY 形参仍是裸 E, 不能因为兄弟通配 `? super V`
// 就放弃 `(E)` 造型。镜像 commons-collections4 MapBackedSet.addAll。
// kill-switch: JDEC_GENERIC_PARAM_INFER_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestMapBackedSetPutKeyCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/MapBackedSet.class")
	if err != nil {
		t.Fatalf("read MapBackedSet: %v", err)
	}

	os.Unsetenv("JDEC_GENERIC_PARAM_INFER_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "this.map.put((E)") && !strings.Contains(on, ".put((E)(") {
		t.Errorf("fix ON: expected `map.put((E)(...))`, got:\n%s", on)
	}

	t.Setenv("JDEC_GENERIC_PARAM_INFER_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "this.map.put((E)") || strings.Contains(off, ".put((E)(") {
		t.Errorf("fix OFF: expected no `(E)` put-key cast, got:\n%s", off)
	}
}
