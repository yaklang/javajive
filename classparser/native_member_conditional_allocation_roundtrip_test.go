package javaclassparser

import (
	"strings"
	"testing"
)

const nativeMemberConditionalAllocationFixture = `class ChoiceEffects{static String trace="";static int fail;static final RuntimeException error=new IllegalArgumentException("original");static Object published;static Object choose(Object value,int branch){trace+=branch==1?"L":"R";if(fail==branch)throw error;return value;}}
class ChoiceParent{final Object observed,input;ChoiceParent(Object input){ChoiceEffects.trace+="P";this.observed=owner();this.input=input;ChoiceEffects.published=this;if(ChoiceEffects.fail==3)throw ChoiceEffects.error;}Object owner(){return null;}}
class ChoiceOwner{class Child extends ChoiceParent{final Object value;Child(Object value){super(value);ChoiceEffects.trace+="C";this.value=value;}Object owner(){return ChoiceOwner.this;}}Child make(boolean left,Object value){return new Child(left?ChoiceEffects.choose(value,1):ChoiceEffects.choose(value,2));}}
class ChoiceDriver{public static void main(String[]args)throws Exception{ChoiceOwner root=new ChoiceOwner();Object token=new Object();int rows=0;for(boolean left:new boolean[]{false,true})for(Object value:new Object[]{null,token})for(int fail:new int[]{0,1,2,3}){ChoiceEffects.fail=fail;ChoiceEffects.trace="";ChoiceEffects.published=null;boolean raises=fail==(left?1:2)||fail==3;try{ChoiceOwner.Child child=root.make(left,value);if(raises||child.input!=value||child.value!=value||child.owner()!=root||child.observed!=root||ChoiceEffects.published!=child||!ChoiceEffects.trace.equals(left?"LPC":"RPC"))throw new AssertionError("conditional branch identity/order");}catch(RuntimeException e){if(!raises||e!=ChoiceEffects.error||!ChoiceEffects.trace.equals((left?"L":"R")+(fail==3?"P":"")))throw new AssertionError("original error priority",e);if(fail==3){ChoiceOwner.Child child=(ChoiceOwner.Child)ChoiceEffects.published;if(child==null||child.observed!=root||child.owner()!=root||child.value!=null)throw new AssertionError("capture/default");}else if(ChoiceEffects.published!=null)throw new AssertionError("premature parent");}rows++;}System.out.println(rows+":conditional:constructor:identity:order");}}
`

func TestNativeMemberConditionalAllocationRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeMemberConditionalAllocationFixture, []string{"ChoiceOwner"}, "ChoiceDriver", "16:conditional:constructor:identity:order\n")
}
func TestNativeMemberConditionalAllocationRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeMemberConditionalAllocationFixture, "ChoiceOwner", "OtherChoiceScope")
	testNativeIndependentFamilyFixture(t, f, []string{"OtherChoiceScope"}, "ChoiceDriver", "16:conditional:constructor:identity:order\n")
}
func TestNativeMemberConditionalNestedAllocationRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeMemberConditionalAllocationFixture, `return new Child(left?ChoiceEffects.choose(value,1):ChoiceEffects.choose(value,2));`, `return new Child(new Child(left?ChoiceEffects.choose(value,1):ChoiceEffects.choose(value,2)));`)
	f = strings.ReplaceAll(f, `child.input!=value||child.value!=value`, `child.input!=child.value||((ChoiceOwner.Child)child.value).value!=value||((ChoiceOwner.Child)child.value).owner()!=root`)
	f = strings.ReplaceAll(f, `left?"LPC":"RPC"`, `left?"LPCPC":"RPCPC"`)
	testNativeIndependentFamilyFixture(t, f, []string{"ChoiceOwner"}, "ChoiceDriver", "16:conditional:constructor:identity:order\n")
}

func TestNativeMemberConditionalAllocationWideJoinsRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeMemberConditionalAllocationFixture, `final Object observed,input;ChoiceParent(Object input){`, `final Object observed,input;final long n;final double d;ChoiceParent(Object input,long n,double d){this.n=n;this.d=d;`)
	f = strings.ReplaceAll(f, `Child(Object value){super(value);`, `Child(Object value,long n,double d){super(value,n,d);`)
	f = strings.ReplaceAll(f, `ChoiceEffects.choose(value,2));`, `ChoiceEffects.choose(value,2),left?Long.MIN_VALUE:Long.MAX_VALUE,left?-0.0d:Double.NaN);`)
	f = strings.ReplaceAll(f, `child.input!=value||child.value!=value`, `child.n!=(left?Long.MIN_VALUE:Long.MAX_VALUE)||Double.doubleToRawLongBits(child.d)!=(left?Long.MIN_VALUE:0x7ff8000000000000L)||child.input!=value||child.value!=value`)
	testNativeIndependentFamilyFixture(t, f, []string{"ChoiceOwner"}, "ChoiceDriver", "16:conditional:constructor:identity:order\n")
}
func TestNativeMemberConditionalAllocationHandlerRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeMemberConditionalAllocationFixture, `return new Child(left?ChoiceEffects.choose(value,1):ChoiceEffects.choose(value,2));`, `try{return new Child(left?ChoiceEffects.choose(value,1):ChoiceEffects.choose(value,2));}catch(RuntimeException e){ChoiceEffects.trace+="K";throw e;}`)
	f = strings.ReplaceAll(f, `(left?"L":"R")+(fail==3?"P":"")`, `(left?"L":"R")+(fail==3?"PK":"K")`)
	testNativeIndependentFamilyFixture(t, f, []string{"ChoiceOwner"}, "ChoiceDriver", "16:conditional:constructor:identity:order\n")
}
