package javaclassparser

import "testing"

// javac erases the allocation to ArrayInput[] but retains the method's exact
// formal signatures. The local array declaration must carry that evidence to
// the generic factory/identity-map call; the allocation itself stays reifiable.
func TestAdversarialGenericArrayDeclarationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "GenericArrayDeclaration", `import java.util.*;
interface ArrayInput<T> { T read(); }
interface ArrayMapper<A,B> { B map(A value); }
class ArrayFunctions {
  static <T> ArrayMapper<T,T> identity() { return value -> value; }
  static <T> T require(T value) { if(value==null)throw new NullPointerException("source");return value; }
}


class ArraySequence<T> {
  final List<T> values;
  ArraySequence(List<T> values) { this.values=values; }
  static <T> ArraySequence<T> fromArray(T... values) { return new ArraySequence<T>(Arrays.asList(values)); }
  <R> ArraySequence<R> flatten(ArrayMapper<? super T,? extends ArrayInput<? extends R>> mapper) {
    List<R> result=new ArrayList<>();
    for(T value:values) result.add(mapper.map(value).read());
    return new ArraySequence<R>(result);
  }
  public String toString() { return values.toString(); }
  int keep(T witness) { return values.size()+(witness==null?0:1); }
}
class ArrayText implements ArrayInput<String> {
  final String value;
  ArrayText(String value) { this.value=value; }
  public String read() { GenericArrayDeclaration.trace+="S";return value; }
}
class ArrayNumber implements ArrayInput<Integer> {
  final int value;
  ArrayNumber(int value) { this.value=value; }
  public Integer read() { GenericArrayDeclaration.trace+="N";return value; }
}
public class GenericArrayDeclaration<X> {
  static String trace;
  static <T> ArraySequence<T> pair(ArrayInput<? extends T> a,ArrayInput<? extends T> b) {
    ArrayInput<? extends T>[] inputs=new ArrayInput[]{a,b};
    return ArraySequence.fromArray(inputs).flatten(ArrayFunctions.identity());
  }
  static <T extends Number> ArraySequence<T> triple(ArrayInput<? extends T> a,ArrayInput<? extends T> b,ArrayInput<? extends T> c) {
    ArrayInput<? extends T>[] inputs=new ArrayInput[]{a,b,c};
    return ArraySequence.fromArray(inputs).flatten(ArrayFunctions.identity());
  }
  static <X> ArraySequence<X> four(ArrayInput<? extends X> a,ArrayInput<? extends X> b,ArrayInput<? extends X> c,ArrayInput<? extends X> d) {
    ArrayInput<? extends X>[] inputs=new ArrayInput[]{a,b,c,d};
    return ArraySequence.fromArray(inputs).flatten(ArrayFunctions.identity());
  }
  static <T> ArraySequence<ArrayInput<String>> rawResult(ArrayInput<? extends T> a,ArrayInput<? extends T> b) {
    ArrayInput[] inputs=new ArrayInput[]{a,b};
    return ArraySequence.fromArray(inputs);
  }
  static <T> int rawReceiver(ArrayInput<? extends T> a,ArrayInput<? extends T> b) {
    ArrayInput[] inputs=new ArrayInput[]{a,b};
    return ArraySequence.fromArray(inputs).keep(new ArrayText("witness"));
  }
  static <T> ArraySequence<T> inlinePair(ArrayInput<? extends T> a,ArrayInput<? extends T> b) {
    ArrayFunctions.require(a);ArrayFunctions.require(b);
    return ArraySequence.fromArray(a,b).flatten(ArrayFunctions.identity());
  }
  static <T extends Number> ArraySequence<T> inlineTriple(ArrayInput<? extends T> a,ArrayInput<? extends T> b,ArrayInput<? extends T> c) {
    ArrayFunctions.require(a);ArrayFunctions.require(b);ArrayFunctions.require(c);
    return ArraySequence.fromArray(a,b,c).flatten(ArrayFunctions.identity());
  }
  static <X> ArraySequence<X> inlineFour(ArrayInput<? extends X> a,ArrayInput<? extends X> b,ArrayInput<? extends X> c,ArrayInput<? extends X> d) {
    ArrayFunctions.require(a);ArrayFunctions.require(b);ArrayFunctions.require(c);ArrayFunctions.require(d);
    return ArraySequence.fromArray(a,b,c,d).flatten(ArrayFunctions.identity());
  }
  public static void main(String[] args) {
    System.out.print("raw-count:"+rawResult(new ArrayNumber(1),new ArrayNumber(2)).values.size()+";");
    System.out.print("raw-receiver:"+rawReceiver(new ArrayNumber(1),new ArrayNumber(2))+";");
    System.out.print(inlinePair(new ArrayText("a"),new ArrayText("b"))+":"+
        inlineTriple(new ArrayNumber(1),new ArrayNumber(2),new ArrayNumber(3))+":"+
        inlineFour(new ArrayText("w"),new ArrayText("x"),new ArrayText("y"),new ArrayText("z"))+";");
    for(int mask=0;mask<4;mask++) {
      trace="";
      try { System.out.print(pair(new ArrayText("a"),(mask&1)==0?new ArrayText("b"):null)+":"+
          triple(new ArrayNumber(1),new ArrayNumber(2),(mask&2)==0?new ArrayNumber(3):null)+":"+
          four(new ArrayText("w"),new ArrayText("x"),new ArrayText("y"),new ArrayText("z"))+":"); }
      catch(NullPointerException failure) { System.out.print("null:"); }
      System.out.print(trace+";");
    }
  }
}`, Precision, Compatibility, "legacy")
}

// A mutable or heterogeneously parameterized initializer must not acquire
// the positive fixture's homogeneous declaration. Trace and bounds failures
// also check that the alias store keeps its original order and array identity.
func TestAdversarialGenericArrayMutationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "GenericArrayMutation", `
interface MutableInput<T> { T read(); }
class MutableText implements MutableInput<String> {
  final String value;
  MutableText(String value) { this.value=value; }
  public String read() { GenericArrayMutation.trace+="S";return value; }
}
class MutableNumber implements MutableInput<Integer> {
  public Integer read() { GenericArrayMutation.trace+="N";return 42; }
}
public class GenericArrayMutation {
  static String trace;
  static <T> String change(MutableInput<? extends T> a,MutableInput<? extends T> b,MutableInput replacement,int index) {
    MutableInput<? extends T>[] inputs=new MutableInput[]{a,b};
    Object[] alias=inputs;
    try { alias[index]=replacement;trace+="W"; }
    catch(ArrayIndexOutOfBoundsException failure) { trace+="E"; }
    try { return inputs[0].read().toString()+":"+inputs[1].read(); }
    catch(NullPointerException failure) { return "null"; }
  }
  static String mixed(MutableInput<String> a,MutableInput<Integer> b) {
    MutableInput[] inputs=new MutableInput[]{a,b};
    return inputs[0].read()+":"+inputs[1].read();
  }
  public static void main(String[] args) {
    MutableInput[] replacements={new MutableText("z"),null,new MutableNumber()};
    for(int mask=0;mask<6;mask++) {
      MutableInput replacement=replacements[mask%3];
      trace="";
      System.out.print(change(new MutableText("a"),new MutableText("b"),replacement,mask<3?0:2)+":"+trace+";");
    }
    trace="";
    System.out.print(mixed(new MutableText("m"),new MutableNumber())+":"+trace);
  }
}`, Precision, Compatibility, "legacy")
}
