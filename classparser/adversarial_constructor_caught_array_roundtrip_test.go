package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialConstructorFilledArrayWithCaughtFailureKeepsOrderRoundTrip(t *testing.T) {
	for _, owner := range []string{"ArraySuperOwner", "RenamedArraySuperOwner"} {
		t.Run(owner, func(t *testing.T) {
			fixture := `class ArraySuperOwner extends ArraySuperBase {
 ArraySuperOwner(Object input,int n){super(input,new ArraySuperItem[]{make(n)});ArraySuperEffects.trace+="Q";try{if(ArraySuperEffects.fail==4)throw ArraySuperEffects.postFailure;}catch(RuntimeException error){ArraySuperEffects.trace+="C";throw error;}}
 private static ArraySuperItem make(int n){ArraySuperEffects.trace+="F";if(ArraySuperEffects.fail==1)throw ArraySuperEffects.failure;return new ArraySuperItem(n*31+7);}
}
class ArraySuperItem{final int value;ArraySuperItem(int n){ArraySuperEffects.trace+="I";if(ArraySuperEffects.fail==2)throw ArraySuperEffects.failure;value=n;}}
class ArraySuperBase{final Object token;final ArraySuperItem[] items;ArraySuperBase(Object input,ArraySuperItem... a){ArraySuperEffects.trace+="P";if(ArraySuperEffects.fail==3)throw ArraySuperEffects.failure;token=input;items=a;}}
class ArraySuperEffects{static String trace;static int fail;static final RuntimeException failure=new RuntimeException("producer/constructor identity"),postFailure=new RuntimeException("post super identity");}
class ArraySuperDriver{public static void main(String[]args){int rows=0;Object token=new Object();for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(Object input:new Object[]{null,token})for(int fail:new int[]{0,1,2,3,4}){ArraySuperEffects.trace="";ArraySuperEffects.fail=fail;try{ArraySuperOwner first=new ArraySuperOwner(input,n);int expected=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7)).intValue();if(fail!=0||first.token!=input||first.items.length!=1||first.items[0].value!=expected||!ArraySuperEffects.trace.equals("FIPQ"))throw new AssertionError("second argument producer/identity/order");ArraySuperOwner second=new ArraySuperOwner(input,n);if(second==first||second.items==first.items||second.items[0]==first.items[0]||second.items[0].value!=expected||!ArraySuperEffects.trace.equals("FIPQFIPQ"))throw new AssertionError("array/allocation repeated identity/order");}catch(RuntimeException error){String expected=fail==1?"F":fail==2?"FI":fail==3?"FIP":"FIPQC";if(fail==0||error!=(fail==4?ArraySuperEffects.postFailure:ArraySuperEffects.failure)||!ArraySuperEffects.trace.equals(expected))throw new AssertionError("partial initialization effects/error identity",error);}rows++;}System.out.println(rows+":filled-array:super:order:identity:errors");}}
`
			fixture = strings.ReplaceAll(fixture, "ArraySuperOwner", owner)
			testNativePrivateSetterFixture(t, fixture, owner, "ArraySuperDriver", "50:filled-array:super:order:identity:errors\n")
		})
	}
}
