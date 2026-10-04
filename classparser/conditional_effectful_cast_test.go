package javaclassparser

import "testing"

// The cast and its producer belong to the selected arm. An implicit stack
// temporary may disappear only if both the call and checkcast stay on that arm.
func TestAdversarialConditionalEffectfulCastRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalCast", `import java.util.*;
public class ConditionalCast {
  static int calls;
  static Object read(List<Object> values) { calls++;return values.get(1); }
  static String choose(List<Object> values) {
    return values.size()==1 ? null : (String)read(values);
  }
  static void consume(String prefix,String value) { System.out.print(prefix+":"+value+":"+calls+";"); }
  static void loop(List<List<Object>> rows) {
    for(List<Object> row:rows) {
      consume((String)row.get(0),row.size()==1 ? null : (String)read(row));
    }
  }
  public static void main(String[] args) {
    List<Object> one=Arrays.<Object>asList("a");
    List<Object> two=Arrays.<Object>asList("b","c");
    List<Object> bad=Arrays.<Object>asList("d",Integer.valueOf(7));
    calls=0;loop(Arrays.asList(one,two));
    System.out.print(choose(one)+":"+calls+";");
    System.out.print(choose(two)+":"+calls+";");
    try { choose(bad); } catch(ClassCastException e) { System.out.print("cast:"+calls+";"); }
    try { choose(Collections.emptyList()); } catch(IndexOutOfBoundsException e) { System.out.print("index:"+calls+";"); }
  }
}`)
}
