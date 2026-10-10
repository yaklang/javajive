package javaclassparser

import "testing"

func TestAdversarialExplicitPrimaryCleanupIdentityRoundTrip(t *testing.T) {
	t.Parallel()
	// This explicit source models the older primary-local/two-handler lowering,
	// independently of modern javac's optimized try-with-resources lowering.
	roundTripGenericFlow(t, "ExplicitPrimaryCleanupReview", `
class ExplicitReviewResource implements AutoCloseable {static int closes;final Exception failure;ExplicitReviewResource(Exception e){failure=e;}public void close()throws Exception{closes++;if(failure!=null)throw failure;}}
public class ExplicitPrimaryCleanupReview {
 static String work(ExplicitReviewResource input,int mode,RuntimeException bodyRuntime,AssertionError bodyError)throws Exception{
  ExplicitReviewResource local=input;Throwable primary=null;
  try{try{if(mode==1)throw bodyRuntime;if(mode==2)throw bodyError;return "ok";}catch(Throwable caught){primary=caught;throw caught;}}
  finally{if(local!=null){if(primary!=null){try{local.close();}catch(Throwable closing){primary.addSuppressed(closing);}}else{local.close();}}}
 }
 static String run(int mode,boolean fail,boolean missing){ExplicitReviewResource.closes=0;Exception closing=new Exception("close");RuntimeException runtime=new IllegalStateException("body");AssertionError error=new AssertionError("body-error");ExplicitReviewResource resource=missing?null:new ExplicitReviewResource(fail?closing:null);try{return work(resource,mode,runtime,error)+":"+ExplicitReviewResource.closes;}catch(Throwable caught){Throwable[] suppressed=caught.getSuppressed();return caught.getClass().getName()+":"+(caught==runtime)+":"+(caught==error)+":"+(caught==closing)+":"+suppressed.length+":"+(suppressed.length>0&&suppressed[0]==closing)+":"+ExplicitReviewResource.closes;}}
 public static void main(String[] args){for(int mode=0;mode<3;mode++)for(boolean fail:new boolean[]{false,true})for(boolean missing:new boolean[]{false,true})System.out.println(run(mode,fail,missing));}
}`, Precision, Compatibility, "legacy")
}
