package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Original T[] is erased to Object[]. A write through that array view preserves
// the actual AASTORE runtime component check; it need not add a source (T) cast.
func reviewedTypeVarArrayParameter(t *testing.T, source string) string {
	t.Helper()
	match := regexp.MustCompile(`T\[\]toArray\(T\[\]([A-Za-z_$][A-Za-z0-9_$]*)\)`).FindStringSubmatch(compactReviewedGenericSource(source))
	if len(match) != 2 {
		t.Fatal("original generic toArray parameter was lost")
	}
	return match[1]
}
func TestTypeVarArrayReassignCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/AbstractLinkedList.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedTypeVarMethod(t, data, "toArray", "([Ljava/lang/Object;)[Ljava/lang/Object;", "<T:Ljava/lang/Object;>([TT;)[TT;")
	assertReviewedTypeVarInvoke(t, "testdata/regression/AbstractLinkedList.class", "toArray", "([Ljava/lang/Object;)[Ljava/lang/Object;", 22, core.OP_INVOKESTATIC, "java/lang/reflect/Array", "newInstance", "(Ljava/lang/Class;I)Ljava/lang/Object;")
	_, code, _ := reviewedFixtureMethod(t, "testdata/regression/AbstractLinkedList.class", "toArray", "([Ljava/lang/Object;)[Ljava/lang/Object;")
	assertReviewedOpcode(t, code, 31, core.OP_ASTORE_1)
	assertReviewedOpcode(t, code, 56, core.OP_AASTORE)

	var on string
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_TYPEVAR_ARRAY_REASSIGN_OFF", setting)
		t.Setenv("JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF", "")
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		compact := compactReviewedGenericSource(source)
		parameter := reviewedTypeVarArrayParameter(t, source)
		if !strings.Contains(compact, parameter+"=(T[])") || !strings.Contains(compact, "Array.newInstance("+parameter+".getClass().getComponentType()") || !strings.Contains(compact, "return"+parameter+";") {
			t.Fatal("generic array resize must assign and return the original array-local identity")
		}
		if setting == "" {
			on = source
		} else if source != on {
			t.Fatal("retired array-reassign spelling gate must not change declaration-proven output")
		}
	}
}
func TestTypeVarLocalArrayElemStoreCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/AbstractLinkedList.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedTypeVarMethod(t, data, "toArray", "([Ljava/lang/Object;)[Ljava/lang/Object;", "<T:Ljava/lang/Object;>([TT;)[TT;")
	assertReviewedTypeVarInvoke(t, "testdata/regression/AbstractLinkedList.class", "toArray", "([Ljava/lang/Object;)[Ljava/lang/Object;", 22, core.OP_INVOKESTATIC, "java/lang/reflect/Array", "newInstance", "(Ljava/lang/Class;I)Ljava/lang/Object;")
	_, code, _ := reviewedFixtureMethod(t, "testdata/regression/AbstractLinkedList.class", "toArray", "([Ljava/lang/Object;)[Ljava/lang/Object;")
	assertReviewedOpcode(t, code, 31, core.OP_ASTORE_1)
	assertReviewedOpcode(t, code, 56, core.OP_AASTORE)

	var on string
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF", setting)
		t.Setenv("JDEC_TYPEVAR_ARRAY_REASSIGN_OFF", "")
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		compact := compactReviewedGenericSource(source)
		parameter := reviewedTypeVarArrayParameter(t, source)
		if !strings.Contains(compact, "((java.lang.Object[])("+parameter+"))[") || !regexp.MustCompile(regexp.QuoteMeta("((java.lang.Object[])("+parameter+"))[")+`[^\]]+\]=[A-Za-z_$][A-Za-z0-9_$]*\.getValue\(\);`).MatchString(compact) {
			t.Fatal("element store must use the same original array through its erased Object[] view")
		}
		if setting == "" {
			on = source
		} else if source != on {
			t.Fatal("retired element-cast spelling gate must not change the original array store")
		}
	}
}
