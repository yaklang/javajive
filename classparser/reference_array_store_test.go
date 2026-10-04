package javaclassparser

import "testing"

// The oracle checks JVM AASTORE exception precedence, including an explicit
// CHECKCAST that must remain before the store. An Object[] alias is allowed to
// hold an incompatible value in source; the actual component rejects it later.
func TestAdversarialReferenceArrayStoreRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ReferenceArrayStore", `
public class ReferenceArrayStore<T extends Number> {
  static String trace;
  T[] field;
  ReferenceArrayStore(T[] field) { this.field=field; }
  static String[] array(String[] value,int failure) {
    trace+="A";if(failure==1)throw new IllegalStateException("array");return value;
  }
  static int index(int value,int failure) {
    trace+="I";if(failure==2)throw new IllegalArgumentException("index");return value;
  }
  static Object value(Object value,int failure) {
    trace+="V";if(failure==3)throw new UnsupportedOperationException("value");return value;
  }
  static String store(String[] input,int position,Object replacement,int failure) {
    trace="";
    try {
      Object[] alias=array(input,failure);
      alias[index(position,failure)]=value(replacement,failure);
      trace+="W";
      return "ok:"+trace;
    } catch(Throwable error) { return error.getClass().getName()+":"+trace; }
  }
  static <N extends Number> String bounded(N[] input,int position,Object replacement) {
    trace="";
    try {
      Object[] alias=input;
      alias[index(position,0)]=value(replacement,0);
      trace+="W";
      return "ok:"+trace;
    } catch(Throwable error) { return error.getClass().getName()+":"+trace; }
  }
  String fieldStore(int position,Object replacement) {
    trace="";
    try {
      Object[] alias=field;
      alias[index(position,0)]=value(replacement,0);
      trace+="W";
      return "ok:"+trace;
    } catch(Throwable error) { return error.getClass().getName()+":"+trace; }
  }
  static String nested(Number[][] input,int position,Object replacement) {
    trace="";
    try {
      Object[] alias=input;
      alias[index(position,0)]=value(replacement,0);
      trace+="W";
      return "ok:"+trace;
    } catch(Throwable error) { return error.getClass().getName()+":"+trace; }
  }
  static String explicit(String[] input,int position,Object replacement) {
    trace="";
    try {
      Object[] alias=input;
      alias[index(position,0)]=(String)value(replacement,0);
      trace+="W";
      return "ok:"+trace;
    } catch(Throwable error) { return error.getClass().getName()+":"+trace; }
  }
  public static void main(String[] args) { ReferenceStoreOracle.main(args); }
}
// Keep the matrix driver on the original classpath. Every call still resolves
// to the rebuilt ReferenceArrayStore, so this tests stores without spending
// regression time decompiling an unrelated four-deep loop used only as a driver.
class ReferenceStoreOracle {
  public static void main(String[] args) {
    Object[] strings={null,"text",Integer.valueOf(7),new Object()};
    Object[] numbers={null,Integer.valueOf(3),Double.valueOf(2.5),"wrong"};
    Object[] rows={null,new Integer[]{1},new int[]{2},new Object()};
    for(int state=0;state<3;state++) {
      for(int position=-1;position<=1;position++) {
        for(int choice=0;choice<4;choice++) {
          for(int failure=0;failure<4;failure++) {
            String[] input=state==0?null:(state==1?new String[0]:new String[]{"old"});
            System.out.println("s:"+state+":"+position+":"+choice+":"+failure+":"+ReferenceArrayStore.store(input,position,strings[choice],failure)+":"+java.util.Arrays.toString(input));
          }
          Integer[] input=state==0?null:(state==1?new Integer[0]:new Integer[]{9});
          System.out.println("b:"+state+":"+position+":"+choice+":"+ReferenceArrayStore.bounded(input,position,numbers[choice])+":"+java.util.Arrays.toString(input));
          ReferenceArrayStore<Integer> holder=new ReferenceArrayStore<Integer>(input);
          System.out.println("f:"+state+":"+position+":"+choice+":"+holder.fieldStore(position,numbers[choice])+":"+java.util.Arrays.toString(input));
          Number[][] matrix=state==0?null:(state==1?new Number[0][]:new Number[][]{new Integer[]{5}});
          System.out.println("m:"+state+":"+position+":"+choice+":"+ReferenceArrayStore.nested(matrix,position,rows[choice])+":"+java.util.Arrays.deepToString(matrix));
          String[] castInput=state==0?null:(state==1?new String[0]:new String[]{"old"});
          System.out.println("c:"+state+":"+position+":"+choice+":"+ReferenceArrayStore.explicit(castInput,position,strings[choice])+":"+java.util.Arrays.toString(castInput));
        }
      }
    }
  }
}`, Precision, Compatibility, "legacy")
}
