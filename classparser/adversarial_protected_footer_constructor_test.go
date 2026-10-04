package javaclassparser

import "testing"

func TestAdversarialConstructorProtectedFooterAndFinallyPriority(t *testing.T) {
	roundTripGenericFlowUnits(t, "ProtectedFooterDriver", `
class ProtectedFooterEvents {
 String trace="";int mode;
 final RuntimeException bodyFailure=new RuntimeException("body"),footerFailure=new RuntimeException("footer"),closeFailure=new RuntimeException("close"),cleanupFailure=new RuntimeException("cleanup");
 Object open(){trace+="O";return (mode&2)==0?this:null;}
 void body(){trace+="B";if((mode&1)!=0)throw bodyFailure;}
 void footer(Object resource,Throwable failure){if(resource!=this||failure!=null&&failure!=bodyFailure&&failure!=footerFailure&&failure!=closeFailure)throw new AssertionError("footer operands");trace+=failure==null?"N":"F";if((mode&4)!=0)throw footerFailure;}
 void close(Object resource){if(resource!=this)throw new AssertionError("close operand");trace+="C";if((mode&8)!=0)throw closeFailure;}
 void cleanup(Object[] resources){Object resource=resources[1];if(resources.length!=2||!(resources[0] instanceof ProtectedFooterConsumer)||resource!=null&&resource!=this)throw new AssertionError("cleanup operand");trace+="X";if((mode&16)!=0)throw cleanupFailure;}
}
class ProtectedFooterConsumer {
 final int number;final long chunks,dirty,docs;
 ProtectedFooterConsumer(ProtectedFooterEvents events,int n,long c,long d,long r){boolean success=false;Object resource=null;
  try{
   try{resource=events.open();events.body();if(n<0){number=-1;chunks=-1;dirty=-1;docs=-1;}else{number=n;chunks=c;dirty=d;docs=r;}if(chunks<dirty){throw events.bodyFailure;}else{if((dirty==0)!=(docs==0)){throw events.bodyFailure;}else{if(docs<dirty){throw events.bodyFailure;}else{if(resource!=null){events.footer(resource,null);events.close(resource);}success=true;}}}}
   catch(Throwable failure){if(resource!=null){events.footer(resource,failure);throw new AssertionError("unreachable");}else{throw failure;}}
  }finally{if(!success)events.cleanup(new Object[]{this,resource});}
 }
}
public class ProtectedFooterDriver {
 public static void main(String[]args){long[][] tuples={{-1,-1,-1},{1,2,3},{2,1,0},{7,2,1},{7,2,3},{Long.MAX_VALUE,Long.MAX_VALUE,Long.MAX_VALUE}};boolean[] invalid={false,true,true,true,false,false};for(int mode=0;mode<32;mode++)for(int n:new int[]{-1,0,7})for(int index=0;index<tuples.length;index++){
  ProtectedFooterEvents events=new ProtectedFooterEvents();events.mode=mode;String trace="OB";Throwable expected=null;boolean assertion=false;
  if((mode&1)!=0||n>=0&&invalid[index]){expected=events.bodyFailure;}
  else if((mode&2)==0){trace+="N";if((mode&4)!=0)expected=events.footerFailure;else{trace+="C";if((mode&8)!=0)expected=events.closeFailure;}}
  if(expected!=null&&(mode&2)==0){trace+="F";if((mode&4)!=0)expected=events.footerFailure;else{expected=null;assertion=true;}}
  if(expected!=null||assertion){trace+="X";if((mode&16)!=0){expected=events.cleanupFailure;assertion=false;}}
  Throwable actual=null;ProtectedFooterConsumer consumer=null;try{consumer=new ProtectedFooterConsumer(events,n,tuples[index][0],tuples[index][1],tuples[index][2]);}catch(Throwable failure){actual=failure;}
  if(!trace.equals(events.trace)||(assertion?!(actual instanceof AssertionError):actual!=expected)||actual==null&&(consumer==null||consumer.number!=(n<0?-1:n)))throw new AssertionError("priority mode="+mode+" trace="+events.trace+" expected="+trace,actual);
  System.out.println(mode+":"+n+":"+trace+":"+(assertion?"assertion":expected==null?"normal":expected.getMessage()));
 }}
}`, nil, []string{"ProtectedFooterConsumer"}, Precision, Compatibility, "legacy")
}
