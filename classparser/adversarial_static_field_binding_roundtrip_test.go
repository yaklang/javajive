package javaclassparser

import (
	"strings"
	"testing"
)

// The unused value declarations are deliberately absent from all field
// instructions. Source binding must nevertheless account for them. Ordinary
// reads, writes, same-named parameters and interface fields share the rule.
func TestAdversarialStaticFieldBindingRetainsOriginalClassAndEffects(t *testing.T) {
	fixture := `class FieldBindingState{static String trace="";static final Object token=new Object();}
interface FieldBindingValues{Object token=FieldBindingState.token;}
class FieldBindingDecoy{static int value=101;static Object token=new Object();}
class FieldBindingOwner{
 FieldBindingDecoy FieldBindingOwner,FieldBindingValues;
 static volatile int value;static Object token;
 static{FieldBindingState.trace+="C";value=31;token=FieldBindingState.token;}
 static class Anchor{}
 int read(int value){return ((FieldBindingOwner)null).value;}
 Object alias(Object FieldBindingOwner){return ((FieldBindingOwner)null).token;}
 int write(int value,Object FieldBindingOwner){((FieldBindingOwner)null).value=value;return ((FieldBindingOwner)null).value;}
 Object contract(){return ((FieldBindingValues)null).token;}
}
class FieldBindingDriver{public static void main(String[]args){
 if(!FieldBindingState.trace.equals(""))throw new AssertionError("early initialization");
 FieldBindingOwner owner=new FieldBindingOwner();
 if(!FieldBindingState.trace.equals("C")||owner.read(99)!=31||owner.alias(null)!=FieldBindingState.token||owner.contract()!=FieldBindingState.token)throw new AssertionError("symbolic owner/type/initialization");
 int rows=0;for(int v:new int[]{Integer.MIN_VALUE,0,31,Integer.MAX_VALUE}){
  if(owner.write(v,null)!=v||owner.read(v^17)!=v||owner.alias(new Object())!=FieldBindingState.token||!FieldBindingState.trace.equals("C"))throw new AssertionError("volatile read/write and no receiver check");rows++;
 }System.out.println(rows+":static:field:type:value:effects");}}
`
	for _, renamed := range []bool{false, true} {
		name := "original names"
		owner := "FieldBindingOwner"
		f := fixture
		if renamed {
			name = "independent names"
			owner = "IndependentStaticScope"
			f = strings.ReplaceAll(strings.ReplaceAll(f, "FieldBindingOwner", owner), "FieldBindingValues", "SeparateStaticContract")
		}
		t.Run(name, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, f, []string{owner}, "FieldBindingDriver", "4:static:field:type:value:effects\n", nativeLexicalExactSignatures)
		})
	}
}
