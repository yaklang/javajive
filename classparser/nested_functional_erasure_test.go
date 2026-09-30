package javaclassparser

import "testing"

// The SAM descriptor records List/Stream/Optional erasure, not their nested
// arguments. A typed method reference must keep its creation target, while the
// invocation accepts the same function object through its descriptor erasure.
func TestAdversarialNestedFunctionalErasureRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedFunctionalErasure", `import java.util.*;
import java.util.function.*;
import java.util.stream.*;
interface PipelineMarker {}
class PipelineMetadata {
  <R> List<R> expand(List<List<String>> values,Function<List<String>,Stream<R>> function) {
    return values.stream().flatMap(function).collect(Collectors.toList());
  }
}
public class NestedFunctionalErasure {
  List<String> flatten(List<List<String>> values) {
    Function<List<String>,Stream<String>> mapper=List::stream;
    return values.stream().flatMap(mapper).collect(Collectors.toList());
  }
  List<String> lambda(List<List<String>> values) {
    Function<List<String>,Stream<String>> mapper=value -> PipelineOracle.expand(value);
    return values.stream().flatMap(mapper).collect(Collectors.toList());
  }
  List<String> marked(List<List<String>> values) {
    Function<List<String>,Stream<String>> mapper=(Function<List<String>,Stream<String>> & PipelineMarker) List::stream;
    PipelineOracle.trace.append(mapper instanceof PipelineMarker).append(';');
    return values.stream().flatMap(mapper).collect(Collectors.toList());
  }
  List<String> external(List<List<String>> values) {
    Function<List<String>,Stream<String>> mapper=List::stream;
    return new PipelineMetadata().expand(values,mapper);
  }
  Optional<String> optional(Optional<String> value) {
    Function<String,Optional<String>> mapper=PipelineOracle::wrap;
    return value.flatMap(mapper);
  }
  List<String> finishList(List<String> values) {
    Function<List<String>,List<String>> finisher=Collections::unmodifiableList;
    return values.stream().collect(Collectors.collectingAndThen(Collectors.toList(),finisher));
  }
  Set<String> finishSet(List<String> values) {
    Function<Set<String>,Set<String>> finisher=Collections::unmodifiableSet;
    return values.stream().collect(Collectors.collectingAndThen(Collectors.toSet(),finisher));
  }
  public static void main(String[] args) {PipelineOracle.run();}
}
class PipelineOracle {
  static StringBuilder trace;
  static Stream<String> expand(List<String> value) {
    trace.append("expand;");if(value==null) throw new IllegalStateException("null group");
    return value.stream();
  }
  static Optional<String> wrap(String value) {
    trace.append("wrap;");if(value.equals("throw")) throw new IllegalArgumentException("value");
    return value.equals("null") ? null : Optional.of(value+"!");
  }
  static void run() {
    NestedFunctionalErasure pipeline=new NestedFunctionalErasure();
    List[] inputs={Collections.emptyList(),Arrays.asList(Arrays.asList("a","b"),Arrays.asList("c")),Arrays.asList(Arrays.asList("x",null)),Arrays.asList((Object)null),null,Arrays.asList(Arrays.asList("a"),"wrong"),Arrays.asList(Collections.emptyList(),Arrays.asList("last"))};
    for(int method=0;method<4;method++) for(int row=0;row<inputs.length;row++) {
      trace=new StringBuilder();String outcome;
      try {
        List<String> result=method==0 ? pipeline.flatten(inputs[row]) : method==1 ? pipeline.lambda(inputs[row]) : method==2 ? pipeline.marked(inputs[row]) : pipeline.external(inputs[row]);
        outcome=result.toString();
      } catch (RuntimeException ex) {outcome=ex.getClass().getSimpleName();}
      System.out.println(method+":"+row+":"+outcome+":"+trace);
    }
    for(String value:new String[]{"x","throw","null",null}) {
      trace=new StringBuilder();String outcome;
      try {outcome=pipeline.optional(Optional.ofNullable(value)).toString();} catch(RuntimeException ex) {outcome=ex.getClass().getSimpleName();}
      System.out.println("optional:"+value+":"+outcome+":"+trace);
    }
    for(int shape=0;shape<2;shape++) for(int row=0;row<3;row++) {
      List<String> input=row==0 ? Collections.emptyList() : row==1 ? Arrays.asList("b","a","b",null) : null;
      String outcome;
      try {
        Collection<String> result=shape==0 ? pipeline.finishList(input) : pipeline.finishSet(input);
        List<String> sorted=new ArrayList<>(result);sorted.sort(Comparator.nullsFirst(Comparator.naturalOrder()));
        try {result.add("bad");outcome="mutable";} catch(UnsupportedOperationException ex) {outcome="immutable:"+sorted;}
      } catch(RuntimeException ex) {outcome=ex.getClass().getSimpleName();}
      System.out.println("finisher:"+shape+":"+row+":"+outcome);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
