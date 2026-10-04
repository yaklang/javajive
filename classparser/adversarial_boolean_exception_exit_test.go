package javaclassparser

import "testing"

func TestAdversarialBooleanExitSnapshotAcrossCatchAndFinally(t *testing.T) {
	roundTripGenericFlow(t, "BooleanExitDriver", `
class BooleanExitEffects {
 String trace="";int mode;final RuntimeException failure=new RuntimeException("action"),releaseFailure=new RuntimeException("release");
 void acquire()throws InterruptedException{trace+="A";if(mode==0)throw new InterruptedException();}
 Object description(){trace+="D";return mode==3?null:this;}
 Object sync(){trace+="S";return mode==4?null:this;}
 boolean execute(){trace+="E";if(mode==1)throw failure;return mode!=2;}
 void caught(Exception ex){if(ex!=failure)throw new AssertionError("caught identity");trace+="C";}
 Object async(){trace+="Y";return mode==5?this:null;}
 void schedule(){trace+="Q";}
 void release(){trace+="R";if(mode==6)throw releaseFailure;}
}
public class BooleanExitDriver {
 static boolean run(BooleanExitEffects effects){boolean release=false;
  try{effects.acquire();release=true;}catch(InterruptedException failure){return false;}
  boolean result=true;
  try{Object description=effects.description();if(description!=null){
   if(effects.sync()!=null){try{result=effects.execute();}catch(Exception failure){result=false;effects.caught(failure);}}
   if(result&&effects.async()!=null){effects.schedule();release=false;}return result;
  }else{return false;}}finally{if(release)effects.release();}
 }
 public static void main(String[]args){String[] traces={"A","ADSECR","ADSER","ADR","ADSYR","ADSEYQ","ADSEYR","ADSEYR"};
  for(int mode=0;mode<8;mode++){BooleanExitEffects effects=new BooleanExitEffects();effects.mode=mode;
   boolean result=false;Throwable actual=null;try{result=run(effects);}catch(Throwable failure){actual=failure;}
   if(!traces[mode].equals(effects.trace)||actual!=(mode==6?effects.releaseFailure:null)||result!=(mode==4||mode==5||mode==7))throw new AssertionError("return/effects/exception mode="+mode+" trace="+effects.trace,actual);
   System.out.println(mode+":"+result+":"+effects.trace);
  }
 }
}`, Precision, Compatibility, "legacy")
}
