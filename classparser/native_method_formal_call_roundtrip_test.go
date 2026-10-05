package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"slices"
	"strings"
	"testing"
)

// Erased raw payloads intentionally include non-String values: a source
// inference or overload change must not add an element/return cast.
func TestNativeMethodFormalPrivateCallRoundTrip(t *testing.T) {
	f := strings.Replace(nativeConcreteGenericCallFixture,
		"private java.util.List<String> prepare(java.util.List<String> token,long n)",
		"private <E> java.util.List<E> prepare(java.util.List<E> token,long n)", 1)
	testNativePrivateSetterCompiledFixture(t, "ConcreteOwner", "ConcreteDriver", "3:concrete:generic:private:erasure:binding\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) }, nativeConcreteCallNoAddedCast)
}
func TestNativeMethodFormalPrivateCallRenamedRoundTrip(t *testing.T) {
	f := strings.Replace(nativeConcreteGenericCallFixture,
		"private java.util.List<String> prepare(java.util.List<String> token,long n)",
		"private <Payload,Unused> java.util.List<Payload> prepare(java.util.List<Payload> token,long n)", 1)
	f = strings.ReplaceAll(f, "ConcreteOwner", "MethodScope")
	f = strings.ReplaceAll(f, "prepare", "selectPayload")
	testNativePrivateSetterCompiledFixture(t, "MethodScope", "ConcreteDriver", "3:concrete:generic:private:erasure:binding\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) }, nativeConcreteCallNoAddedCast)
}
func TestNativeMethodFormalStaticCallRoundTrip(t *testing.T) {
	f := strings.Replace(nativeStaticPrivateCallFixture,
		"private static Object prepare(Object token,long n)",
		"private static <E> E prepare(E token,long n)", 1)
	testNativePrivateSetterCompiledFixture(t, "StaticCallOwner", "StaticCallDriver", "2:static:private:call:binding:order\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) }, nativeConcreteCallNoAddedCast)
}

const nativeMethodArrayFixture = `class ArrayMethodOwner{private <Element> Element[] select(Element[] token){return token;}private String[] select(String[] token){throw new AssertionError("overload");}static class Reader{Object[] get(ArrayMethodOwner owner,String input){return owner.<Object>select((Object[])new String[]{input});}}}
class ArrayMethodDriver{public static void main(String[]args){ArrayMethodOwner owner=new ArrayMethodOwner();ArrayMethodOwner.Reader reader=new ArrayMethodOwner.Reader();int rows=0;for(String s:new String[]{null,"payload"}){Object[] result=reader.get(owner,s);if(result.getClass()!=String[].class||result.length!=1||result[0]!=s)throw new AssertionError("array shape/identity");try{result[0]=new Object();throw new AssertionError("array type widened");}catch(ArrayStoreException expected){}rows++;}System.out.println(rows+":method:array:physical:identity");}}`

func TestNativeMethodFormalArrayIdentityRoundTrip(t *testing.T) {
	f := nativeMethodArrayFixture
	testNativePrivateSetterCompiledFixture(t, "ArrayMethodOwner", "ArrayMethodDriver", "2:method:array:physical:identity\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) }, nativeMethodArrayExactCastSequence)
}

func nativeMethodArrayExactCastSequence(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	if !strings.HasSuffix(name, "$Reader.class") {
		return
	}
	casts := func(raw []byte) []string {
		obj, e := Parse(raw)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		var out []string
		for _, m := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, m.NameIndex)
			if n != "get" {
				continue
			}
			found = true
			for _, a := range m.Attributes {
				if code, ok := a.(*CodeAttribute); ok {
					d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					if e := d.ParseOpcode(); e != nil {
						t.Fatal(e)
					}
					for _, op := range d.Opcodes() {
						if op.Instr.OpCode == core.OP_CHECKCAST {
							idx := int(binary.BigEndian.Uint16(op.Data))
							c, ok := obj.ConstantPool[idx-1].(*ConstantClassInfo)
							if !ok || c == nil {
								t.Fatal("cast class tag")
							}
							name, known := sourceBridgeUTF8(obj, c.NameIndex)
							if !known {
								t.Fatal("cast target")
							}
							out = append(out, name)
						}
					}
				}
			}
		}
		if !found {
			t.Fatal("missing reader")
		}
		return out
	}
	old, next := casts(original), casts(rebuilt)
	if !slices.Equal(old, next) {
		t.Fatalf("changed original runtime cast sequence %v -> %v", old, next)
	}
}

func TestNativeMethodFormalFreshArrayRoundTrip(t *testing.T) {
	f := strings.Replace(nativeMethodArrayFixture, "owner.<Object>select((Object[])new String[]{input})", "owner.<Object>select(new Object[]{input})", 1)
	f = strings.Replace(f, "result.getClass()!=String[].class", "result.getClass()!=Object[].class", 1)
	f = strings.Replace(f, "try{result[0]=new Object();throw new AssertionError(\"array type widened\");}catch(ArrayStoreException expected){}", "Object next=new Object();result[0]=next;if(result[0]!=next)throw new AssertionError(\"array alias\");", 1)
	testNativePrivateSetterCompiledFixture(t, "ArrayMethodOwner", "ArrayMethodDriver", "2:method:array:physical:identity\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) }, nativeMethodArrayExactCastSequence)
}
func TestNativeMethodFormalVarargsArrayRoundTrip(t *testing.T) {
	f := strings.Replace(nativeMethodArrayFixture, "Element[] select(Element[] token)", "Element[] select(Element... token)", 1)
	testNativePrivateSetterCompiledFixture(t, "ArrayMethodOwner", "ArrayMethodDriver", "2:method:array:physical:identity\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) }, nativeMethodArrayExactCastSequence)
}

func TestNativeMethodFormalCallerShadowRoundTrip(t *testing.T) {
	f := strings.Replace(nativeStaticPrivateCallFixture, "private static Object prepare(Object token,long n)", "private static <E> E prepare(E token,long n)", 1)
	f = strings.Replace(f, "Object get(Object token,long n)", "<Object extends Number> java.lang.Object get(java.lang.Object token,long n)", 1)
	testNativePrivateSetterCompiledFixture(t, "StaticCallOwner", "StaticCallDriver", "2:static:private:call:binding:order\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) }, nativeConcreteCallNoAddedCast)
}
