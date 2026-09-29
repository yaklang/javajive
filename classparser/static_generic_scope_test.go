package javaclassparser

import "testing"

func TestAdversarialStaticFactoryGenericScopeRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "ScopedFactory", `public class ScopedFactory<U> {
  final U value;
  ScopedFactory(U value) { this.value=value; }
  static ScopedFactory<String> text(String value) { return new ScopedFactory<String>(value); }
  static ScopedFactory<char[]> array(char[] value) { return new ScopedFactory<char[]>(value); }
  static <U> ScopedFactory<U> generic(U value) { return new ScopedFactory<U>(value); }
  public static void main(String[] args) {
    char[] data={'a','b'};
    if(array(data).value!=data) throw new AssertionError("identity");
    System.out.print(text("ok").value+":"+generic(Integer.valueOf(42)).value+":"+array(data).value.length);
  }
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialRawGenericHierarchyScopeRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "RawHierarchy", `class RawParent<X> {
  X value;
  void accept(X value) { this.value=value; }
  X get() { return value; }
}

class FixedHierarchy<X> extends RawParent<String> {
  static Object raw(FixedHierarchy receiver,Object value) { receiver.accept(value); return receiver.get(); }
}
public class RawHierarchy<X> extends RawParent<X> {
  static Object raw(RawHierarchy receiver,Object value) { receiver.accept(value); return receiver.get(); }
  static <X> X typed(RawHierarchy<X> receiver,X value) { receiver.accept(value); return receiver.get(); }
  public static void main(String[] args) {
    Object value=new Object();
    RawHierarchy receiver=new RawHierarchy();
    if(raw(receiver,value)!=value) throw new AssertionError("raw identity");
    if(FixedHierarchy.raw(new FixedHierarchy(),value)!=value) throw new AssertionError("raw supertype identity");
    System.out.print(raw(receiver,"ok")+":"+typed(new RawHierarchy<Integer>(),Integer.valueOf(42)));
  }
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialGenericFactoryConstructorBindingRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "FactoryOverloads", `public class FactoryOverloads<U> {
  final String chosen;
  FactoryOverloads(U value) { chosen="generic"; }
  FactoryOverloads(String value) { chosen="string"; }
  static FactoryOverloads<Object> value(String value) { return new FactoryOverloads<Object>((Object)value); }
  static FactoryOverloads<Object> empty() { return new FactoryOverloads<Object>((Object)null); }
  public static void main(String[] args) {
    System.out.print(value("x").chosen+":"+empty().chosen+":"+new FactoryOverloads<Object>("x").chosen);
  }
}`, Precision, Compatibility, "legacy")
}
