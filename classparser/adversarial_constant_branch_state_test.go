package javaclassparser

import "testing"

func TestAdversarialConstantBranchPreservesJoinsLoopsAndExceptionalState(t *testing.T) {
	roundTripGenericFlow(t, "BranchStateReview", `
class BranchStateEvents {String trace="";final RuntimeException failure=new RuntimeException("shared");void effect(boolean fail){trace+="E";if(fail)throw failure;}void record(boolean value){trace+=value?"T":"F";}}
public class BranchStateReview {
 static void joined(BranchStateEvents e,boolean choose){boolean flag;if(choose){e.trace+="A";flag=true;}else{e.trace+="B";flag=true;}if(flag)e.record(flag);else throw new AssertionError();}
 static void mixed(BranchStateEvents e,boolean choose){boolean flag;if(choose){e.trace+="A";flag=true;}else{e.trace+="B";flag=false;}if(flag)e.record(true);else e.record(false);}
 static void exceptional(BranchStateEvents e,boolean fail){boolean flag=false;try{e.effect(fail);flag=true;}catch(RuntimeException x){if(x!=e.failure)throw new AssertionError(x);e.record(flag);}e.record(flag);}
 static void loop(BranchStateEvents e,int count){int word=1;for(int i=0;i<count;i++)word++;if(word==1)e.record(true);else e.record(false);}
 public static void main(String[]args){for(boolean choose:new boolean[]{false,true})for(boolean fail:new boolean[]{false,true})for(int count=0;count<12;count++){
  BranchStateEvents e=new BranchStateEvents();joined(e,choose);mixed(e,choose);exceptional(e,fail);loop(e,count);
  String expected=(choose?"ATAT":"BTBF")+(fail?"EFF":"ET")+(count==0?"T":"F");
  if(!expected.equals(e.trace))throw new AssertionError("state "+e.trace+" expected "+expected);
  System.out.println(choose+":"+fail+":"+count+":"+e.trace);
 }}
}`, Precision, Compatibility, "legacy")
}
