package javaclassparser

import "testing"

// Rebuild only the accessor. Original callers and producers independently check
// allocation identity/component, initializer order, mutation isolation and the
// original throwable, across actual public modes and both input compilers.
func TestAdversarialUnicodeArrayDeclarationRetainsOriginalAllocationAndEffects(t *testing.T) {
	const fixture = `class 项目{final int number;项目(int number){this.number=number;}}
class 原件{static final 项目 A=new 项目(1),B=new 项目(2);static int fail=-1;static String trace="";static final RuntimeException error=new RuntimeException("original");static 项目 take(int index){trace+=index;if(fail==index)throw error;return index==0?A:B;}}
class BindingAccess{static 项目[] values(){return new 项目[]{原件.take(0),原件.take(1)};}}
class BindingDriver{public static void main(String[]args){int rows=0;for(int i=0;i<4;i++){原件.fail=-1;原件.trace="";项目[] first=BindingAccess.values(),second=BindingAccess.values();if(first==second||first.length!=2||second.length!=2||first[0]!=原件.A||first[1]!=原件.B||second[0]!=原件.A||second[1]!=原件.B||!原件.trace.equals("0101"))throw new AssertionError("fresh arrays/original element identities/order");first[0]=null;if(second[0]!=原件.A)throw new AssertionError("array mutation isolation");try{Object[] view=second;view[0]=new Object();throw new AssertionError("runtime component widened");}catch(ArrayStoreException expected){}rows++;}for(int fail:new int[]{0,1}){原件.fail=fail;原件.trace="";try{BindingAccess.values();throw new AssertionError("missing initializer failure");}catch(RuntimeException error){if(error!=原件.error)throw new AssertionError("original throwable identity",error);}if(!原件.trace.equals(fail==0?"0":"01"))throw new AssertionError("failure order");rows++;}System.out.println(rows+":unicode-array:fresh:identity:effects:component");}}
`
	fieldOwnerOriginalDriverRoundTrip(t, fixture, "6:unicode-array:fresh:identity:effects:component\n")
}
