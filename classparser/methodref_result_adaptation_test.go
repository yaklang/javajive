package javaclassparser

import "testing"

func TestAdversarialMethodReferenceResultAdaptationRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "MethodRefResult", `import java.util.*;
import java.util.function.*;
import java.util.stream.*;
class RefCell<A,B> {
  final A value;
  RefCell(A value) { this.value=value; }
  A read() { return value; }
}
public class MethodRefResult {
  static Function<Optional<String>,String> getter() { return Optional::get; }
  static Function<Map.Entry<String,Integer>,String> key() { return Map.Entry::getKey; }
  static ToIntFunction<Optional<Integer>> number() { return Optional::get; }
  static String pipeline(List<Optional<String>> inputs) {
    return inputs.stream().map(Optional::get).collect(Collectors.joining(","));
  }
  static String keys(List<Map.Entry<String,Integer>> inputs) {
    return inputs.stream().map(Map.Entry::getKey).collect(Collectors.joining(","));
  }
  static int numbers(List<Optional<Integer>> inputs) {
    return inputs.stream().mapToInt(Optional::get).sum();
  }
  static String cells(List<RefCell<String,Integer>> inputs) {
    return inputs.stream().map(RefCell::read).collect(Collectors.joining(","));
  }
  static long discarded(List<Optional<String>> inputs) {
    // filter prevents size-based count shortcuts. Each getter runs, but its
    // erased result is discarded without an added String checkcast.
    return inputs.stream().filter(x -> true).map(Optional::get).count();
  }
  public static void main(String[] args) {
    Function<Optional<String>,String> f=getter();
    System.out.print(pipeline(Arrays.asList(Optional.of("a"),Optional.of("b")))+":");
    System.out.print(f.apply(Optional.of("ok"))+":"+key().apply(new AbstractMap.SimpleEntry<>("key",7))+":"+number().applyAsInt(Optional.of(9)));
    System.out.print(":"+keys(Arrays.asList(new AbstractMap.SimpleEntry<>("k",1)))+":"+numbers(Arrays.asList(Optional.of(2),Optional.of(3)))+":"+cells(Arrays.asList(new RefCell<String,Integer>("cell"))));
    System.out.print(":"+discarded((List)Arrays.asList(Optional.of(1),Optional.of(2))));
    try { f.apply(Optional.empty()); } catch(NoSuchElementException e) { System.out.print(":empty"); }
    try { ((Function)f).apply(Optional.of(1)); } catch(ClassCastException e) { System.out.print(":cast"); }
  }
}`, Precision, Compatibility, "legacy")
}
