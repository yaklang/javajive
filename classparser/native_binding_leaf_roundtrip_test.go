package javaclassparser

import (
	"strings"
	"testing"
)

const nativeBindingAnchorFixture = `class AnchorEffects{static final IllegalArgumentException failure=new IllegalArgumentException("original");static int calls;static Object token(Object value,boolean fail){calls++;if(fail)throw failure;return value;}}
class AnchorOwner{static int seed;private Object token;AnchorOwner(Object value,boolean fail){try{token=AnchorEffects.token(value,fail);}catch(IllegalArgumentException e){throw e;}}class Reader{Object get(){return token;}}}
class AnchorDriver{public static void main(String[]args){int rows=0;for(Object token:new Object[]{null,new Object()}){AnchorEffects.calls=0;AnchorOwner owner=new AnchorOwner(token,false);if(owner.new Reader().get()!=token||AnchorEffects.calls!=1)throw new AssertionError("identity/once");try{new AnchorOwner(token,true);throw new AssertionError("missing failure");}catch(IllegalArgumentException e){if(e!=AnchorEffects.failure||AnchorEffects.calls!=2)throw new AssertionError("failure identity/once");}rows++;}System.out.println(rows+":anchor:binding:identity:failure");}}`

// Legacy verification accepts a handler interval containing an implicit Object
// constructor. Its empty source node must still retain original CFG ownership.
func testNativeBindingAnchor(t *testing.T, source, owner string) {
	t.Helper()
	testNativePrivateSetterFixtureWithMutation(t, source, owner, "AnchorDriver", "2:anchor:binding:identity:failure\n", func(t *testing.T, files map[string][]byte) {
		for name, raw := range files {
			obj, e := Parse(raw)
			if e != nil {
				t.Fatal(e)
			}
			obj.MajorVersion = 49
			obj.MinorVersion = 0
			if name == owner+".class" {
				for _, m := range obj.Methods {
					mn, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if mn != "<init>" {
						continue
					}
					for _, a := range m.Attributes {
						if code, ok := a.(*CodeAttribute); ok {
							if len(code.ExceptionTable) != 1 {
								t.Fatal("expected one original handler")
							}
							code.ExceptionTable[0].StartPc = 0
						}
					}
				}
			}
			files[name] = obj.Bytes()
		}
	})
}
func TestNativeBindingImplicitConstructorAnchorRoundTrip(t *testing.T) {
	testNativeBindingAnchor(t, nativeBindingAnchorFixture, "AnchorOwner")
}
func TestNativeBindingRenamedConstructorAnchorRoundTrip(t *testing.T) {
	testNativeBindingAnchor(t, strings.ReplaceAll(nativeBindingAnchorFixture, "AnchorOwner", "DifferentBindingScope"), "DifferentBindingScope")
}

const nativeBindingBooleanFixture = `class BooleanEffects{static String trace="";static int calls,fail;static final IllegalArgumentException failure=new IllegalArgumentException();static boolean first(boolean flag){calls++;trace+="A";if(fail==1)throw failure;return flag;}static void touch(){}static boolean second(boolean flag){calls++;trace+="B";if(fail==2)throw failure;return flag;}}
class BooleanOwner{private Object token;static int reserved;BooleanOwner(Object token){this.token=token;}boolean accept(boolean first,boolean second){boolean state=true;if(first){state=BooleanEffects.first(second);}else{state=BooleanEffects.first(false);}boolean result=BooleanEffects.second(first)&state;BooleanEffects.touch();return result;}boolean mix(boolean first,int word){return (BooleanEffects.first(first)?1:0)==word;}class Reader{Object get(){return token;}}}
class BooleanDriver{public static void main(String[]args){BooleanOwner owner=new BooleanOwner(new Object());if(owner.new Reader().get()==null)throw new AssertionError("binding");int rows=0;for(boolean a:new boolean[]{false,true})for(boolean b:new boolean[]{false,true}){BooleanEffects.fail=0;BooleanEffects.trace="";BooleanEffects.calls=0;if(owner.accept(a,b)!=(a&b)||BooleanEffects.calls!=2||!BooleanEffects.trace.equals("AB"))throw new AssertionError("bitwise eager once");for(int fail:new int[]{1,2}){BooleanEffects.fail=fail;BooleanEffects.trace="";BooleanEffects.calls=0;try{owner.accept(a,b);throw new AssertionError("missing failure");}catch(IllegalArgumentException e){if(e!=BooleanEffects.failure||BooleanEffects.calls!=fail||!BooleanEffects.trace.equals(fail==1?"A":"AB"))throw new AssertionError("failure identity/order");}}BooleanEffects.fail=0;for(int word:new int[]{Integer.MIN_VALUE,-2,-1,0,1,2,Integer.MAX_VALUE}){BooleanEffects.calls=0;if(owner.mix(a,word)!=((a?1:0)==word)||BooleanEffects.calls!=1)throw new AssertionError("numeric ABI/once");}rows++;}System.out.println(rows+":boolean:binding:eager:identity:failure");}}`

func TestNativeBindingBooleanWordRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeBindingBooleanFixture, "BooleanOwner", "BooleanDriver", "4:boolean:binding:eager:identity:failure\n")
}
func TestNativeBindingBooleanWordRenamedRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, strings.ReplaceAll(nativeBindingBooleanFixture, "BooleanOwner", "DifferentBooleanScope"), "DifferentBooleanScope", "BooleanDriver", "4:boolean:binding:eager:identity:failure\n")
}
