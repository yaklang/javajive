package javaclassparser

import "testing"

func TestAdversarialLoopContinuationBesideEarlyReturnsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "LoopTerminalExits", `public class LoopTerminalExits {
  static int scan(String input) {
    int limit=Math.min(3,input.length()),i=0,value=0,sign=1;
    outer:
    for(;;i++) {
      if(i==limit) {
        if(i==10) return -99;
        if(limit==input.length()) break;
        limit=Math.min(limit+3,input.length());
      }
      char c=input.charAt(i);
      switch(c) {
        case '-':
          if(i!=0) return -1;
          sign=-1;
          break;
        case '+': return -2;
        default:
          if(c<'0'||c>'9') {
            if(c==' ') break outer;
            return -3;
          }
          value=value*10+c-'0';
      }
    }
    if(i==0) return 0;
    return value*sign;
  }
  static int scanFlat(String input) {
    int limit=Math.min(3,input.length()),i=0,value=0;
    for(;;i++) {
      if(i==limit) {
        if(i==10) return -99;
        if(limit==input.length()) break;
        limit=Math.min(limit+3,input.length());
      }
      char c=input.charAt(i);
      if(c==' ') break;
      if(c<'0'||c>'9') return -3;
      value=value*10+c-'0';
    }
    return value;
  }
  public static void main(String[] args) {
    for(String s:new String[]{"","1","12","123","123456","-1234","42 "," ","+1","1-2","12x"}) {
      System.out.print(scan(s)+":"+scanFlat(s)+";");
    }
  }
}`, Precision, Compatibility, "legacy")
}
