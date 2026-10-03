package javaclassparser

import "testing"

// Missing inherited declarations prohibit creating a synthetic helper. Java's
// unchanged catch-parameter rethrow needs no helper: its checked domain comes
// from the original protected operations, not the catch's broad static type.
func TestAdversarialPreciseCatchRethrowWithoutInheritedHelperNamespace(t *testing.T) {
	roundTripGenericFlowUnits(t, "PreciseRethrowDriver", `
class PreciseRethrowParent {}
class PreciseRethrowOps {
 static final java.io.IOException checked=new java.io.IOException("checked");
 static final RuntimeException runtime=new RuntimeException("runtime");
 static final Error fatal=new AssertionError("error");
 static int calls,records;static Throwable recorded;
 static int effect(int mode)throws java.io.IOException{calls++;if(mode==1)throw checked;if(mode==2)throw runtime;if(mode==3)throw fatal;return 31;}
 static void record(Throwable failure){records++;recorded=failure;}
}
class PreciseRethrowConsumer extends PreciseRethrowParent {
 static int run(int mode)throws java.io.IOException{
  try{return PreciseRethrowOps.effect(mode);}catch(Throwable caught){PreciseRethrowOps.record(caught);throw caught;}
 }
 static int nested(int mode)throws java.io.IOException{
  try{try{return PreciseRethrowOps.effect(mode);}catch(java.io.IOException caught){PreciseRethrowOps.record(caught);throw caught;}}
  catch(Throwable outer){PreciseRethrowOps.record(outer);throw outer;}
 }
}
public class PreciseRethrowDriver {public static void main(String[]args){for(int mode=0;mode<4;mode++)for(boolean nested:new boolean[]{false,true}){
 PreciseRethrowOps.calls=PreciseRethrowOps.records=0;PreciseRethrowOps.recorded=null;Object result;
 try{result=Integer.valueOf(nested?PreciseRethrowConsumer.nested(mode):PreciseRethrowConsumer.run(mode));}catch(Throwable failure){result=failure;}
 Object expected=mode==1?PreciseRethrowOps.checked:mode==2?PreciseRethrowOps.runtime:mode==3?PreciseRethrowOps.fatal:Integer.valueOf(31);
 int records=mode==0?0:mode==1&&nested?2:1;
 if((expected instanceof Throwable?result!=expected:!expected.equals(result))||PreciseRethrowOps.calls!=1||PreciseRethrowOps.records!=records||(records>0&&PreciseRethrowOps.recorded!=expected))throw new AssertionError("precise rethrow identity/effects");
 System.out.println(mode+":"+nested+":"+records+":"+(result instanceof Throwable?((Throwable)result).getMessage():result));
 }}
}`, func(name string) bool { return name != "PreciseRethrowParent" }, []string{"PreciseRethrowConsumer"}, Precision, Compatibility, "legacy")
}
