package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousInheritedCallFixture = `class ImmediateEffects{static int calls,fail;static String trace="";static final IllegalArgumentException failure=new IllegalArgumentException("original");}
class ImmediateOwner{private Object token;ImmediateOwner(Object token){this.token=token;}abstract class Layer{final Object run(){ImmediateEffects.trace+="R";return visit();}abstract Object visit();}Object get(){return new Layer(){Object visit(){ImmediateEffects.calls++;ImmediateEffects.trace+="V";if(ImmediateEffects.fail!=0)throw ImmediateEffects.failure;return token;}}.run();}}
class ImmediateDriver{public static void main(String[]args){int rows=0;for(Object token:new Object[]{null,new Object()}){ImmediateOwner owner=new ImmediateOwner(token);for(int fail:new int[]{0,1}){ImmediateEffects.calls=0;ImmediateEffects.trace="";ImmediateEffects.fail=fail;try{Object got=owner.get();if(fail!=0||got!=token)throw new AssertionError("result identity");}catch(IllegalArgumentException e){if(fail==0||e!=ImmediateEffects.failure)throw new AssertionError("failure identity");}if(ImmediateEffects.calls!=1||!ImmediateEffects.trace.equals("RV"))throw new AssertionError("dispatch/once/order");rows++;}}System.out.println(rows+":anonymous:inherited:dispatch:identity:failure");}}`

func testNativeAnonymousInheritedCall(t *testing.T, source, owner string) {
	t.Helper()
	testNativePrivateSetterFixtureWithMutation(t, source, owner, "ImmediateDriver", "4:anonymous:inherited:dispatch:identity:failure\n", func(t *testing.T, files map[string][]byte) {
		for n, b := range files {
			obj, e := Parse(b)
			if e != nil {
				t.Fatal(e)
			}
			obj.MajorVersion = 49
			obj.MinorVersion = 0
			files[n] = obj.Bytes()
		}
	})
}
func TestNativeAnonymousInheritedCallRoundTrip(t *testing.T) {
	testNativeAnonymousInheritedCall(t, nativeAnonymousInheritedCallFixture, "ImmediateOwner")
}
func TestNativeAnonymousInheritedCallRenamedRoundTrip(t *testing.T) {
	testNativeAnonymousInheritedCall(t, strings.ReplaceAll(nativeAnonymousInheritedCallFixture, "ImmediateOwner", "OtherImmediateScope"), "OtherImmediateScope")
}
