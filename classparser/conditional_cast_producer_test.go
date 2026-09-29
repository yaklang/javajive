package javaclassparser

import "testing"

func TestAdversarialConditionalCastProducerRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CastProducer", `import java.util.*;
public class CastProducer {
  static final Object KEY=new Object();
  static int[] option(Map<Object,Object> values) { return values==null ? null : (int[])values.get(KEY); }
  static byte[] copy(boolean selected,byte[] value) { return selected ? (byte[])value.clone() : value; }
  static Object invoke(Object receiver,Object[] args) {
    System.out.print(receiver+":"+Arrays.toString(args)+";"); return args;
  }
  static Object[] parameters={"arg"};
  static Object[] readParameters() { System.out.print("read;");return parameters; }
  static void invokeSelected(int count) { invoke(null,count==0 ? (Object[])null : readParameters()); }
  public static void main(String[] args) {
    Map<Object,Object> values=new HashMap<Object,Object>();
    System.out.print(option(null)+":"+option(values)+";");
    int[] data={1,3,7};values.put(KEY,data);
    if(option(values)!=data)throw new AssertionError("identity");
    System.out.print(Arrays.toString(option(values))+";");
    values.put(KEY,"wrong");
    try {option(values);throw new AssertionError("missing cast");}
    catch(ClassCastException e){System.out.print("cast;");}
    for(boolean selected:new boolean[]{false,true}) {
      byte[] bytes={4,5};byte[] result=copy(selected,bytes);
      System.out.print((result==bytes)+":"+Arrays.toString(result)+";");
      try {System.out.print(copy(selected,null)+";");}
      catch(NullPointerException e){System.out.print("null-copy;");}
    }
    invokeSelected(0);invokeSelected(1);invokeSelected(-1);
  }
}`, Precision, Compatibility, "legacy")
}
