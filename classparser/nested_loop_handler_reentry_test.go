package javaclassparser

import "testing"

func TestAdversarialNestedLoopHandlerReentryRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedLoopHandlerReentry", `public class NestedLoopHandlerReentry {
  static String trace;
  static int divide(int value) { trace+="D";return 100/value; }
  static int evaluate(int[] values,int rounds,boolean delay) {
    int sum=0;
    for(int round=0;round<rounds;round++) {
      trace+="O";
      for(int i=0;i<values.length;i++) {
        trace+="I";
        try { sum+=divide(values[i]); }
        catch(ArithmeticException e) {
          trace+="C";
          if(!delay)return -1000-sum;
          sum-=7;
        }
        trace+="T";
        if(sum>150)break;
      }
      sum+=round;
    }
    return sum;
  }
  static int drain(int[] values,int rounds,boolean delay) {
    int sum=0;
    for(int round=0;round<rounds;round++) {
      int emitted=0;
      while(emitted<3) {
        boolean missing=false;
        for(int i=0;i<values.length;i++) {
          try {
            int value=divide(values[i]);
            if(value<0)missing=true;
            else sum+=value;
          } catch(ArithmeticException e) {
            trace+="C";
            if(!delay)return -1000-sum;
            missing=true;
          }
        }
        if(missing)break;
        trace+="E";
        sum+=emitted++;
      }
      trace+="R";
    }
    return sum;
  }
  public static void main(String[] args) {
    for(int[] values:new int[][]{{},{1,2},{0},{1,0,2},{0,2,0},{-4,5}})
      for(int rounds:new int[]{0,1,3})for(boolean delay:new boolean[]{false,true}) {
        trace="";
        System.out.print(evaluate(values,rounds,delay)+":"+trace+";");
        trace="";
        System.out.print(drain(values,rounds,delay)+":"+trace+";");
      }
  }
}`, Precision, Compatibility, "legacy")
}
