package javaclassparser

import "testing"

// The local's reaching definitions require Object, although the selected
// invocation physically returns Number. The caller's unconstrained Class<T>
// must not become the callee's bounded N merely because both are source formals.
// The unchanged driver observes lazy guards, payload identity, overloads and
// the callee's original Class.cast, including a deliberately polluted witness.
func TestAdversarialErasedBoundedResultInWidenedLocalRoundTrip(t *testing.T) {
	fixture := `class BoundedResultOps {
 static String trace="";
 static <N extends Number> N choose(Number value,Class<N> type){trace+="G";return type.cast(value);}
 static Number choose(Integer value,Object type){trace+="X";return Integer.valueOf(-1000);}
}
public class WidenedResultOwner {
 public static <T> Object convert(Object input,Class<T> type,boolean bypass,boolean suffix){
  Object result=input;
  if(!bypass && result instanceof Number && Number.class.isAssignableFrom(type))result=BoundedResultOps.choose((Number)result,(Class)type);
  if(suffix)result="suffix";
  return result;
 }
}
class WidenedResultDriver {
 static void check(Object input,Class type,boolean bypass,boolean suffix,Object expected,String trace){
  BoundedResultOps.trace="";Object got=WidenedResultOwner.convert(input,type,bypass,suffix);
  if(got!=expected||!BoundedResultOps.trace.equals(trace))throw new AssertionError("identity/order/overload:"+BoundedResultOps.trace);
 }
 public static void main(String[] args){
  Number number=Integer.valueOf(42);Object marker=new Object();
  check(number,Integer.class,false,false,number,"G");check(number,Number.class,false,false,number,"G");
  check(number,Integer.class,false,true,"suffix","G");check(number,Double.class,true,false,number,"");
  check(marker,null,false,false,marker,"");check(null,null,false,false,null,"");check(number,String.class,false,false,number,"");
  BoundedResultOps.trace="";try{WidenedResultOwner.convert(number,Double.class,false,false);throw new AssertionError("lost callee cast");}catch(ClassCastException expected){if(!BoundedResultOps.trace.equals("G"))throw new AssertionError("moved check");}
  BoundedResultOps.trace="";try{WidenedResultOwner.convert(number,null,false,false);throw new AssertionError("lost null guard");}catch(NullPointerException expected){if(!BoundedResultOps.trace.isEmpty())throw new AssertionError("eager call");}
  System.out.println("9:widened:bounded:identity:lazy:overload:original-check");
 }
}`
	testNativePrivateSetterSourceFixture(t, map[string]string{"WidenedResultOwner.java": fixture}, "WidenedResultOwner", "WidenedResultDriver", "9:widened:bounded:identity:lazy:overload:original-check\n")
}
