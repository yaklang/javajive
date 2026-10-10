package javaclassparser

import "testing"

// The first definition is folded into a control head; later stores stay in
// its body. Moving only a bare declaration must preserve the computed value,
// short-circuit calls, repeated iterations and exceptions.
func TestAdversarialControlHeadLocalRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionLocal", `
public class ConditionLocal {
  static int normalize(int[] input,boolean enabled) {
    int total=0;
    for(int index=0;index<input.length;index++) {
      if(enabled) {
        int value=ConditionOracle.read(input,index);
        if(value!=0) {
          if(value<0) value=-value;
          total=total*7+value;
        }
      }
    }
    return total;
  }
  static int decode(int[] input,boolean enabled) {
    int total=0;
    for(int index=0;index<input.length;index++) {
      if(enabled) {
        int value=ConditionOracle.read(input,index);
        if(value==31) return total;
        if((value&32)==0) value|=64;
        total=total*3+value;
      }
    }
    return total;
  }
  static int fold(int[] input,boolean enabled) {
    int total=0;
    for(int index=0;index<input.length;index++) {
      int value;
      if(enabled && (value=ConditionOracle.read(input,index))!=0) {
        if(value<0) value=-value;
        total^=value*(index+1);
      }
    }
    return total;
  }
  public static void main(String[] args){ConditionOracle.run();}
}
class ConditionOracle {
  static StringBuilder trace;static int calls,failAt;
  static int read(int[] input,int index){
    trace.append(index).append(':').append(input[index]).append(';');
    if(++calls==failAt) throw new IllegalStateException("read:"+index);
    return input[index];
  }
  static void run(){
    int[][] inputs={ {},{0},{-3,0,5},{31,1},{32,63,2,-9},{Integer.MIN_VALUE,0,7} };
    for(int shape=0;shape<3;shape++)for(int sample=0;sample<inputs.length;sample++)for(boolean enabled:new boolean[]{false,true})for(int failure:new int[]{0,1,2,4}) {
      trace=new StringBuilder();calls=0;failAt=failure;String result;
      try { int value=shape==0 ? ConditionLocal.normalize(inputs[sample],enabled) : shape==1 ? ConditionLocal.decode(inputs[sample],enabled) : ConditionLocal.fold(inputs[sample],enabled);result="value:"+value; }
      catch(Throwable e){result=e.getClass().getSimpleName()+":"+e.getMessage();}
      System.out.println(shape+":"+sample+":"+enabled+":"+failure+":"+result+":"+calls+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
