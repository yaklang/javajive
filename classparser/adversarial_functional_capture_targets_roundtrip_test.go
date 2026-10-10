package javaclassparser

import "testing"

// Assertions and all input construction remain in the original driver. The
// target receives valid wildcard containers, including a lower-bounded one
// holding values outside that lower bound. No invented narrowing is allowed.
func TestAdversarialFunctionalCaptureTargetsPreserveValuesAndBindingRoundTrip(t *testing.T) {
	fixture := `public class CaptureTargetOwner {
 public static Object[] unbounded(Iterable<?> values){java.util.List<Object> out=new java.util.ArrayList<Object>();values.forEach(v->out.add(v));return out.toArray();}
 public static long upper(Iterable<? extends Number> values){java.util.concurrent.atomic.AtomicLong out=new java.util.concurrent.atomic.AtomicLong();values.forEach(v->{if(v!=null)out.addAndGet(v.longValue());});return out.get();}
 public static Object[] lower(Iterable<? super Integer> values){java.util.List<Object> out=new java.util.ArrayList<Object>();values.forEach(v->out.add(v));return out.toArray();}
 public static int entries(java.util.Map<?,?> values){java.util.concurrent.atomic.AtomicInteger out=new java.util.concurrent.atomic.AtomicInteger();values.forEach((k,v)->out.addAndGet(k==v?7:3));return out.get();}
}
class CaptureTargetCheck {public static void main(String[] args){
 Object token=new Object();java.util.List<Object> values=java.util.Arrays.asList(null,token,"outside-lower-bound",Integer.MIN_VALUE,Long.MAX_VALUE);
 Object[] a=CaptureTargetOwner.unbounded(values),b=CaptureTargetOwner.lower(values);int rows=0;
 if(a.length!=values.size()||b.length!=values.size())throw new AssertionError("captured container length");
 for(int i=0;i<values.size();i++){if(a[i]!=values.get(i)||b[i]!=values.get(i))throw new AssertionError("captured payload identity");rows++;}
 for(java.util.List<Number> numbers:java.util.Arrays.asList(java.util.Arrays.<Number>asList(),java.util.Arrays.<Number>asList(null,Integer.MIN_VALUE,Long.MAX_VALUE,17L))){long expected=0;for(Number n:numbers)if(n!=null)expected+=n.longValue();if(CaptureTargetOwner.upper(numbers)!=expected)throw new AssertionError("upper bound / overflow / binding");rows++;}
 java.util.Map<Object,Object> map=new java.util.LinkedHashMap<Object,Object>();map.put(null,null);map.put(token,token);map.put("key",Long.MAX_VALUE);int expected=0;for(java.util.Map.Entry<Object,Object> e:map.entrySet())expected+=e.getKey()==e.getValue()?7:3;
 if(CaptureTargetOwner.entries(map)!=expected)throw new AssertionError("independent captures / identity");rows++;
 System.out.println(rows+":capture:unbounded:upper:lower:identity:binding:overflow");
}}`
	testNativePrivateSetterSourceFixture(t, map[string]string{"CaptureTargetOwner.java": fixture}, "CaptureTargetOwner", "CaptureTargetCheck", "8:capture:unbounded:upper:lower:identity:binding:overflow\n")
}
