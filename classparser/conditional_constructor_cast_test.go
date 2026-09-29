package javaclassparser

import "testing"

func TestAdversarialConditionalConstructorCastRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalNumber", `import java.math.BigDecimal;
class ConstructorOperand {
  ConstructorOperand(int first,String value) { ConditionalNumber.trace.append("C"); }
}
public class ConditionalNumber {
  static StringBuilder trace=new StringBuilder();
  final Object value;
  ConditionalNumber(Object value) { this.value=value; }
  Number number() { return value instanceof String ? new BigDecimal((String)value) : (Number)value; }
  static int first() { trace.append("F");return 1; }
  static Object read(Object value) { trace.append("R");return value; }
  static Object choose(boolean construct,Object value) {
    return construct ? new ConstructorOperand(first(),(String)read(value)) : (Number)value;
  }
  static String earlier(boolean selected,Object value) {
    String saved=(String)read(value);
    return selected ? saved : null;
  }
  static String earlierPure(boolean selected,Object value) {
    String saved=(String)value;
    return selected ? saved : null;
  }
  public static void main(String[] args) {
    for(Object value:new Object[]{"1.5","-42","bad",Integer.valueOf(7),Double.valueOf(-0.0),null,new Object()}) {
      try {
        Number result=new ConditionalNumber(value).number();
        if(result==null) System.out.print("null;");
        else System.out.print(result.getClass().getSimpleName()+":"+result+";");
      } catch(RuntimeException e) { System.out.print(e.getClass().getSimpleName()+";"); }
    }
    for(boolean construct:new boolean[]{false,true}) {
      for(Object value:new Object[]{"ok",Integer.valueOf(7),null}) {
        trace.setLength(0);
        try {
          Object result=choose(construct,value);
          System.out.print((result==null ? "null" : result.getClass().getSimpleName())+":"+trace+";");
        } catch(ClassCastException e) { System.out.print("cast:"+trace+";"); }
        trace.setLength(0);
        try { System.out.print(earlier(construct,value)+":"+trace+";"); }
        catch(ClassCastException e) { System.out.print("early-cast:"+trace+";"); }
        try { System.out.print(earlierPure(construct,value)+";"); }
        catch(ClassCastException e) { System.out.print("early-pure-cast;"); }
      }
    }
  }
}`, Precision, Compatibility, "legacy")
}
