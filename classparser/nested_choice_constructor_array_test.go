package javaclassparser

import "testing"

func TestAdversarialNestedChoiceConstructorArrayRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedChoiceArray", `
public class NestedChoiceArray extends ChoiceParent {
 NestedChoiceArray(Number current,Number previous,int index,boolean forward,boolean strict,int fail) {
  super(ChoiceProbe.choose(forward,"F",fail,0) ? (ChoiceProbe.choose(strict,"S",fail,1) ? Choice.A : Choice.B) : (ChoiceProbe.choose(strict,"S",fail,1) ? Choice.C : Choice.D),current,new Object[]{previous,ChoiceProbe.box(index,"A",fail,2),ChoiceProbe.box(index-1,"D",fail,3)});
 }
 public static void main(String[] args){ChoiceProbe.main(args);}
}
interface ChoiceMarker {String text();}
enum Choice implements ChoiceMarker {A,B,C,D;public String text(){return name();}}
class ChoiceParent {
 final String text;
 ChoiceParent(ChoiceMarker choice,Number current,Object... details){ChoiceProbe.trace+="B";if(ChoiceProbe.failBase)throw new IllegalStateException("B");text=choice.text()+":"+current+":"+java.util.Arrays.toString(details);}
}
class ChoiceProbe {
 static String trace="";
 static boolean failBase;
 static boolean choose(boolean value,String event,int fail,int stage){trace+=event;if(fail==stage)throw new IllegalStateException(event);return value;}
 static Integer box(int value,String event,int fail,int stage){trace+=event;if(fail==stage)throw new IllegalStateException(event);return Integer.valueOf(value);}

 public static void main(String[] args){
  for(boolean forward:new boolean[]{false,true})for(boolean strict:new boolean[]{false,true})for(int index:new int[]{Integer.MIN_VALUE,-1,0,1,127,128,Integer.MAX_VALUE})for(boolean nil:new boolean[]{false,true})for(int fail=-1;fail<5;fail++){
   trace="";failBase=fail==4;String result;
   try{NestedChoiceArray value=new NestedChoiceArray(nil?null:Integer.valueOf(7),nil?null:Double.valueOf(3.5),index,forward,strict,fail);result=value.text;}catch(IllegalStateException error){result="error:"+error.getMessage();}
   System.out.println(forward+":"+strict+":"+index+":"+nil+":"+fail+":"+result+":"+trace);
  }
 }
}
`, Precision, Compatibility, "legacy")
}
