package javaclassparser

import "testing"

// A case can fall through into a sibling's terminal body. That shared body
// must keep its state update even when other arms continue a nested scan loop.
func TestAdversarialSwitchTerminalScanRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "TerminalScan", `public class TerminalScan {
  final String input;
  final boolean lenient;
  int pos,limit,checks,fills;
  TerminalScan(String input,boolean lenient) {
    this.input=input;this.lenient=lenient;
    limit=Math.min(3,input.length());
  }
  void check() {
    checks++;
    if(!lenient) throw new IllegalStateException();
  }
  boolean fill() {
    fills++;
    if(limit==input.length()) return false;
    limit=Math.min(input.length(),limit+3);
    return true;
  }
  void skip() {
    do {
      int i=0;
      for(;pos+i<limit;i++) {
        switch(input.charAt(pos+i)) {
          case '#': case ';': check();
          case ' ': case ',': pos+=i; return;
          default:
        }
      }
      pos+=i;
    } while(fill());
  }
  static void probe(String input,boolean lenient) {
    TerminalScan s=new TerminalScan(input,lenient);
    try { s.skip(); System.out.print("ok:"); }
    catch(IllegalStateException e) { System.out.print("fail:"); }
    System.out.print(s.pos+":"+s.checks+":"+s.fills+";");
  }
  public static void main(String[] args) {
    for(String input:new String[]{"","abc","abcdefg"," x","ab,x","abcde x","#x","ab;x","abcde;x"}) {
      probe(input,false);probe(input,true);
    }
  }
}`, Precision, Compatibility, "legacy")
}
