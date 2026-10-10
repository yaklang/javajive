package javaclassparser

import "testing"

func TestAdversarialConditionalArrayValueRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalArrayValue", `import java.util.Arrays;
public class ConditionalArrayValue {
  static StringBuilder trace=new StringBuilder();
  static String item(String value) { trace.append("item;"); return value; }
  static String[] wrap(boolean enabled,String text) {
    if (enabled && text.length()>0) {
      String value=item(text.equals("nil") ? null : text);
      String[] result=value==null ? null : new String[]{value};
      trace.append("done;");
      return result;
    }
    return null;
  }
  static int element(int value) { trace.append(value).append(';'); if(value==7) throw new IllegalArgumentException("seven"); return value; }
  static int[] choose(boolean a,boolean b,int value) {
    return a || b ? new int[]{element(value),element(value+1)} : new int[]{element(-value)};
  }
  static String[][] nested(boolean a,String text) {
    return a ? new String[][]{new String[]{text},null} : null;
  }
  public static void main(String[] args) { ConditionalArrayValueOracle.main(args); }
}
class ConditionalArrayValueOracle {
  public static void main(String[] args) {
    System.out.print(Arrays.toString(ConditionalArrayValue.wrap(false,"x"))+":"+Arrays.toString(ConditionalArrayValue.wrap(true,""))+":"+Arrays.toString(ConditionalArrayValue.wrap(true,"nil"))+":"+Arrays.toString(ConditionalArrayValue.wrap(true,"x"))+":"+ConditionalArrayValue.trace);
    for(int mask=0;mask<4;mask++) {
      int[] values=ConditionalArrayValue.choose((mask&1)!=0,(mask&2)!=0,mask);
      System.out.print(":"+Arrays.toString(values));
    }
    System.out.print(":"+Arrays.deepToString(ConditionalArrayValue.nested(true,"ConditionalArrayValue.nested"))+":"+Arrays.deepToString(ConditionalArrayValue.nested(false,"none"))+":"+ConditionalArrayValue.trace);
    try { ConditionalArrayValue.choose(true,false,6); } catch(IllegalArgumentException e) { System.out.print(":"+e.getMessage()+":"+ConditionalArrayValue.trace); }
    String[] left=ConditionalArrayValue.wrap(true,"same"),right=ConditionalArrayValue.wrap(true,"same");
    left[0]="changed";
    System.out.print(":"+(left==right)+":"+right[0]+":"+Arrays.toString(ConditionalArrayValue.choose(false,false,Integer.MIN_VALUE)));
  }
}`)
}

func TestAdversarialNegationPreservesValueDependenciesRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NegationDependencies", `public class NegationDependencies {
  static int negate(int a,int b) { int before=a+b; return -before; }
  static long negate(long value) { return -(-value); }
  static int negate(float value) { return Float.floatToIntBits(-value); }
  static long negate(double a,double b) { double value=a+b;return Double.doubleToLongBits(-value); }
  public static void main(String[] args) {
    System.out.print(negate(3,9)+":"+negate(Integer.MAX_VALUE,1)+":"+negate(Long.MIN_VALUE)+":"+negate(0.0f)+":"+negate(-0.0f)+":"+negate(0.0,0.0)+":"+negate(Double.NaN,1.0));
  }
}`)
}
