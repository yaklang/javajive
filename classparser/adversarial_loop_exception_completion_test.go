package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialLoopCatchPreservesExceptionalCompletion(t *testing.T) {
	const fixture = `
class LoopFailureEffects {
 static int visits,failAt;static String trace="";
 static final IllegalStateException TOKEN=new IllegalStateException("identity");
 static void step(){trace+="S";visits++;if(visits==failAt)throw TOKEN;}
}
public class LoopFailure {
 static int run(int remaining){int result=0;
  while(remaining>0){
   try{remaining--;LoopFailureEffects.step();result+=3;}
   catch(IllegalStateException caught){throw new RuntimeException(caught);}
  }
  LoopFailureEffects.trace+="C";return result;
 }
}
class LoopFailureDriver {public static void main(String[]args){int rows=0;
 for(int count:new int[]{-1,0,1,2,5})for(int failAt:new int[]{0,1,2,6}){
  LoopFailureEffects.visits=0;LoopFailureEffects.failAt=failAt;LoopFailureEffects.trace="";
  int actual=0;Throwable failure=null;try{actual=LoopFailure.run(count);}catch(Throwable e){failure=e;}
  int iterations=Math.max(count,0);boolean fails=failAt>0&&failAt<=iterations;int calls=fails?failAt:iterations;
  StringBuilder expected=new StringBuilder();for(int i=0;i<calls;i++)expected.append('S');if(!fails)expected.append('C');
  if(LoopFailureEffects.visits!=calls||!expected.toString().equals(LoopFailureEffects.trace)||(fails?failure==null||failure.getClass()!=RuntimeException.class||failure.getCause()!=LoopFailureEffects.TOKEN:failure!=null||actual!=iterations*3))throw new AssertionError("exceptional completion count="+count+" failAt="+failAt+" trace="+LoopFailureEffects.trace,failure);
  rows++;
 }System.out.println(rows+":loop:throw:identity:partial-effects");}}
`
	for _, rename := range []bool{false, true} {
		owner, source := "LoopFailure", fixture
		if rename {
			owner = "RepeatedOperation"
			source = strings.ReplaceAll(source, "LoopFailure", owner)
		}
		driver := owner + "Driver"
		t.Run(owner, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, source, owner, driver, "20:loop:throw:identity:partial-effects\n")
		})
	}
}
