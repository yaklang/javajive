package javaclassparser

import "testing"

func TestAdversarialNestedSwitchKeepsContinuationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedSwitchContinuation", `public class NestedSwitchContinuation {
  static String decode(int[] values) {
    StringBuilder out=new StringBuilder();
    for(int value:values) {
      switch(value) {
        case 0: break;
        case 1: out.append('A');break;
        default:
          switch(value) {
            case 2: out.append('B');break;
            case 3: out.append('C');break;
            default: throw new IllegalArgumentException();
          }
          break;
      }
      out.append('.');
    }
    return out.toString();
  }
  public static void main(String[] args) {
    System.out.print(decode(new int[]{2,1,3,0})+":"+decode(new int[]{}));
    try { decode(new int[]{2,9}); } catch(IllegalArgumentException e) {System.out.print(":rejected");}
  }
}`, Precision, Compatibility, "legacy")
}
