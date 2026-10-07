package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestOriginalMethodReferenceCategoriesSurviveStaticCallCloning(t *testing.T) {
	files := nativeCompileClasses(t, `interface KindApi{static int m(){return 1;}}class KindClass{static int m(){return 2;}}class KindProbe{static int holder;static int f(){return KindApi.m()+KindClass.m()+holder;}}`)
	obj, err := Parse(files["KindProbe.class"])
	if err != nil {
		t.Fatal(err)
	}
	seen := map[values.MethodOwnerKind]int{}
	for i, constant := range obj.ConstantPool {
		var want values.MethodOwnerKind
		switch constant.(type) {
		case *ConstantMethodrefInfo:
			want = values.MethodOwnerClass
		case *ConstantInterfaceMethodrefInfo:
			want = values.MethodOwnerInterface
		case *ConstantFieldrefInfo:
			want = values.MethodOwnerUnknown
		default:
			continue
		}
		member := GetValueFromCP(obj.ConstantPool, i+1).(*values.JavaClassMember)
		if member.MethodOwnerKind != want {
			t.Fatalf("original CP %T became category %d, want %d", constant, member.MethodOwnerKind, want)
		}
		if want == values.MethodOwnerUnknown {
			continue
		}
		call := values.NewFunctionCallExpression(values.NewJavaClassValue(types.NewJavaClass(member.Name)), member, member.JavaType.FunctionType())
		if call.MethodOwnerKind != want || call.Clone().MethodOwnerKind != want {
			t.Fatal("original method owner category lost during call construction/clone")
		}
		seen[want]++
	}
	if seen[values.MethodOwnerClass] == 0 || seen[values.MethodOwnerInterface] == 0 {
		t.Fatalf("missing original class/interface controls: %v", seen)
	}
	synthetic := values.NewJavaClassMember("UnknownOwner", "method", "()I", nil)
	if synthetic.MethodOwnerKind != values.MethodOwnerUnknown {
		t.Fatal("synthetic member invented an owner-kind witness")
	}
}
