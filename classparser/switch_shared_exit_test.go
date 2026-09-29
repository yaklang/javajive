package javaclassparser

import "testing"

// javac shares the inner break with the enclosing switch's exit. Empty arms
// still transfer control; rendering them as grouped labels changes state.
func TestAdversarialNestedSwitchEmptySharedExitRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "NestedEmptyExit", `public class NestedEmptyExit {
  static String decode(int mode, int[] codes) {
    StringBuilder out=new StringBuilder();
    int shifted=0;
    for(int code:codes) {
      switch(mode) {
        case 1:
          if(code<10) {
            out.append((char)('A'+code+shifted));
            shifted=0;
          } else {
            switch(code) {
              case 10: case 11: break;
              case 12: shifted=3; break;
              case 13: out.append('!'); break;
              default: break;
            }
          }
          break;
        case 2: out.append(code); break;
        default: throw new IllegalArgumentException();
      }
      out.append('.');
    }
    return out.toString();
  }
  // In contrast, these labels intentionally fall through into the state change.
  static int fallThrough(int mode,int code) {
    int state=1;
    switch(mode) {
      case 1:
        switch(code) {
          case 10: case 11: state+=2;
          case 12: state*=3; break;
          default: state=7;
        }
        break;
      default: state=5;
    }
    return state+100;
  }
  public static void main(String[] args) {
    int[] codes={0,10,1,11,2,12,3,13,4,14,5};
    System.out.print(decode(1,codes)+":"+decode(2,codes));
    for(int mode=0;mode<3;mode++)
      for(int code=9;code<14;code++) System.out.print(":"+fallThrough(mode,code));
  }
}`, Precision, Compatibility, "legacy")
}
