package javaclassparser

// 承重测试: Map<E, ? super V>.put 的 KEY 形参仍是裸 E, 不能因为兄弟通配 `? super V`
// 就放弃 `(E)` 造型。镜像 commons-collections4 MapBackedSet.addAll。
// kill-switch: JDEC_GENERIC_PARAM_INFER_OFF。

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestMapBackedSetPutKeyCastIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/MapBackedSet.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedGenericField(t, raw, "map", "Ljava/util/Map;", "Ljava/util/Map<TE;-TV;>;")
	assertReviewedTypeVarMethod(t, raw, "addAll", "(Ljava/util/Collection;)Z", "(Ljava/util/Collection<+TE;>;)Z")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_GENERIC_PARAM_INFER_OFF", setting)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		body := reviewedSourceMethod(t, source, `boolean\s+addAll\(Collection<\? extends E>\s+\w+\)`)
		element := requireReviewedPattern(t, body, `E\s+(\w+)\s*=\s*\w+\.next\(\)\s*;`)[1]
		requireReviewedPattern(t, body, `this\.map\.put\(\s*`+regexp.QuoteMeta(element)+`\s*,[^;]*this\.dummyValue`)
		if strings.Contains(body, "(String)") {
			t.Fatalf("erased E gained a payload check:\n%s", body)
		}
	}
}
