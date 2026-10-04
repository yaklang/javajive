package javaclassparser

import "testing"

// javac reuses the same int-category slot for the short-circuit flag and the
// array length. Their def/use webs are disjoint despite identical istore types.
// The live parameter and loop controls below must keep their joined identities.
func TestAdversarialPrimitiveSlotWebRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "PrimitiveReuse", `
public class PrimitiveReuse {
  static int allocate(boolean enabled,int length,int failure) {
    try {
      Object context=PrimitiveOracle.context(length);
      boolean flag=enabled && PrimitiveOracle.check(failure);
      PrimitiveOracle.note(context);
      PrimitiveOracle.note(Boolean.valueOf(flag));
    } catch(IllegalStateException e) { PrimitiveOracle.note(e.getMessage()); }
    try {
      Object context=PrimitiveOracle.context(length);
      int size=enabled ? length : 4;
      PrimitiveOracle.note(context);
      byte[] buffer=new byte[size];
      PrimitiveOracle.note(buffer.length);
      return buffer.length;
    } catch(NegativeArraySizeException e) { return -7; }
  }
  static int accumulate(boolean enabled,int length,int failure) {
    boolean flag=enabled;
    int total=0;
    for(int index=0;index<length;index++) {
      if(index==failure) flag=!flag;
      if(flag) total=total*3+index;
    }
    PrimitiveOracle.note(Boolean.valueOf(flag));
    return total;
  }
  static int branchLocal(boolean enabled,int length,int failure) {
    if(enabled) {
      double value=PrimitiveOracle.read(length);
      PrimitiveOracle.note(value);
      PrimitiveOracle.note(value*2);
    } else {
      int index=failure;
      PrimitiveOracle.note(index);
      PrimitiveOracle.note(index+1);
    }
    int[] result={length,failure};
    return result.length;
  }
  public static void main(String[] args) { PrimitiveOracle.run(); }
}
class PrimitiveOracle {
  static StringBuilder trace;
  static Object context(int length) { trace.append("context;");return Integer.valueOf(length); }
  static boolean check(int failure) { trace.append("check;");if(failure==1)throw new IllegalStateException("flag");return failure!=2; }
  static double read(int length) { trace.append("read;");if(length<0)throw new ArithmeticException("length");return length*0.25; }
  static void note(Object value) { trace.append(value).append(';'); }
  static void run() {
    for(int shape=0;shape<3;shape++)for(boolean enabled:new boolean[]{false,true})for(int length:new int[]{-2,0,1,3,8})for(int failure:new int[]{0,1,2,5}) {
      trace=new StringBuilder();String result;
      try { int value=shape==0 ? PrimitiveReuse.allocate(enabled,length,failure) : shape==1 ? PrimitiveReuse.accumulate(enabled,length,failure) : PrimitiveReuse.branchLocal(enabled,length,failure);result="value:"+value; }
      catch(Throwable e) { result=e.getClass().getSimpleName()+":"+e.getMessage(); }
      System.out.println(shape+":"+enabled+":"+length+":"+failure+":"+result+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
