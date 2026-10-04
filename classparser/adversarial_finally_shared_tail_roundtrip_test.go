package javaclassparser

import "testing"

func TestAdversarialRecoveredFinallySharedVoidTailRoundTrip(t *testing.T) {
	roundTripGenericFlowUnits(t, "FinallySharedVoidTailReview", `
class FinallyTailEffects {static String trace="";static final RuntimeException actionFailure=new IllegalArgumentException("action");static final RuntimeException failureFailure=new IllegalStateException("failure");static final RuntimeException cleanupFailure=new UnsupportedOperationException("cleanup");static void action(int mode){trace+="action;";if(mode==1||mode==2)throw actionFailure;}static boolean finish(int mode){trace+="finish;";return mode!=0;}static void failure(Throwable caught,int mode){trace+="failure:"+(caught==actionFailure)+";";if(mode==2)throw failureFailure;}static void cleanup(boolean fail){trace+="cleanup;";if(fail)throw cleanupFailure;}static void report(Throwable caught){System.out.println((caught==actionFailure)+":"+(caught==failureFailure)+":"+(caught==cleanupFailure)+":"+trace);}}
public class FinallySharedVoidTailReview {
 static void run(boolean early,boolean done,boolean flush,int mode,boolean failCleanup){if(early)return;if(done)return;try{FinallyTailEffects.action(mode);FinallyTailEffects.finish(mode);}catch(Throwable caught){FinallyTailEffects.failure(caught,mode);}finally{if(flush)FinallyTailEffects.cleanup(failCleanup);}}
 public static void main(String[]args){for(int mode=0;mode<3;mode++){FinallyTailEffects.trace="";try{run(false,false,false,mode,false);}catch(Throwable caught){FinallyTailEffects.report(caught);}System.out.println(FinallyTailEffects.trace);FinallyTailEffects.trace="";try{run(false,false,true,mode,true);}catch(Throwable caught){FinallyTailEffects.report(caught);}System.out.println(FinallyTailEffects.trace);}FinallyTailEffects.trace="";run(true,false,true,0,true);run(false,true,true,0,true);System.out.println(FinallyTailEffects.trace);}
}`, nil, nil, Precision, Compatibility, "legacy")
}
