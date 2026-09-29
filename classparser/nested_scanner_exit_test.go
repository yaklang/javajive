package javaclassparser

import "testing"

func TestAdversarialNestedScannerTerminalBranchesRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "NestedScanner", `public class NestedScanner {
  char[] data;
  int pos,limit;
  NestedScanner(String input) { data=input.toCharArray(); limit=Math.min(3,data.length); }
  boolean refill() { if(limit==data.length) return false; limit=Math.min(limit+3,data.length); return true; }
  String token(char stop) {
    char[] chars=data;
    StringBuilder joined=null;
    for(;;) {
      int i=pos,end=limit,start=i;
      while(i<end) {
        char c=chars[i++];
        if(c==stop) {
          pos=i;
          int length=i-start-1;
          if(joined==null) return new String(chars,start,length);
          joined.append(chars,start,length);
          return joined.toString();
        }
        if(c=='~') {
          pos=i;
          int length=i-start-1;
          if(joined==null) joined=new StringBuilder();
          joined.append(chars,start,length);
          joined.append('!');
          i=pos; end=limit; start=i;
        }
      }
      if(joined==null) joined=new StringBuilder();
      joined.append(chars,start,i-start);
      pos=i;
      if(!refill()) throw new IllegalStateException("missing delimiter");
    }
  }
  public static void main(String[] args) {
    for(String input:new String[]{";","a;","ab;","abc;","abcdefgh;","~;","a~bc~defgh;","a;b;","bad"}) {
      NestedScanner s=new NestedScanner(input);
      try { System.out.print(s.token(';')+":"+s.pos+";"); }
      catch(IllegalStateException e) { System.out.print(e.getMessage()+":"+s.pos+";"); }
    }
  }
}`, Precision, Compatibility, "legacy")
}
