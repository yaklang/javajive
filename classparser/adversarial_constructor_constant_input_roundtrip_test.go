package javaclassparser

import "testing"

// Existing positive control for native member capture reconstruction.
// At this call site the original constant selects a normal receiver-silent
// path. The parent remains original bytecode; the oracle also exercises failure
// at other inputs so a general no-throw annotation would be detected.
func TestAdversarialConstructorCaptureUsesOriginalConstantInputRoundTrip(t *testing.T) {
	const f = `class InputProofParent{final int api;InputProofParent(int n){if(n!=7)throw new IllegalArgumentException("api:"+n);api=n;}Object token(){return null;}Object owner(){return null;}}class InputProofOwner{class Child extends InputProofParent{final Object value;Child(Object value){super(7);this.value=value;}Object token(){return value;}Object owner(){return InputProofOwner.this;}}InputProofParent make(Object value){return new Child(value);}}class InputProofDriver{public static void main(String[]args){Object token=new Object();InputProofOwner owner=new InputProofOwner();for(Object v:new Object[]{null,token}){InputProofParent p=owner.make(v);if(p.api!=7||p.token()!=v||p.owner()!=owner)throw new AssertionError("capture/constant input");System.out.println("7:"+(p.token()==token));}for(int n:new int[]{-1,0,6,8}){try{new InputProofParent(n);throw new AssertionError("missing original failure");}catch(IllegalArgumentException e){System.out.println(e.getMessage());}}}}`
	testNativeIndependentFamilyFixture(t, f, []string{"InputProofOwner"}, "InputProofDriver", "7:false\n7:true\napi:-1\napi:0\napi:6\napi:8\n", nativeLexicalExactSignatures)
}
