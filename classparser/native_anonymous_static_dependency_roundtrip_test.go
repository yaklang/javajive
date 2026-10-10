package javaclassparser

import (
	"strings"
	"testing"
)

// The foreign member occurs only inside the owned anonymous body's method
// descriptor/Signature. The enclosing class has no CONSTANT_Class for it.
// Both source families must compile together; keeping the foreign originals
// on the classpath would conceal a stale flat binary source name.
const nativeAnonymousStaticDependencyFixture = `class ForeignBindingScope{static class Value{final Object token;Value(Object token){this.token=token;}}}
abstract class ForeignBindingContract{abstract ForeignBindingScope.Value echo(ForeignBindingScope.Value value);abstract java.util.List<ForeignBindingScope.Value> list(java.util.List<ForeignBindingScope.Value> value);abstract ForeignBindingScope.Value[] array(ForeignBindingScope.Value[] value);}
class AnonymousBindingOwner{static class Local{}static final AnonymousBindingOwner empty=new AnonymousBindingOwner(){};static final ForeignBindingContract callback=new ForeignBindingContract(){ForeignBindingScope.Value echo(ForeignBindingScope.Value value){return value;}java.util.List<ForeignBindingScope.Value> list(java.util.List<ForeignBindingScope.Value> value){return value;}ForeignBindingScope.Value[] array(ForeignBindingScope.Value[] value){return value;}};}
class AnonymousBindingDriver{public static void main(String[]args)throws Exception{Object token=new Object();ForeignBindingScope.Value value=new ForeignBindingScope.Value(token);java.util.List<ForeignBindingScope.Value> list=new java.util.ArrayList<ForeignBindingScope.Value>();list.add(value);list.add(null);ForeignBindingScope.Value[] array={value,null};ForeignBindingContract callback=AnonymousBindingOwner.callback;if(callback.echo(value)!=value||callback.echo(null)!=null||callback.list(list)!=list||callback.list(null)!=null||callback.array(array)!=array||callback.array(null)!=null||value.token!=token)throw new AssertionError("descriptor/signature/array identity");if(callback.getClass().getEnclosingClass()!=AnonymousBindingOwner.class||AnonymousBindingOwner.empty.getClass().getEnclosingClass()!=AnonymousBindingOwner.class||ForeignBindingScope.Value.class.getDeclaringClass()!=ForeignBindingScope.class)throw new AssertionError("source ownership");System.out.println("anonymous:static:dependency:identity:scope");}}`

func TestNativeAnonymousStaticDependencyDescriptorAndSignatureRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeAnonymousStaticDependencyFixture, []string{"AnonymousBindingOwner", "ForeignBindingScope"}, "AnonymousBindingDriver", "anonymous:static:dependency:identity:scope\n", nativeLexicalExactSignatures)
}

func TestNativeAnonymousStaticDependencyRenamedRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativeAnonymousStaticDependencyFixture, "AnonymousBindingOwner", "ChangedAnonymousScope")
	fixture = strings.ReplaceAll(fixture, "ForeignBindingScope", "OtherMemberNames")
	fixture = strings.ReplaceAll(fixture, "Value", "Payload")
	testNativeIndependentFamilyFixture(t, fixture, []string{"ChangedAnonymousScope", "OtherMemberNames"}, "AnonymousBindingDriver", "anonymous:static:dependency:identity:scope\n", nativeLexicalExactSignatures)
}
