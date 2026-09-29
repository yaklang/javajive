package javaclassparser

import "testing"

func TestAdversarialGenericFactoryInferenceRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "GenericFactories", `import java.util.*;
public class GenericFactories {
  static final List<String> LABELS=Collections.singletonList("label");
  static int calls;
  static String next() { return "v"+(++calls); }
  static List<String> list(String v) { return Collections.singletonList(v); }
  static Set<String> set(String v) { return Collections.singleton(v); }
  static Map<String,String> map(String k,String v) { return Collections.singletonMap(k,v); }
  static List<String> copies(int n,String v) { return Collections.nCopies(n,v); }
  public static void main(String[] args) {
    System.out.print(LABELS+":"+list("x")+":"+set("y")+":"+map("k","v")+":"+copies(3,"z"));
    Map<String,String> ordered=Collections.singletonMap(next(),next());
    if(calls!=2 || !"v2".equals(ordered.get("v1"))) throw new AssertionError("evaluation order");
    List<String> nullable=Collections.singletonList(null);
    Map<String,String> nullMap=Collections.singletonMap(null,null);
    System.out.print(":"+ordered+":"+nullable+":"+nullMap);
    String[] array={"a","b"};
    List<String[]> one=Collections.singletonList(array);
    if(one.size()!=1 || one.get(0)!=array) throw new AssertionError("array identity");
    char[] chars={'a','b'};
    if(!String.valueOf((Object)chars).startsWith("[C@")) throw new AssertionError("Object overload");
    if(!String.valueOf(chars).equals("ab")) throw new AssertionError("array overload");
    System.out.print(":"+String.valueOf((Object)null));
    try { Collections.nCopies(-1,"bad"); throw new AssertionError("negative count"); }
    catch(IllegalArgumentException expected) { System.out.print(":negative"); }
    try { String v=Objects.requireNonNull((String)null,"missing"); throw new AssertionError(v); }
    catch(NullPointerException expected) { System.out.print(":"+expected.getMessage()); }
  }
}`, Precision, Compatibility, "legacy")
}
