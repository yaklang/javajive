package javaclassparser

import "testing"

func TestAdversarialConditionalObjectVarargsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalObjectVarargs", `
public class ConditionalObjectVarargs {
 static Result select(boolean active, Weight first, Weight second, int fail) {
  return new Result(VarargProbe.mark("P",fail),active ? VarargProbe.collect(first,second) : null,VarargProbe.mark("Q",fail));
 }
 public static void main(String[] args){VarargProbe.main(args);}
}
class Weight {
 final int value;
 Weight(int value){this.value=value;}
}
class Result {
 final String text;
 Result(int prefix,String values,int suffix){text=prefix+":"+values+":"+suffix;}
}
class VarargProbe {
 static String trace="";
 static int mark(String event,int fail){trace+=event;if(event.equals("P")&&fail==1 || event.equals("Q")&&fail==2)throw new IllegalStateException(event);return event.equals("P")?1:2;}
 static String collect(Weight... weights){trace+="C";String value="";for(Weight weight:weights){trace+="W";value+=weight==null?"null,":weight.value+",";}return value;}
 public static void main(String[] args){
  for(boolean active:new boolean[]{false,true})for(int kind:new int[]{0,1,2,3})for(int fail:new int[]{0,1,2}){
   trace="";String value;
   try{value=ConditionalObjectVarargs.select(active,(kind&1)==0?new Weight(3):null,(kind&2)==0?new Weight(7):null,fail).text;}catch(IllegalStateException error){value=error.getMessage();}
   System.out.println(active+":"+kind+":"+fail+":"+value+":"+trace);
  }
 }
}
`, Precision, Compatibility, "legacy")
}
