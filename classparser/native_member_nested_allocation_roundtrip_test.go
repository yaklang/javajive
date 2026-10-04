package javaclassparser

import (
	"strings"
	"testing"
)

const nativeMemberNestedAllocationFixture = `class AllocationEffects{static String trace="";static int fail;static final RuntimeException error=new IllegalArgumentException("original");static Object published;}
class AllocationBox{final Object value;AllocationBox(Object value){AllocationEffects.trace+="B";if(AllocationEffects.fail==1)throw AllocationEffects.error;this.value=value;}}
class AllocationParent{final Object observed;final Object input;AllocationParent(Object input){AllocationEffects.trace+="P";AllocationEffects.published=this;this.observed=owner();this.input=input;if(AllocationEffects.fail==2)throw AllocationEffects.error;}Object owner(){return null;}}
class AllocationOwner{class Child extends AllocationParent{final Object value;Child(Object value){super(value);AllocationEffects.trace+="C";this.value=value;}Object owner(){return AllocationOwner.this;}}Child make(Object value){return new Child(new AllocationBox(value));}}
class AllocationDriver{public static void main(String[]args)throws Exception{AllocationOwner root=new AllocationOwner();Object token=new Object();int rows=0;for(int fail:new int[]{0,1,2})for(Object value:new Object[]{null,token}){AllocationEffects.trace="";AllocationEffects.published=null;AllocationEffects.fail=fail;try{AllocationOwner.Child child=root.make(value);if(fail!=0||child.owner()!=root||child.observed!=root||child.value!=child.input||((AllocationBox)child.value).value!=value||AllocationEffects.published!=child||!AllocationEffects.trace.equals("BPC"))throw new AssertionError("nested NEW identity/effects");}catch(RuntimeException e){if(e!=AllocationEffects.error||fail==0||!AllocationEffects.trace.equals(fail==1?"B":"BP"))throw new AssertionError("failure priority",e);if(fail==1&&AllocationEffects.published!=null)throw new AssertionError("premature parent");if(fail==2){AllocationOwner.Child child=(AllocationOwner.Child)AllocationEffects.published;if(child==null||child.owner()!=root||child.observed!=root||child.value!=null)throw new AssertionError("pre-super capture/post-super default");}}rows++;}System.out.println(rows+":nested:allocation:identity:order");}}
`

func TestNativeMemberNestedAllocationRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeMemberNestedAllocationFixture, []string{"AllocationOwner"}, "AllocationDriver", "6:nested:allocation:identity:order\n")
}
func TestNativeMemberNestedAllocationRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(strings.ReplaceAll(nativeMemberNestedAllocationFixture, "AllocationOwner", "ChangedAllocationScope"), "AllocationBox", "DifferentOperandType")
	testNativeIndependentFamilyFixture(t, f, []string{"ChangedAllocationScope"}, "AllocationDriver", "6:nested:allocation:identity:order\n")
}

const nativeMemberSameOwnerAllocationFixture = `class SameAllocationEffects{static String trace="";static int fail;static int parents;static final RuntimeException error=new IllegalArgumentException("original");static Object published;}
class SameAllocationParent{final Object observed;final Object input;SameAllocationParent(Object input){SameAllocationEffects.trace+="P";SameAllocationEffects.published=this;this.observed=owner();this.input=input;if(++SameAllocationEffects.parents==SameAllocationEffects.fail)throw SameAllocationEffects.error;}Object owner(){return null;}}
class SameAllocationOwner{class Child extends SameAllocationParent{final Object value;Child(Object value){super(value);SameAllocationEffects.trace+="C";this.value=value;}Object owner(){return SameAllocationOwner.this;}}Child make(Object value){return new Child(new Child(value));}}
class SameAllocationDriver{public static void main(String[]args)throws Exception{SameAllocationOwner root=new SameAllocationOwner();Object token=new Object();int rows=0;for(int fail:new int[]{0,1,2})for(Object value:new Object[]{null,token}){SameAllocationEffects.trace="";SameAllocationEffects.parents=0;SameAllocationEffects.published=null;SameAllocationEffects.fail=fail;try{SameAllocationOwner.Child child=root.make(value);SameAllocationOwner.Child inner=(SameAllocationOwner.Child)child.value;if(fail!=0||child==inner||inner.value!=value||child.value!=child.input||inner.value!=inner.input||child.owner()!=root||inner.owner()!=root||child.observed!=root||inner.observed!=root||SameAllocationEffects.published!=child||!SameAllocationEffects.trace.equals("PCPC"))throw new AssertionError("distinct same-type NEW identities");}catch(RuntimeException e){if(e!=SameAllocationEffects.error||fail==0||!SameAllocationEffects.trace.equals(fail==1?"P":"PCP"))throw new AssertionError("nested failure order",e);SameAllocationOwner.Child child=(SameAllocationOwner.Child)SameAllocationEffects.published;if(child==null||child.owner()!=root||child.observed!=root||child.value!=null)throw new AssertionError("capture/default");}rows++;}System.out.println(rows+":same:owner:distinct:allocation");}}
`

func TestNativeMemberNestedSameOwnerAllocationsRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeMemberSameOwnerAllocationFixture, []string{"SameAllocationOwner"}, "SameAllocationDriver", "6:same:owner:distinct:allocation\n")
}

func TestNativeMemberNestedAllocationWideArgumentsRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeMemberNestedAllocationFixture, `final Object value;AllocationBox(Object value){`, `final long n;final double d;final Object value;AllocationBox(Object value,long n,double d){`)
	f = strings.ReplaceAll(f, `this.value=value;}}`, `this.value=value;this.n=n;this.d=d;}}`)
	f = strings.ReplaceAll(f, `Child make(Object value){return new Child(new AllocationBox(value));}`, `Child make(Object value,long n,double d){return new Child(new AllocationBox(value,n,d));}`)
	f = strings.ReplaceAll(f, `root.make(value)`, `root.make(value,Long.MIN_VALUE,-0.0d)`)
	f = strings.ReplaceAll(f, `((AllocationBox)child.value).value!=value`, `((AllocationBox)child.value).value!=value||((AllocationBox)child.value).n!=Long.MIN_VALUE||Double.doubleToRawLongBits(((AllocationBox)child.value).d)!=Long.MIN_VALUE`)
	testNativeIndependentFamilyFixture(t, f, []string{"AllocationOwner"}, "AllocationDriver", "6:nested:allocation:identity:order\n")
}

func TestNativeMemberNestedAllocationHandlerRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeMemberNestedAllocationFixture, `return new Child(new AllocationBox(value));`, `try{return new Child(new AllocationBox(value));}catch(RuntimeException e){AllocationEffects.trace+="K";throw e;}`)
	f = strings.ReplaceAll(f, `fail==1?"B":"BP"`, `fail==1?"BK":"BPK"`)
	testNativeIndependentFamilyFixture(t, f, []string{"AllocationOwner"}, "AllocationDriver", "6:nested:allocation:identity:order\n")
}
