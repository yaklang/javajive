package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
	"testing"
)

// The raw list deliberately contains a non-String payload. The bytecode bridge
// only forwards its reference; introducing an element cast changes its contract.
const nativeConcreteGenericCallFixture = `class ConcreteEffects{static String trace="";static int fail;static final java.io.IOException failure=new java.io.IOException("original");static ConcreteOwner receiver(ConcreteOwner owner)throws java.io.IOException{trace+="R";if(fail==1)throw failure;return owner;}static java.util.List argument(java.util.List token)throws java.io.IOException{trace+="A";if(fail==2)throw failure;return token;}static long wide(long n)throws java.io.IOException{trace+="W";if(fail==3)throw failure;return n;}}
class ConcreteOwner{private java.util.List<String> prepare(java.util.List<String> token,long n)throws java.io.IOException{ConcreteEffects.trace+="P";if(ConcreteEffects.fail==4)throw ConcreteEffects.failure;if(n!=Long.MIN_VALUE)throw new AssertionError("wide");return token;}private Object prepare(Object token,long n){throw new AssertionError("Object overload");}private java.util.ArrayList<String> prepare(java.util.ArrayList<String> token,long n){throw new AssertionError("ArrayList overload");}static class Reader{java.util.List get(ConcreteOwner owner,java.util.List token,long n)throws java.io.IOException{return ConcreteEffects.receiver(owner).prepare(ConcreteEffects.argument(token),ConcreteEffects.wide(n));}}}
class ConcreteDerived extends ConcreteOwner{public java.util.List<String> prepare(java.util.List<String> token,long n){throw new AssertionError("virtual binding");}}
class ConcreteDriver{public static void main(String[]args)throws Exception{ConcreteOwner.Reader reader=new ConcreteOwner.Reader();ConcreteOwner owner=new ConcreteDerived();java.util.List raw=new java.util.ArrayList();Object payload=Integer.valueOf(123456);raw.add(payload);int rows=0;for(java.util.List token:new java.util.List[]{null,new java.util.ArrayList(),raw}){ConcreteEffects.fail=0;ConcreteEffects.trace="";java.util.List result=reader.get(owner,token,Long.MIN_VALUE);if(result!=token||!ConcreteEffects.trace.equals("RAWP"))throw new AssertionError("binding/identity/order");if(token==raw){if(result.get(0)!=payload)throw new AssertionError("payload cast");Object other=new Object();result.add(other);if(raw.get(1)!=other)throw new AssertionError("alias");}for(int fail:new int[]{1,2,3,4}){ConcreteEffects.fail=fail;ConcreteEffects.trace="";try{reader.get(owner,token,Long.MIN_VALUE);throw new AssertionError("missing failure");}catch(java.io.IOException e){if(e!=ConcreteEffects.failure)throw new AssertionError("checked identity");}if(!ConcreteEffects.trace.equals(fail==1?"R":fail==2?"RA":fail==3?"RAW":"RAWP"))throw new AssertionError("failure order");}rows++;}ConcreteEffects.fail=0;ConcreteEffects.trace="";try{reader.get(null,null,Long.MIN_VALUE);throw new AssertionError("missing null receiver");}catch(NullPointerException e){if(!ConcreteEffects.trace.equals("RAW"))throw new AssertionError("null before arguments");}System.out.println(rows+":concrete:generic:private:erasure:binding");}}`

func TestNativeConcreteGenericPrivateCallRoundTrip(t *testing.T) {
	testNativePrivateSetterCompiledFixture(t, "ConcreteOwner", "ConcreteDriver", "3:concrete:generic:private:erasure:binding\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, nativeConcreteGenericCallFixture, debug)
	}, nativeConcreteCallNoAddedCast)
}

func nativeConcreteCallNoAddedCast(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	if !strings.HasSuffix(name, "$Reader.class") {
		return
	}
	for _, raw := range [][]byte{original, rebuilt} {
		obj, e := Parse(raw)
		if e != nil {
			t.Fatal(e)
		}
		found := false
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
							t.Fatal("closed parameterized bridge introduced a runtime cast")
						}
					}
				}
			}
		}
		if !found {
			t.Fatal("missing original reader method")
		}
	}
}
func TestNativeConcreteGenericStaticCallRoundTrip(t *testing.T) {
	f := strings.Replace(nativeStaticPrivateCallFixture, "Object prepare(Object token,long n)", "Object prepare(java.util.List<String> token,long n)", 1)
	f = strings.Replace(f, "prepare(StaticCallEffects.argument(token)", "prepare((java.util.List)StaticCallEffects.argument(token)", 1)
	f = strings.Replace(f, "new Object[]{null,new Object()}", "new Object[]{null,new java.util.ArrayList()}", 1)
	testNativePrivateSetterFixture(t, f, "StaticCallOwner", "StaticCallDriver", "2:static:private:call:binding:order\n")
}
func TestNativeConcreteGenericRenamedWildcardCallRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeConcreteGenericCallFixture, "ConcreteOwner", "DistinctConcreteScope")
	f = strings.ReplaceAll(f, "prepare", "originalDispatch")
	f = strings.ReplaceAll(f, "List<String>", "List<? extends Number>")
	testNativePrivateSetterFixture(t, f, "DistinctConcreteScope", "ConcreteDriver", "3:concrete:generic:private:erasure:binding\n")
}

func TestNativeConcreteGenericArrayCallRoundTrip(t *testing.T) {
	source := `class ConcreteArrayEffects{static final java.io.IOException failure=new java.io.IOException("original");}
class ConcreteArrayOwner{private java.util.List<? super Number>[][] prepare(java.util.List<? super Number>[][] token,long n)throws java.io.IOException{if(n==Long.MIN_VALUE)throw ConcreteArrayEffects.failure;return token;}private Object prepare(Object token,long n){throw new AssertionError("Object overload");}static class Reader{java.util.List[][] get(ConcreteArrayOwner owner,java.util.List[][] token,long n)throws java.io.IOException{return owner.prepare(token,n);}}}
class ConcreteArrayDriver{public static void main(String[]args)throws Exception{ConcreteArrayOwner.Reader reader=new ConcreteArrayOwner.Reader();ConcreteArrayOwner owner=new ConcreteArrayOwner();java.util.List raw=new java.util.ArrayList();Object payload=new Object();raw.add(payload);int rows=0;for(java.util.List[][] token:new java.util.List[][][]{null,new java.util.List[0][],new java.util.List[][]{null,{raw}}}){if(reader.get(owner,token,Long.MAX_VALUE)!=token)throw new AssertionError("rank/identity");if(token!=null&&token.length==2&&token[1][0].get(0)!=payload)throw new AssertionError("raw payload");try{reader.get(owner,token,Long.MIN_VALUE);throw new AssertionError("missing failure");}catch(java.io.IOException e){if(e!=ConcreteArrayEffects.failure)throw new AssertionError("checked identity");}rows++;}System.out.println(rows+":concrete:generic:array:rank:identity");}}`
	testNativePrivateSetterCompiledFixture(t, "ConcreteArrayOwner", "ConcreteArrayDriver", "3:concrete:generic:array:rank:identity\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeConcreteCallNoAddedCast)
}
