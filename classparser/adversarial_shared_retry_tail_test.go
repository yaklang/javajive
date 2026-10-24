package javaclassparser

import "testing"

// Shared slow-path entries must preserve their exact predicate and execute once.
func TestAdversarialSharedRetryTailKeepsShortCircuitAndLocalBoolean(t *testing.T) {
	roundTripGenericFlow(t, "SharedRetryTail", `public class SharedRetryTail {
 RetryNode[] table;int probe;String trace="";int failure;
 int offer(String value){
  int mask;RetryNode node;int result=0;boolean uncontended=true;RetryNode[] entries=table;
  if(entries==null || (mask=entries.length-1)<0 || (node=entries[probe&mask])==null || !(uncontended=((result=node.offer(value,this))!=-1)))retry(value,uncontended);
  return result;
 }
 void retry(String value,boolean uncontended){trace+="R"+(uncontended?"T":"F")+value;if(failure==2)throw RetryDriver.RETRY;}
 public static void main(String[]args){RetryDriver.main(args);}
}
class RetryNode {
 int result;RetryNode(int result){this.result=result;}
 int offer(String value,SharedRetryTail owner){owner.trace+="O"+value;if(owner.failure==1)throw RetryDriver.OFFER;return result;}
}
class RetryDriver {
 static final RuntimeException OFFER=new RuntimeException("original offer payload"),RETRY=new RuntimeException("original retry payload");
 public static void main(String[]args){int rows=0;for(int topology=0;topology<4;topology++)for(int code:new int[]{-1,0,1,7})for(int failure=0;failure<3;failure++)for(int probe=0;probe<4;probe++){
 SharedRetryTail state=new SharedRetryTail();state.failure=failure;state.probe=probe;RetryNode node=new RetryNode(code);state.table=topology==0?null:topology==1?new RetryNode[0]:topology==2?new RetryNode[4]:new RetryNode[]{node,node,node,node};
 boolean offered=topology==3;boolean retried=!offered||code==-1;Throwable expected=offered&&failure==1?OFFER:retried&&failure==2?RETRY:null;
 String trace=offered?"Ox":"";if(!(offered&&failure==1)&&retried)trace+="R"+(offered?"F":"T")+"x";
 Throwable got=null;int result=99;try{result=state.offer("x");}catch(Throwable error){got=error;}
 if(got!=expected||!state.trace.equals(trace)||got==null&&result!=(offered?code:0))throw new AssertionError("retry "+topology+":"+code+":"+failure+":"+probe+":"+state.trace+" expected "+trace,got);rows++;
 }if(rows!=192)throw new AssertionError("coverage:"+rows);System.out.println(rows+":shared-retry-tail:effects:exception-identity:result");}
}`, Precision, Compatibility, "legacy")
}
