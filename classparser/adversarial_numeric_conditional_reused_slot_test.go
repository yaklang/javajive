package javaclassparser

import "testing"

func TestAdversarialNumericConditionalReusedSlotRoundTrip(t *testing.T) {
	const fixture = `
class CharSentinel {
 static long scan(String s,int at){
  {char current=s.charAt(at);if(current!='.')return current;}
  int before=at>0?s.charAt(at-1):-1;
  int after=at+1<s.length()?s.charAt(at+1):-1;
  return ((long)before<<32) ^ (after&0xffffffffL);
 }
}
class CharSentinelDriver {public static void main(String[]args){int rows=0;for(String s:new String[]{".","..","a.b","\u0000.\uffff","\ud800.\udc00","...","x","\uffff.\u0000"}){char[]a=s.toCharArray();for(int p=0;p<a.length;p++){long expected;if(a[p]!='.')expected=a[p];else{int before=p==0?-1:a[p-1];int after=p+1==a.length?-1:a[p+1];expected=((long)before<<32)^(after&0xffffffffL);}if(CharSentinel.scan(s,p)!=expected)throw new AssertionError("char/int sentinel join");rows++;}for(int p:new int[]{-1,a.length})try{CharSentinel.scan(s,p);throw new AssertionError("bounds");}catch(StringIndexOutOfBoundsException expected){rows++;}}if(rows!=35)throw new AssertionError("coverage "+rows);System.out.println("35:char:sentinel:word32:bounds");}}
`
	testNativeIndependentFamilyFixture(t, fixture, []string{"CharSentinel"}, "CharSentinelDriver", "35:char:sentinel:word32:bounds\n", nativeLexicalExactSignatures)
}
