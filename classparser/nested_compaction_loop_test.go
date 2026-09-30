package javaclassparser

import "testing"

func TestAdversarialNestedCompactionExitRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedCompaction", `public class NestedCompaction {
  static String compact(String input) {
    int cursor=input.indexOf('x');
    if(cursor<0) return input;
    char[] data=input.toCharArray();
    int removed=1;
    outer:
    while(true) {
      cursor++;
      while(true) {
        if(cursor==data.length) break outer;
        if(data[cursor]=='x') { removed++; continue outer; }
        data[cursor-removed]=data[cursor];
        cursor++;
      }
    }
    return new String(data,0,cursor-removed);
  }
  public static void main(String[] args) {
    for(String input:new String[]{"","a","x","xx","ax","xa","axb","xaxbxxc","abcdefx","xxxxxx"}) {
      String actual=compact(input);
      if(!actual.equals(input.replace("x",""))) throw new AssertionError(input);
      System.out.print("["+actual+"]");
    }
  }
}`, Precision, Compatibility, "legacy")
}
