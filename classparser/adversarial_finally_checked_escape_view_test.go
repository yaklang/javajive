package javaclassparser

import "testing"

// Only the consumer is rebuilt. The untouched oracle checks both checked and
// unchecked exception identity, terminal cleanup priority, and exact effects.
func TestAdversarialFinallyCheckedEscapeSourceView(t *testing.T) {
	roundTripGenericFlowUnits(t, "FinallyEscapeViewDriver", `
class FinallyEscapeOps {
 static final java.io.IOException checked=new java.io.IOException("body-checked"),cleanupChecked=new java.io.IOException("cleanup-checked");
 static final RuntimeException runtime=new RuntimeException("body-runtime"),cleanupRuntime=new RuntimeException("cleanup-runtime");
 static final Error fatal=new AssertionError("body-error");
 static int calls,cleanupCalls;
 static int body(int mode)throws java.io.IOException{calls++;if(mode==1)throw checked;if(mode==2)throw runtime;if(mode==3)throw fatal;return 17;}
 static void cleanup(int mode)throws java.io.IOException{cleanupCalls++;if(mode==1)throw cleanupChecked;if(mode==2)throw cleanupRuntime;}
}
class FinallyEscapeConsumer {
 static int run(int bodyMode,int cleanupMode,boolean absorb)throws java.io.IOException {
  try{try{return FinallyEscapeOps.body(bodyMode);}catch(java.io.IOException failure){if(absorb)return 23;throw failure;}}
  finally{FinallyEscapeOps.cleanup(cleanupMode);}
 }
}
public class FinallyEscapeViewDriver {
 public static void main(String[]args){for(int body=0;body<4;body++)for(int cleanup=0;cleanup<3;cleanup++)for(boolean absorb:new boolean[]{false,true}){
  FinallyEscapeOps.calls=FinallyEscapeOps.cleanupCalls=0;Object result;
  try{result=Integer.valueOf(FinallyEscapeConsumer.run(body,cleanup,absorb));}catch(Throwable failure){result=failure;}
  Object expected=cleanup==1?FinallyEscapeOps.cleanupChecked:cleanup==2?FinallyEscapeOps.cleanupRuntime:body==1&&!absorb?FinallyEscapeOps.checked:body==2?FinallyEscapeOps.runtime:body==3?FinallyEscapeOps.fatal:Integer.valueOf(body==1?23:17);
  if((expected instanceof Throwable?result!=expected:!expected.equals(result))||FinallyEscapeOps.calls!=1||FinallyEscapeOps.cleanupCalls!=1)throw new AssertionError("finally identity/priority/effects "+body+":"+cleanup+":"+absorb);
  System.out.println(body+":"+cleanup+":"+absorb+":"+(result instanceof Throwable?((Throwable)result).getMessage():result));
 }}
}`, nil, []string{"FinallyEscapeConsumer"}, Precision, Compatibility, "legacy")
}
