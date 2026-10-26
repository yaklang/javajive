package javaclassparser

import (
	"strings"
	"testing"
)

// Several early exits share a return value, but the successful nested arm
// still reaches the common completion. A source arm boundary is not an exit.
func TestAdversarialNestedGateRetainsSharedCompletion(t *testing.T) {
	const fixture = `
public class SharedCompletion {
 long count;int lower,upper,edges;Object empty,result=new Object();
 int failurePhase;static final RuntimeException FAILURE=new IllegalArgumentException("identity");
 StringBuilder events=new StringBuilder();
 void fail(int phase){if(failurePhase==phase)throw FAILURE;}
 void freeze(){events.append("F;");fail(1);}
 void prepare(){events.append("P;");fail(2);}
 void complete(){events.append("C;");fail(3);}
 Object finish(){
  freeze();
  if(count<lower||count<upper||edges==0){
   if(empty==null)return null;
   else if(lower>0||upper>0)return null;
  }else if(upper!=0){prepare();}
  complete();return result;
 }
}
class CompletionDriver {public static void main(String[]args){int rows=0;
 for(long count:new long[]{-1,0,1,3})for(int lower:new int[]{-1,0,1,2})for(int upper:new int[]{-1,0,1,2})for(int edges:new int[]{0,1})for(boolean empty:new boolean[]{false,true})for(int phase=0;phase<4;phase++)for(boolean nullResult:new boolean[]{false,true}){
  SharedCompletion x=new SharedCompletion();x.count=count;x.lower=lower;x.upper=upper;x.edges=edges;x.empty=empty?new Object():null;x.failurePhase=phase;if(nullResult)x.result=null;
  boolean gated=count<lower||count<upper||edges==0;
  boolean accepted=!gated||empty&&lower<=0&&upper<=0;
  boolean prepared=!gated&&upper!=0;
  boolean throwing=phase==1||phase==2&&prepared||phase==3&&accepted;
  String trace="F;"+(phase!=1&&prepared?"P;":"")+(phase!=1&&!(phase==2&&prepared)&&accepted?"C;":"");
  Object actual=null;Throwable caught=null;try{actual=x.finish();}catch(Throwable error){caught=error;}
  if((throwing?caught!=SharedCompletion.FAILURE:caught!=null||actual!=(accepted?x.result:null))||!trace.equals(x.events.toString()))throw new AssertionError("count="+count+" lower="+lower+" upper="+upper+" edges="+edges+" empty="+empty+" events="+x.events+" expected="+trace);
  rows++;
 }System.out.println(rows+":gates:completion:identity:effects");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			source, owner := fixture, "SharedCompletion"
			if spelling == "renamed" {
				source = strings.NewReplacer("SharedCompletion", "AcceptanceState", "finish()", "compute()", "empty", "marker", "edges", "degree").Replace(source)
				owner = "AcceptanceState"
			}
			testAuthoredOwnerAcrossSourceModes(t, source, owner, "CompletionDriver", "2048:gates:completion:identity:effects\n")
		})
	}
}
