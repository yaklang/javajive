package javaclassparser

import (
	"strings"
	"testing"
)

const nativeInvocationRegistrationFixture = `class InvocationEffects{static String trace="";static int fail;static final IllegalArgumentException failure=new IllegalArgumentException("original");static Object argument(Object token){trace+="A";if(fail==1)throw failure;return token;}}
class InvocationSink{Object accept(Object first,Object second){InvocationEffects.trace+="M";if(InvocationEffects.fail==2)throw InvocationEffects.failure;return first;}}
class InvocationOwner{private static InvocationSink sink=new InvocationSink();private Object token;InvocationOwner(Object token){this.token=token;}static void reset(InvocationSink value){sink=value;}static class Reader{Object get(InvocationOwner owner,Object second){return sink.accept(owner.token,InvocationEffects.argument(second));}}}
class InvocationDriver{public static void main(String[]args){InvocationOwner.Reader reader=new InvocationOwner.Reader();int rows=0;for(Object token:new Object[]{null,new Object()}){InvocationOwner owner=new InvocationOwner(token);InvocationOwner.reset(new InvocationSink());InvocationEffects.fail=0;InvocationEffects.trace="";if(reader.get(owner,new Object())!=token||!InvocationEffects.trace.equals("AM"))throw new AssertionError("identity/order/binding");for(int fail:new int[]{1,2}){InvocationEffects.fail=fail;InvocationEffects.trace="";try{reader.get(owner,new Object());throw new AssertionError("missing failure");}catch(IllegalArgumentException e){if(e!=InvocationEffects.failure||!InvocationEffects.trace.equals(fail==1?"A":"AM"))throw new AssertionError("failure identity/order");} }InvocationEffects.fail=0;InvocationOwner.reset(null);InvocationEffects.trace="";try{reader.get(null,new Object());throw new AssertionError("missing first argument failure");}catch(NullPointerException e){if(!InvocationEffects.trace.equals(""))throw new AssertionError("argument read after receiver null check");}InvocationEffects.trace="";try{reader.get(owner,new Object());throw new AssertionError("missing receiver null failure");}catch(NullPointerException e){if(!InvocationEffects.trace.equals("A"))throw new AssertionError("receiver null check before arguments");}rows++;}System.out.println(rows+":invoke:compiler:registration:runtime:order:identity");}}`

func TestNativeInvocationRegistrationArgumentsBeforeReceiverRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeInvocationRegistrationFixture, "InvocationOwner", "InvocationDriver", "2:invoke:compiler:registration:runtime:order:identity\n")
}
func TestNativeInvocationRegistrationRenamedRoundTrip(t *testing.T) {
	source := strings.ReplaceAll(nativeInvocationRegistrationFixture, "InvocationOwner", "OtherInvocationScope")
	testNativePrivateSetterFixture(t, source, "OtherInvocationScope", "InvocationDriver", "2:invoke:compiler:registration:runtime:order:identity\n")
}

func nativePrivateInvocationRegistrationFixture() string {
	source := strings.ReplaceAll(nativeInvocationRegistrationFixture, "return sink.accept(owner.token,InvocationEffects.argument(second));", "return owner.receiver().accept(owner.token,InvocationEffects.argument(second));")
	source = strings.Replace(source, "static void reset", `private InvocationSink receiver(){InvocationEffects.trace+="S";return sink;}static void reset`, 1)
	source = strings.ReplaceAll(source, `equals("AM")`, `equals("SAM")`)
	source = strings.ReplaceAll(source, `fail==1?"A":"AM"`, `fail==1?"SA":"SAM"`)
	source = strings.ReplaceAll(source, `equals("A")`, `equals("SA")`)
	return source
}
func TestNativeInvocationRegistrationPrivateReceiverRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativePrivateInvocationRegistrationFixture(), "InvocationOwner", "InvocationDriver", "2:invoke:compiler:registration:runtime:order:identity\n")
}
func TestNativeInvocationRegistrationNestedReceiverRoundTrip(t *testing.T) {
	source := `class NestedEffects{static String trace="";static final RuntimeException failure=new RuntimeException();static int fail;static Object argument(){trace+="A";if(fail==1)throw failure;return null;}}
class NestedSink{NestedSink link(Object token){NestedEffects.trace+="L";if(NestedEffects.fail==2)throw NestedEffects.failure;return this;}Object accept(Object token,Object arg){NestedEffects.trace+="M";return token;}}
class NestedOwner{private static NestedSink sink=new NestedSink();private Object first,second;NestedOwner(Object token){first=token;second=token;}static void reset(NestedSink value){sink=value;}static class Reader{Object get(NestedOwner owner){return sink.link(owner.first).accept(owner.second,NestedEffects.argument());}}}
class NestedDriver{public static void main(String[]args){NestedOwner.Reader reader=new NestedOwner.Reader();int rows=0;for(Object token:new Object[]{null,new Object()}){NestedOwner owner=new NestedOwner(token);NestedOwner.reset(new NestedSink());NestedEffects.trace="";NestedEffects.fail=0;if(reader.get(owner)!=token||!NestedEffects.trace.equals("LAM"))throw new AssertionError("nested identity/order");for(int fail:new int[]{1,2}){NestedEffects.fail=fail;NestedEffects.trace="";try{reader.get(owner);throw new AssertionError("missing failure");}catch(RuntimeException e){if(e!=NestedEffects.failure||!NestedEffects.trace.equals(fail==1?"LA":"L"))throw new AssertionError("failure identity/order");}}NestedEffects.fail=0;NestedOwner.reset(null);NestedEffects.trace="";try{reader.get(owner);throw new AssertionError("missing receiver null failure");}catch(NullPointerException e){if(!NestedEffects.trace.equals(""))throw new AssertionError("outer args before inner receiver");}NestedEffects.trace="";try{reader.get(null);throw new AssertionError("missing argument null failure");}catch(NullPointerException e){if(!NestedEffects.trace.equals(""))throw new AssertionError("null owner effects");}rows++;}System.out.println(rows+":nested:compiler:registration:runtime:order:identity");}}`
	testNativePrivateSetterFixture(t, source, "NestedOwner", "NestedDriver", "2:nested:compiler:registration:runtime:order:identity\n")
}
