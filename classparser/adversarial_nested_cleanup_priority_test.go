package javaclassparser

import "testing"

func TestAdversarialNestedCleanupKeepsFailurePriorityAndEffects(t *testing.T) {
	roundTripGenericFlow(t, "CleanupPriorityDriver", `
class CleanupEvents {
 String trace="";int mode;
 final RuntimeException bodyFailure=new RuntimeException("body"),footerFailure=new RuntimeException("footer"),closeFailure=new RuntimeException("close"),cleanupFailure=new RuntimeException("cleanup");
 Object open(){trace+="O";return (mode&2)==0?this:null;}
 void body(){trace+="B";if((mode&1)!=0)throw bodyFailure;}
 void footer(Object resource,Throwable failure){if(resource!=this||failure!=null&&failure!=bodyFailure)throw new AssertionError("footer operands");trace+=failure==null?"N":"F";if((mode&4)!=0)throw footerFailure;}
 void close(Object resource){if(resource!=this)throw new AssertionError("close operand");trace+="C";if((mode&8)!=0)throw closeFailure;}
 void cleanup(Object resource){if(resource!=null&&resource!=this)throw new AssertionError("cleanup operand");trace+="X";if((mode&16)!=0)throw cleanupFailure;}
}
public class CleanupPriorityDriver {
 static void run(CleanupEvents events){boolean success=false;Object resource=null;
  try{resource=events.open();
   try{events.body();}catch(Throwable failure){if(resource!=null){events.footer(resource,failure);throw new AssertionError("unreachable");}else{throw failure;}}
   if(resource!=null){events.footer(resource,null);events.close(resource);}success=true;
  }finally{if(!success)events.cleanup(resource);}
 }
 public static void main(String[]args){for(int mode=0;mode<32;mode++){
  CleanupEvents events=new CleanupEvents();events.mode=mode;String trace="OB";Throwable expected=null;boolean assertion=false;
  if((mode&1)!=0){expected=events.bodyFailure;if((mode&2)==0){trace+="F";if((mode&4)!=0)expected=events.footerFailure;else{expected=null;assertion=true;}}}
  else if((mode&2)==0){trace+="N";if((mode&4)!=0)expected=events.footerFailure;else{trace+="C";if((mode&8)!=0)expected=events.closeFailure;}}
  if(expected!=null||assertion){trace+="X";if((mode&16)!=0){expected=events.cleanupFailure;assertion=false;}}
  Throwable actual=null;try{run(events);}catch(Throwable failure){actual=failure;}
  if(!trace.equals(events.trace)||(assertion?!(actual instanceof AssertionError):actual!=expected))throw new AssertionError("failure priority/effects mode="+mode+" trace="+events.trace+" expected="+trace,actual);
  System.out.println(mode+":"+trace+":"+(assertion?"assertion":expected==null?"normal":expected.getMessage()));
 }}
}`, Precision, Compatibility, "legacy")
}
