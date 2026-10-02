package javaclassparser

import "testing"

// Branch conditions use nonzero truth. Numeric reuse retains the producer's
// original word. The driver checks the short-circuit trace independently,
// including failures before later conditions and two different consumers.
func TestAdversarialNestedDecisionConditionConsumers(t *testing.T) {
	roundTripGenericFlowUnits(t, "DecisionConditionAlgorithm", `
class DecisionConditionEvents {
 String trace=""; final RuntimeException failure=new IllegalStateException("gate"); int failAt;
 DecisionConditionEvents(int fail){failAt=fail;}
 boolean gate(int index,boolean value){trace+=index;if(index==failAt)throw failure;return value;}
}
public class DecisionConditionAlgorithm {
 public static void main(String[]args){DecisionConditionDriver.run();}
 static int word(DecisionConditionEvents e,boolean a,int word){
  int value=e.gate(0,a)?word:0;
  int flag=value!=0?1:0;
  return value*7+flag;
 }
 static int select(DecisionConditionEvents e,boolean a,boolean b,boolean c) {
  int word=(e.gate(0,a)||e.gate(1,b)||e.gate(2,c))?1:0;
  int result=word*7;
  if(word!=0) result+=3; else result-=2;
  return result+word;
 }
 static int ordered(long a,long b,long c,long d){
  int result=0;
  if(c>0){int word=(a>0||b>0)?1:0;if(word!=0&&d==0)result+=3;else if(word!=0)result+=7;result+=word;}
  if(d>0){int word=(a>0||b>0||c>0)?1:0;if(word!=0)result+=11;result+=word*2;}
  return result;
 }
}
class DecisionConditionDriver {
 static void run(){
  for(int word:new int[]{-2,0,2,3,7})for(boolean truth:new boolean[]{false,true}){
   DecisionConditionEvents e=new DecisionConditionEvents(-1);int want=truth?word*7+(word!=0?1:0):0;
   int actual=DecisionConditionAlgorithm.word(e,truth,word);if(actual!=want||!e.trace.equals("0"))throw new AssertionError("nonzero vs low bit");
   System.out.println(actual);
  }
  for(int mask=0;mask<8;mask++)for(int fail=-1;fail<3;fail++){
   boolean a=(mask&1)!=0,b=(mask&2)!=0,c=(mask&4)!=0;boolean[] gates={a,b,c};
   String trace="";boolean truth=false,throwsFailure=false;
   for(int i=0;i<3;i++){trace+=i;if(i==fail){throwsFailure=true;break;}if(gates[i]){truth=true;break;}}
   DecisionConditionEvents e=new DecisionConditionEvents(fail);
   try{int actual=DecisionConditionAlgorithm.select(e,a,b,c);if(throwsFailure||actual!=(truth?11:-2))throw new AssertionError("word/branch consumers");}
   catch(RuntimeException ex){if(!throwsFailure||ex!=e.failure)throw new AssertionError("failure identity");}
   if(!trace.equals(e.trace))throw new AssertionError("short-circuit order/once");
   System.out.println(mask+":"+fail+":"+trace);
  }
  long[] words={-2,0,2};for(long a:words)for(long b:words)for(long c:words)for(long d:words){
   int want=0;if(c>0){boolean truth=a>0||b>0;if(truth)want+=d==0?4:8;}if(d>0&& (a>0||b>0||c>0))want+=13;
   int actual=DecisionConditionAlgorithm.ordered(a,b,c,d);if(actual!=want)throw new AssertionError("numeric reuse "+actual+" != "+want);
   System.out.println(actual);
  }
 }
}`, nil, nil, Precision, Compatibility, "legacy")
}
