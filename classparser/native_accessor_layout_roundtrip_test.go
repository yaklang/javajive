package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAccessorInterleavedLayoutFixture = `class LayoutEffects{static String trace="";static Object mark(String label){trace+=label;return new Object();}}
class InterleavedAccessOwner{static Object first=LayoutEffects.mark("L");private Object left,right;InterleavedAccessOwner(Object left,Object right){this.left=left;this.right=right;}static class Alpha{private Alpha(){}Object left(InterleavedAccessOwner owner){return owner.left;}}static Alpha alpha(){return new Alpha();}static Object second=LayoutEffects.mark("R");static class Beta{Object right(InterleavedAccessOwner owner){return owner.right;}}static Beta beta(){return new Beta();}}
class LayoutDriver{public static void main(String[]args)throws Exception{Object first=InterleavedAccessOwner.first,second=InterleavedAccessOwner.second;if(first==second||!LayoutEffects.trace.equals("LR"))throw new AssertionError("enclosing initialization order");InterleavedAccessOwner.Alpha a=InterleavedAccessOwner.alpha();InterleavedAccessOwner.Beta b=InterleavedAccessOwner.beta();int rows=0;for(Object left:new Object[]{null,new Object()})for(Object right:new Object[]{null,left,new Object()}){InterleavedAccessOwner owner=new InterleavedAccessOwner(left,right);if(a.left(owner)!=left||b.right(owner)!=right)throw new AssertionError("source registration binding");rows++;}for(int i=0;i<2;i++){try{if(i==0)a.left(null);else b.right(null);throw new AssertionError("lost dereference");}catch(NullPointerException expected){}}java.lang.reflect.Constructor<?>ctor=InterleavedAccessOwner.Alpha.class.getDeclaredConstructor();if(!java.lang.reflect.Modifier.isPrivate(ctor.getModifiers())||InterleavedAccessOwner.Alpha.class.getDeclaredConstructors().length!=2)throw new AssertionError("private bridge ABI");System.out.println(rows+":layout:ordinal:binding:fields:LR");}}`

func TestNativeAccessorInterleavedDeclarationLayoutRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAccessorInterleavedLayoutFixture, "InterleavedAccessOwner", "LayoutDriver", "6:layout:ordinal:binding:fields:LR\n")
}
func TestNativeAccessorInterleavedDeclarationLayoutRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAccessorInterleavedLayoutFixture, "InterleavedAccessOwner", "OtherPrivateDeclarationScope")
	testNativePrivateSetterFixture(t, f, "OtherPrivateDeclarationScope", "LayoutDriver", "6:layout:ordinal:binding:fields:LR\n")
}
