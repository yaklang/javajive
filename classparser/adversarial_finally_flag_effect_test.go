package javaclassparser

import "testing"

func TestAdversarialFinallyFlagPreservesExceptionAndReleaseEffects(t *testing.T) {
	roundTripGenericFlow(t, "FinallyFlagDriver", `
class FlagEffects {
 int acquireCalls,releaseCalls,actionCalls,mode;final RuntimeException failure=new RuntimeException("shared"),releaseFailure=new RuntimeException("release");
 void acquire(int mode)throws InterruptedException{acquireCalls++;if(mode==0)throw new InterruptedException();}
 boolean action(int mode){actionCalls++;if(mode==1||mode==6)throw failure;return mode!=2;}
 void release(){releaseCalls++;if(mode==5||mode==6)throw releaseFailure;}
}
public class FinallyFlagDriver {
 static boolean run(FlagEffects effects,int mode){
  boolean release=false;
  try{effects.acquire(mode);release=true;}catch(InterruptedException failure){return false;}
  boolean result=true;
  try{
   if(mode!=3){result=effects.action(mode);if(result&&mode==4)release=false;return result;}else{return false;}
  }finally{if(release)effects.release();}
 }
 public static void main(String[]args){for(int mode=0;mode<8;mode++){
  FlagEffects effects=new FlagEffects();effects.mode=mode;
  try{boolean result=run(effects,mode);
   if(result!=(mode==4||mode==7)||effects.acquireCalls!=1||effects.actionCalls!=(mode==0||mode==3?0:1)||effects.releaseCalls!=(mode==0||mode==4?0:1))throw new AssertionError("normal effects");
   System.out.println(mode+":"+result+":"+effects.releaseCalls);
  }catch(RuntimeException failure){if((mode!=1&&mode!=5&&mode!=6)||failure!=(mode==1?effects.failure:effects.releaseFailure)||effects.acquireCalls!=1||effects.actionCalls!=1||effects.releaseCalls!=1)throw new AssertionError("throw identity/effects",failure);System.out.println(failure.getMessage()+":"+effects.releaseCalls);}
 }}
}`, Precision, Compatibility, "legacy")
}
