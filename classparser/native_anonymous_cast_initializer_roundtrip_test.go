package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousCastInitializerFixture = `class CastInitTrace {static String trace="";static CastInitParent published;static final RuntimeException error=new RuntimeException("producer");static Object observe(Object v,int n){trace+="D";if(n==9)throw error;return v;}}
abstract class CastInitParent {CastInitParent(int n){CastInitTrace.trace+="P";CastInitTrace.published=this;if(first()!=0||view()!=null||last()!=0)throw new AssertionError("parent sees defaults");}abstract int first();abstract int last();abstract Object view();}
class CastInitOwner {CastInitParent make(Object input,int tag){return new CastInitParent(tag){int before=17;String item=(String)CastInitTrace.observe(input,tag);int after=23;int first(){return before;}int last(){return after;}Object view(){return item;}};}}
class CastInitDriver {public static void main(String[]args){for(Object value:new Object[]{null,"",new String("identity")}){CastInitTrace.trace="";CastInitParent p=new CastInitOwner().make(value,0);if(p.view()!=value||p.first()!=17||p.last()!=23||!CastInitTrace.trace.equals("PD")||!p.getClass().getName().equals("CastInitOwner$1"))throw new AssertionError("cast target/value/effect/owner");}for(Object value:new Object[]{Integer.valueOf(7),new Object()}){for(int n:new int[]{0,9}){CastInitTrace.trace="";CastInitTrace.published=null;try{new CastInitOwner().make(value,n);throw new AssertionError("missing cast/producer failure");}catch(RuntimeException e){if(n==9?e!=CastInitTrace.error:!(e instanceof ClassCastException))throw new AssertionError("failure kind/identity");CastInitParent partial=CastInitTrace.published;if(partial==null||partial.first()!=17||partial.view()!=null||partial.last()!=0||!CastInitTrace.trace.equals("PD"))throw new AssertionError("original ordered partial stores");}}}System.out.println("7:cast:identity:exception:partial:order");}}
`

func TestNativeAnonymousInitializerReferenceCastRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousCastInitializerFixture, "CastInitOwner", "CastInitDriver", "7:cast:identity:exception:partial:order\n")
}
func TestNativeAnonymousInitializerArrayCastRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousCastInitializerFixture, "String item=(String)", "String[] item=(String[])", 1)
	f = strings.Replace(f, `new Object[]{null,"",new String("identity")}`, `new Object[]{null,new String[0],new String[]{"identity"}}`, 1)
	f = strings.Replace(f, `new Object[]{Integer.valueOf(7),new Object()}`, `new Object[]{new Object[0],new int[0]}`, 1)
	testNativePrivateSetterFixture(t, f, "CastInitOwner", "CastInitDriver", "7:cast:identity:exception:partial:order\n")
}
func TestNativeAnonymousInitializerReferenceCastRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousCastInitializerFixture, "CastInitOwner", "SeparateCastScope")
	testNativePrivateSetterFixture(t, f, "SeparateCastScope", "CastInitDriver", "7:cast:identity:exception:partial:order\n")
}
