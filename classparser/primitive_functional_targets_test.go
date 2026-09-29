package javaclassparser

import "testing"

func TestAdversarialPrimitiveFunctionalTargetsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "PrimitiveFunctionalTargets", `import java.util.*;
import java.util.function.*;
public class PrimitiveFunctionalTargets {
  static long apply(List<String> values) {
    ToLongFunction<String> length=s -> (long)s.length();
    return values.stream().mapToLong(length).sum();
  }
  static double applyDouble(List<Double> values) {
    ToDoubleFunction<Double> value=Double::doubleValue;
    return values.stream().mapToDouble(value).sum();
  }
  static String generated(int count) {
    IntFunction<String[]> make=n -> new String[n];
    ToIntBiFunction<String,String> both=(a,b) -> a.length()+b.length();
    ToLongBiFunction<String,String> big=(a,b) -> (long)a.length()*b.length();
    ToDoubleBiFunction<String,String> fraction=(a,b) -> (double)a.length()/b.length();
    ObjIntConsumer<StringBuilder> repeat=(out,n) -> out.append(n);
    StringBuilder out=new StringBuilder();
    repeat.accept(out,count);
    return make.apply(count).length+":"+both.applyAsInt("abc","x")+":"+big.applyAsLong("abc","xx")+":"+fraction.applyAsDouble("abc","xx")+":"+out;
  }
  public static void main(String[] args) {
    System.out.print(apply(Arrays.asList("","ab","xyz"))+":"+applyDouble(Arrays.asList(0.5,1.25))+":"+generated(2));
  }
}`)
}
