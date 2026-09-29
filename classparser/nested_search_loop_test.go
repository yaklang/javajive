package javaclassparser

import "testing"

func TestAdversarialNestedSearchReturnsAndContinuesRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "NestedSearch", `public class NestedSearch {
  final char[] data;
  int pos,limit,lines,fills;
  NestedSearch(String input) { data=input.toCharArray(); limit=Math.min(3,data.length); }
  boolean fill(int needed) {
    fills++;
    if(limit==data.length) return false;
    limit=Math.min(data.length,Math.max(pos+needed,limit+3));
    return pos+needed<=limit;
  }
  boolean find(String needle) {
    int n=needle.length();
    outer:
    for(;pos+n<=limit || fill(n);pos++) {
      if(data[pos]=='\n') { lines++; continue; }
      for(int i=0;i<n;i++) {
        if(data[pos+i]!=needle.charAt(i)) continue outer;
      }
      return true;
    }
    return false;
  }
  public static void main(String[] args) {
    for(String text:new String[]{"","*","*/","x*/y","****/","x\nyy*/","aaaaa","abacababacaba"}) {
      for(String needle:new String[]{"*/","aba","aa","zz"}) {
        NestedSearch s=new NestedSearch(text);
        System.out.print(s.find(needle)+":"+s.pos+":"+s.lines+":"+s.fills+";");
      }
    }
  }
}`, Precision, Compatibility, "legacy")
}
