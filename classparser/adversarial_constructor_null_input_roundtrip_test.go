package javaclassparser

import (
	"strings"
	"testing"
)

// The original null operand selects the silent arm of a constructor. An
// unknown or non-null input still permits a callback before capture stores.
// Keep the parent and driver as original bytecode so that moving those stores
// without a call-site proof changes an independently observed result.
func TestAdversarialConstructorOriginalNullInputRoundTrip(t *testing.T) {
	const fixture = `
class NullInputParent {
 final int word;static Object observed;
 NullInputParent(Object gate,int n){this(n,gate);}
 NullInputParent(int n,Object gate){if(gate!=null)observed=token();word=n;}
 Object token(){return null;}Object owner(){return null;}
}
class NullInputOwner {
 class Child extends NullInputParent {
  final Object value;
  Child(Object value,int n){super(null,n);this.value=value;}
  Object token(){return value;}Object owner(){return NullInputOwner.this;}
 }
 NullInputParent make(Object value,int n){return new Child(value,n);}
}
class NullInputDriver {
 public static void main(String[]args){int rows=0;NullInputOwner owner=new NullInputOwner();Object token=new Object();
 for(Object v:new Object[]{null,token})for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  NullInputParent.observed=token;NullInputParent p=owner.make(v,n);
  if(p.word!=n||p.token()!=v||p.owner()!=owner||NullInputParent.observed!=token)throw new AssertionError("null delegation/capture");rows++;
 }
 // The parent's original public domain includes both sides of the condition.
 NullInputParent.observed=token;new NullInputParent(null,3);if(NullInputParent.observed!=token)throw new AssertionError("null arm");
 new NullInputParent(token,4);if(NullInputParent.observed!=null)throw new AssertionError("non-null arm");
 System.out.println(rows+":null:delegation:unknown-parent-domain");
 }
}`
	for _, namespace := range []string{"NullInput", "AbsentGate"} {
		t.Run(namespace, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "NullInput", namespace)
			testNativeIndependentFamilyFixture(t, source, []string{namespace + "Owner"}, namespace+"Driver", "10:null:delegation:unknown-parent-domain\n", nativeLexicalExactSignatures)
		})
	}
}
