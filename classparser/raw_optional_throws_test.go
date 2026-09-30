package javaclassparser

import "testing"

// A raw Optional erases the method-scoped X in orElseThrow's throws clause as
// well as its value T. Restoring Optional<?> keeps exception inference without
// inventing a value type or changing receiver/argument evaluation order.
func TestAdversarialRawOptionalGenericThrowsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "OptionalThrowsProbe", `import java.util.*;
import java.util.function.*;
import java.util.stream.*;
import java.io.*;
public class OptionalThrowsProbe {
  String unchecked(String input,int mode) {
    Optional<String> value=mode==1 ? null : Optional.ofNullable(input);
    Supplier<IllegalStateException> fault=ThrowsOracle.unchecked(mode);
    return value.orElseThrow(fault);
  }
  String checked(String input,int mode) throws IOException {
    Optional<String> value=mode==1 ? null : Optional.ofNullable(input);
    Supplier<IOException> fault=ThrowsOracle.checked(mode);
    return value.orElseThrow(fault);
  }
  String first(List<List<String>> values,int mode) {
    Function<List<String>,Stream<String>> mapper=List::stream;
    Supplier<IllegalStateException> fault=ThrowsOracle.unchecked(mode);
    return values.stream().flatMap(mapper).findFirst().orElseThrow(fault);
  }
  String rawPayload(int mode) {
    Supplier raw=ThrowsOracle.polluted(mode);
    Object value=raw.get();
    return value==null ? "null" : value.toString();
  }
  public static void main(String[] args) {ThrowsOracle.run();}
}
class ThrowsOracle {
  static StringBuilder trace;
  static Supplier<IllegalStateException> unchecked(int mode) {
    trace.append("supplier;");
    return mode==2 ? null : () -> {
      trace.append("get;");if(mode==3)throw new UnsupportedOperationException("get");
      return mode==4 ? null : new IllegalStateException("empty");
    };
  }
  static Supplier<IOException> checked(int mode) {
    trace.append("supplier;");
    return mode==2 ? null : () -> {
      trace.append("get;");if(mode==3)throw new UnsupportedOperationException("get");
      return mode==4 ? null : new IOException("empty");
    };
  }
  static Supplier<IllegalStateException> polluted(int mode) {
    Supplier raw=() -> mode==0 ? "payload" : mode==1 ? new IOException("checked") : mode==2 ? null : new IllegalStateException("runtime");
    return raw;
  }
  static void run() {
    OptionalThrowsProbe p=new OptionalThrowsProbe();
    for(int method=0;method<2;method++)for(String input:new String[]{"value","",null})for(int mode=0;mode<5;mode++) {
      trace=new StringBuilder();String result;
      try {result=method==0 ? p.unchecked(input,mode) : p.checked(input,mode);}catch(Exception ex){result=ex.getClass().getName();}
      System.out.println(method+":"+input+":"+mode+":"+result+":"+trace);
    }
    List[] rows={Collections.emptyList(),Arrays.asList(Arrays.asList("a","b")),null,Arrays.asList((Object)null),Arrays.asList("wrong")};
    for(int row=0;row<rows.length;row++)for(int mode=0;mode<5;mode++) {
      trace=new StringBuilder();String result;
      try {result=p.first(rows[row],mode);}catch(Exception ex){result=ex.getClass().getName();}
      System.out.println("first:"+row+":"+mode+":"+result+":"+trace);
    }
    for(int mode=0;mode<4;mode++) {
      String result;try {result=p.rawPayload(mode);}catch(Exception ex){result=ex.getClass().getName();}
      System.out.println("raw-payload:"+mode+":"+result);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
