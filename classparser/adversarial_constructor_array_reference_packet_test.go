package javaclassparser

import (
	"strings"
	"testing"
)

const constructorArrayReferencePacketFixture = `
class ArrayPacketParent {final Object[] result;ArrayPacketParent(Object[] result){this.result=result;}}
class ArrayPacketAdapter {static Object[] forward(Object[] values){return values;}}
class ArrayPacketSubject extends ArrayPacketParent {
 ArrayPacketSubject(int[] payload,boolean flag){super(ArrayPacketAdapter.forward(new Object[]{payload,flag?null:"r"}));}
}
class ArrayPacketDriver {
 public static void main(String[]args){
  for(int[] payload:new int[][]{null,new int[0],new int[]{Integer.MIN_VALUE,Integer.MAX_VALUE}})
   for(boolean flag:new boolean[]{false,true}){
    ArrayPacketSubject subject=new ArrayPacketSubject(payload,flag);
    if(subject.result.getClass()!=Object[].class||subject.result[0]!=payload||
      (flag?subject.result[1]!=null:!"r".equals(subject.result[1])))throw new AssertionError("array identity/element selection");
    System.out.println((payload==null?-1:payload.length)+":"+flag+":"+(subject.result[0]==payload));
   }
 }
}`

// A JVM array, including an array of primitives, is an identity-bearing
// reference. Treating it as a scalar primitive or rejecting its descriptor
// prevents reconstruction of an otherwise closed constructor operand packet.
// The original driver, forwarding adapter and superclass stay untouched.
func TestAdversarialConstructorArrayReferencePacketsRoundTrip(t *testing.T) {
	for _, variant := range []string{"primitive array", "reference array", "nested primitive array", "renamed"} {
		t.Run(variant, func(t *testing.T) {
			f := constructorArrayReferencePacketFixture
			subject := "ArrayPacketSubject"
			switch variant {
			case "reference array":
				f = strings.ReplaceAll(f, "int[", "String[")
				f = strings.ReplaceAll(f, "Integer.MIN_VALUE,Integer.MAX_VALUE", "\"a\",\"z\"")
			case "nested primitive array":
				f = strings.ReplaceAll(f, "int[] payload", "int[][] payload")
				f = strings.ReplaceAll(f, "new int[][]{null,new int[0],new int[]{Integer.MIN_VALUE,Integer.MAX_VALUE}}", "new int[][][]{null,new int[0][],new int[][]{new int[]{Integer.MIN_VALUE,Integer.MAX_VALUE},null}}")
			case "renamed":
				f = strings.ReplaceAll(f, "ArrayPacketSubject", "NestedOperandSubject")
				f = strings.ReplaceAll(f, "ArrayPacketAdapter", "OperandIdentity")
				f = strings.ReplaceAll(f, "forward", "preserve")
				subject = "NestedOperandSubject"
			}
			testNativeIndependentFamilyFixture(t, f, []string{subject}, "ArrayPacketDriver",
				"-1:false:true\n-1:true:true\n0:false:true\n0:true:true\n2:false:true\n2:true:true\n", nativeLexicalExactSignatures)
		})
	}
}
