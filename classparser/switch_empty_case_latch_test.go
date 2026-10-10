package javaclassparser

import "testing"

func TestAdversarialSwitchEmptyCaseKeepsLoopLatchRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "SwitchLoopLatch", `public class SwitchLoopLatch {
  static int validate(String text) {
    int scans=0;
    for(int i=0;i<text.length();i++) {
      if(++scans>40) throw new IllegalStateException("lost loop progress");
      char c=text.charAt(i);
      switch(c) {
        case 241: case 242: case 243: case 244: break;
        default: if(c>127) throw new IllegalArgumentException();
      }
    }
    return scans;
  }
  public static void main(String[] args) {
    System.out.print(validate("Abc-123")+":"+validate("\u00f1\u00f2\u00f3\u00f4A")+":"+validate(""));
    try {validate("\u0200");} catch(IllegalArgumentException e) {System.out.print(":invalid");}
  }
}`, Precision, Compatibility, "legacy")
}
