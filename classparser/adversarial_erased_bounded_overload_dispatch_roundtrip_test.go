package javaclassparser

import (
	"strings"
	"testing"
)

// JVM overload selection is fixed by the invocation descriptor. A discarded
// generic result must not let the source pick the narrower wrapper itself.
func TestAdversarialErasedBoundedOverloadKeepsOriginalDispatchRoundTrip(t *testing.T) {
	for _, owner := range []string{"BoundedDispatchOwner", "RenamedBoundedDispatchOwner"} {
		t.Run(owner, func(t *testing.T) {
			fixture := `class BoundedDispatchOwner {
 int calls;
 public <A extends Appendable> A write(A target,java.util.Iterator<?> parts)throws java.io.IOException{calls++;while(parts.hasNext())target.append(String.valueOf(parts.next()));return target;}
 public StringBuilder write(StringBuilder target,java.util.Iterator<?> parts){try{write((Appendable)target,parts);return target;}catch(java.io.IOException failure){throw new AssertionError(failure);}}
 public StringBuilder write(StringBuilder target,Iterable<?> parts){return write(target,parts.iterator());}
}
class BoundedDispatchDriver {
 static final class Recording implements Appendable{final java.io.IOException failure=new java.io.IOException("sentinel");final StringBuilder text=new StringBuilder();public Appendable append(char ch)throws java.io.IOException{if(text.length()==2)throw failure;text.append(ch);return this;}public Appendable append(CharSequence s)throws java.io.IOException{for(int i=0;i<s.length();i++)append(s.charAt(i));return this;}public Appendable append(CharSequence s,int a,int b)throws java.io.IOException{return append(s.subSequence(a,b));}}
 public static void main(String[] args)throws Exception{int rows=0;for(Object[] parts:new Object[][]{new Object[]{},new Object[]{"x"},new Object[]{"alpha",null,"\u03a9",42}}){BoundedDispatchOwner owner=new BoundedDispatchOwner();StringBuilder target=new StringBuilder("prefix:");StringBuilder result=owner.write(target,java.util.Arrays.asList(parts));StringBuilder expected=new StringBuilder("prefix:");for(Object p:parts)expected.append(String.valueOf(p));if(result!=target||owner.calls!=1||!target.toString().equals(expected.toString()))throw new AssertionError("original generic target/identity/effects");rows++;}BoundedDispatchOwner owner=new BoundedDispatchOwner();Recording recording=new Recording();try{owner.write(recording,java.util.Arrays.asList("ab","cd").iterator());throw new AssertionError("missing effectful IOException");}catch(java.io.IOException failure){if(failure!=recording.failure||owner.calls!=1||!recording.text.toString().equals("ab"))throw new AssertionError("exception identity/partial effects");}System.out.println(rows+":bounded:dispatch:identity:effects");}
}
`
			fixture = strings.ReplaceAll(fixture, "BoundedDispatchOwner", owner)
			testNativePrivateSetterFixture(t, fixture, owner, "BoundedDispatchDriver", "3:bounded:dispatch:identity:effects\n")
		})
	}
}
