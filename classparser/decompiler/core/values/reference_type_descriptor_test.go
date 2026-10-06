package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestReferenceTypeDescriptorPreservesIdentityAndBounds(t *testing.T) {
	stringTag := types.NewJavaPrimer(types.JavaString)
	integer := types.NewJavaPrimer(types.JavaInteger)
	for _, tc := range []struct {
		name      string
		typeValue types.JavaType
		want      string
	}{
		{"literal String", stringTag, "Ljava/lang/String;"},
		{"descriptor String", types.NewJavaClass("java.lang.String"), "Ljava/lang/String;"},
		{"array of literal String", types.NewJavaArrayType(stringTag), "[Ljava/lang/String;"},
		{"primitive array", types.NewJavaArrayType(integer), "[I"},
		{"nested primitive array", types.NewJavaArrayType(types.NewJavaArrayType(integer)), "[[I"},
		{"scalar int", integer, ""},
		{"scalar char", types.NewJavaPrimer(types.JavaChar), ""},
		{"void array", types.NewJavaArrayType(types.NewJavaPrimer(types.JavaVoid)), ""},
		{"unknown default package identity", types.NewJavaClass("Token"), ""},
		{"invalid class identity", types.NewJavaClass("bad;Name"), ""},
		{"absent type", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReferenceTypeDescriptor(tc.typeValue, &class_context.ClassContext{}); got != tc.want {
				t.Fatalf("descriptor=%q want=%q", got, tc.want)
			}
		})
	}
	ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) {
		return callbinding.Class{Name: "Token"}, name == "Token"
	}}
	if got := ReferenceTypeDescriptor(types.NewJavaClass("Token"), ctx); got != "LToken;" {
		t.Fatal("independent default-package declaration identity", got)
	}
	cyclic := types.NewJavaArrayType(integer)
	cyclic.RawType().(*types.JavaArrayType).JavaType = cyclic
	if ReferenceTypeDescriptor(cyclic, ctx) != "" {
		t.Fatal("cyclic type graph admitted")
	}
	rank := types.NewJavaArrayType(integer)
	rank.RawType().(*types.JavaArrayType).Dimension = 255
	if got := ReferenceTypeDescriptor(rank, ctx); len(got) != 256 || got[255:] != "I" {
		t.Fatal("maximum legal JVM rank identity", got)
	}
	rank.RawType().(*types.JavaArrayType).Dimension = 256
	if ReferenceTypeDescriptor(rank, ctx) != "" {
		t.Fatal("overflowing JVM rank admitted")
	}
}
