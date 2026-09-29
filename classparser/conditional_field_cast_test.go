package javaclassparser

import "testing"

func TestAdversarialConditionalFieldArgumentCastRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "ConditionalFieldCast", `import java.util.*;
class CastKey {
  static final Object KEY=initialize();
  static Object initialize() {ConditionalFieldCast.trace.append("init;");return new Object();}
}
class CastMap extends HashMap<Object,Object> {
  public Object get(Object key) {ConditionalFieldCast.trace.append("get;");return super.get(key);}
}
public class ConditionalFieldCast {
  static StringBuilder trace=new StringBuilder();
  static boolean read(Map<Object,Object> input) {
    for(int i=0;i<2;i++) {
      try {
        Collection values=input==null ? null : (Collection)input.get(CastKey.KEY);
        boolean accepted=values==null || values.contains("a");
        if(accepted) return true;
      } catch(ClassCastException e) {trace.append("bad;");}
    }
    return false;
  }
  public static void main(String[] args) {
    System.out.print(read(null)+":"+trace);
    CastMap map=new CastMap();map.put(CastKey.KEY,Arrays.asList("a"));
    System.out.print(":"+read(map)+":"+trace);
    map.put(CastKey.KEY,Arrays.asList("b"));System.out.print(":"+read(map)+":"+trace);
    map.put(CastKey.KEY,"wrong");System.out.print(":"+read(map)+":"+trace);
  }
}`)
}
