package javaclassparser

import "testing"

func TestAdversarialErasedSelfBoundArithmeticRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ArithmeticOps", `interface PlainElement<T> {
  T multiply(T other); T multiply(int n); T subtract(T other); int value();
}
interface ExtendedElement<T> extends PlainElement<T> { T multiply(double n); }
final class IntElement implements ExtendedElement<IntElement> {
  final int value;
  IntElement(int value) { this.value=value; }
  public IntElement multiply(IntElement other) { return new IntElement(value*other.value); }
  public IntElement multiply(int n) { return new IntElement(value*n); }
  public IntElement multiply(double n) { return new IntElement((int)(value*n)); }
  public IntElement subtract(IntElement other) { return new IntElement(value-other.value); }
  public int value() { return value; }
}
public class ArithmeticOps<T extends ExtendedElement<T>> {
  final T factor;
  ArithmeticOps(T factor) { this.factor=factor; }
  T field(T value) { return factor.multiply(value); }
  T array(T[] values) { return values[0].multiply(values[1]).subtract(factor); }
  T local(T first,T second) { T value=first.multiply(second);return value.subtract(first); }
  static <T extends ExtendedElement<T>> T method(T first,T second) { return first.multiply(second); }
  public static void main(String[] args) {
    for(int a:new int[]{0,1,-1,23}) {
      ArithmeticOps<IntElement> ops=new ArithmeticOps<IntElement>(new IntElement(3));
      IntElement x=new IntElement(a),y=new IntElement(7);
      System.out.print(ops.field(x).value()+":"+ops.array(new IntElement[]{x,y}).value()+":"+
        ops.local(x,y).value()+":"+method(x,y).value()+";");
    }
  }
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialOverloadedEnumArrayConstructorRoundTrip(t *testing.T) {
	// Enum argument spill recovery currently belongs to the compatibility
	// pipeline. Cover both entry points without claiming Precision support.
	roundTripGenericFlow(t, "EnumOverloads", `public enum EnumOverloads {
    FIRST(new int[]{1,2},new String[0]),
    SECOND(new int[]{3,4},new String[]{"second"}),
    THIRD(5,"third");
    final int[] values;
    final String[] names;
    EnumOverloads(int value,String... names) { this(new int[]{value},names); }
    EnumOverloads(int[] values,String... names) { this.values=values;this.names=names; }
  public static void main(String[] args) {
    for(EnumOverloads code:EnumOverloads.values()) {
      System.out.print(code.name()+":"+java.util.Arrays.toString(code.values)+":"+
        java.util.Arrays.toString(code.names)+";");
    }
  }
}`, Compatibility, "legacy")
}
