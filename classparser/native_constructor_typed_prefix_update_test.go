package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The original parameter write is part of the delegation argument. It must
// occur once before the parent observes its value, and remain visible to the
// child body after delegation, including narrowing and floating point rounding.
func nativeConstructorTypedPrefixFixture() (string, string) {
	var classes, oracle, checks strings.Builder
	rows := 0
	for _, typ := range nativeTypedPostCases {
		n := typ.name
		classes.WriteString(strings.ReplaceAll(`static class ParentTYPE{final TYPE argument;ParentTYPE(TYPE value){argument=value;parents++;}}`, "TYPE", n))
		oracle.WriteString(strings.NewReplacer("TYPE", n, "NEXT", typ.next, "EQUAL", typ.equal).Replace(`static TYPE nextTYPE(TYPE x,int delta){return NEXT;}static boolean eqTYPE(TYPE got,TYPE next){return EQUAL;}`))
		for _, delta := range []int{1, -1} {
			op, suffix := "++", "Up"
			if delta < 0 {
				op, suffix = "--", "Down"
			}
			classes.WriteString(strings.NewReplacer("TYPE", n, "SUFFIX", suffix, "UPDATE", op).Replace(`static class ChildTYPESUFFIX extends ParentTYPE{final TYPE later;ChildTYPESUFFIX(TYPE value){super(UPDATEvalue);later=value;children++;}}`))
			checks.WriteString(strings.NewReplacer("TYPE", n, "SUFFIX", suffix, "DELTA", fmt.Sprint(delta), "INPUTS", typ.inputs).Replace(`for(TYPE x:new TYPE[]{INPUTS}){TYPE next=nextTYPE(x,DELTA);ConstructorNumericOwner.parents=0;ConstructorNumericOwner.children=0;ConstructorNumericOwner.ChildTYPESUFFIX c=new ConstructorNumericOwner.ChildTYPESUFFIX(x);if(!eqTYPE(c.argument,next)||!eqTYPE(c.later,next)||ConstructorNumericOwner.parents!=1||ConstructorNumericOwner.children!=1)throw new AssertionError("TYPE SUFFIX constructor argument/body/order");rows++;}`))
			rows += strings.Count(typ.inputs, ",") + 1
		}
	}
	source := fmt.Sprintf(`class ConstructorNumericOwner{static int parents,children;%s}class ConstructorNumericDriver{%s public static void main(String[]args){int rows=0;%s System.out.println("constructor:numeric:"+rows);}}`, classes.String(), oracle.String(), checks.String())
	return source, fmt.Sprintf("constructor:numeric:%d\n", rows)
}
func TestNativeConstructorTypedPrefixPreservesArgumentAndBody(t *testing.T) {
	source, want := nativeConstructorTypedPrefixFixture()
	testNativePrivateSetterFixture(t, source, "ConstructorNumericOwner", "ConstructorNumericDriver", want)
}
func TestNativeConstructorTypedPrefixOriginalCompiler(t *testing.T) {
	if os.Getenv("JAVA8_JAVAC") == "" {
		t.Skip("JAVA8_JAVAC required for original compiler")
	}
	source, want := nativeConstructorTypedPrefixFixture()
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		return localGetterCompileJavac8(t, map[string]string{"ConstructorNumericOwner.java": source}, debug)
	}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"ConstructorNumericOwner"}, "ConstructorNumericDriver", want, nil, nativeLexicalExactSignatures)
}
