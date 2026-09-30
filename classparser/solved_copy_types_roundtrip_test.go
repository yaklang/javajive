package javaclassparser

import "testing"

func TestAdversarialSolvedCopyTypesRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SolvedCopyTypes", `import java.util.*;
public class SolvedCopyTypes {
  static int sum(Map values,boolean enabled) {
    int[] selected=enabled ? (int[])values.get("array") : null;
    int[] copy=selected;
    int result=0;
    if(copy!=null)for(int value:copy)result=result*31+value;
    return result;
  }
  static int next(Iterator nodes) {
    while(nodes.hasNext()) {
      CopyNode value=(CopyNode)nodes.next();
      if(value instanceof CopyPred)return value.number;
    }
    return -1;
  }
  static int stack(CopyReader reader,Throwable failure) {
    StackTraceElement[] values=(StackTraceElement[])reader.read(StackTraceElement[].class);
    StackTraceElement[] copy=values;
    if(copy!=null)failure.setStackTrace(copy);
    return failure.getStackTrace().length;
  }
  public static void main(String[] args){CopyOracle.run();}
}
class CopyNode {final int number;CopyNode(int number){this.number=number;}}
class CopyPred extends CopyNode {CopyPred(int number){super(number);}}
class CopyOther extends CopyNode {CopyOther(int number){super(number);}}
class CopyReader {
  final Object value;CopyReader(Object value){this.value=value;}
  <T> T read(Class<T> type){return (T)value;}
}
class CopyOracle {
  static String failure(Throwable value){return value.getClass().getSimpleName();}
  static void run(){
    for(int seed=-6;seed<=6;seed++)for(int length=0;length<=5;length++) {
      int[] array=new int[length];for(int i=0;i<length;i++)array[i]=seed*(i+1);
      Map values=new HashMap();values.put("array",array);
      System.out.println("array:"+seed+":"+length+":"+SolvedCopyTypes.sum(values,true)+":"+SolvedCopyTypes.sum(values,false));
      List nodes=new ArrayList();for(int i=0;i<length;i++)nodes.add(new CopyOther(seed+i));nodes.add(new CopyPred(seed));
      System.out.println("node:"+seed+":"+length+":"+SolvedCopyTypes.next(nodes.iterator()));
      StackTraceElement[] frames=new StackTraceElement[length];for(int i=0;i<length;i++)frames[i]=new StackTraceElement("C","m","C.java",seed+i);
      Throwable t=new IllegalArgumentException();t.setStackTrace(new StackTraceElement[0]);
      System.out.println("stack:"+seed+":"+length+":"+SolvedCopyTypes.stack(new CopyReader(frames),t));
    }
    for(Object value:new Object[]{null,"wrong",new Object[0],new int[0]}) {
      Map values=new HashMap();values.put("array",value);
      try{System.out.println("bad-array:"+SolvedCopyTypes.sum(values,true));}catch(Throwable t){System.out.println("bad-array:"+failure(t));}
      try{System.out.println("bad-stack:"+SolvedCopyTypes.stack(new CopyReader(value),new Throwable()));}catch(Throwable t){System.out.println("bad-stack:"+failure(t));}
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
