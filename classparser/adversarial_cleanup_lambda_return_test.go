package javaclassparser

import (
	"strings"
	"testing"
)

// A shared return beyond a conditional finally must not be consumed by its
// still-raw try body. Check identity, exception priority, factory cardinality,
// captures and effect counts independently of the reconstructed output.
func TestAdversarialSharedReturnPreservesConditionalFinallyFactory(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
import java.util.function.*;
public class CleanupDecision {
 public static int works,cleanups,events;
 public static final RuntimeException FAIL=new RuntimeException("work"),CLOSE=new RuntimeException("close");
 public static final Supplier<Runnable> seed=()->()->{events++;};
 public static void work(boolean fail){works++;if(fail)throw FAIL;}
 public static void use(IntConsumer sink){cleanups++;sink.accept(7);}
 public static Object select(Object result,int count,int failAt,int captured,boolean closeFail){
  if(result!=null){boolean abandoned=true;
   try{for(int step=0;step<count;step++)work(step==failAt);abandoned=false;}
   finally{if(abandoned)use(n->{events+=n+captured;if(closeFail)throw CLOSE;});}
  }
  return result;
 }
}
class CleanupDecisionDriver{public static void main(String[]args){int rows=0;
 for(boolean present:new boolean[]{false,true})for(int count=-1;count<=3;count++)for(int failAt=-1;failAt<=3;failAt++)for(int captured:new int[]{-7,0,13})for(boolean closeFail:new boolean[]{false,true}){
 CleanupDecision.works=CleanupDecision.cleanups=CleanupDecision.events=0;Object token=present?new Object():null,got=null;RuntimeException caught=null;
 boolean fails=present&&failAt>=0&&failAt<count;int calls=present?(fails?failAt+1:Math.max(0,count)):0;
 try{got=CleanupDecision.select(token,count,failAt,captured,closeFail);}catch(RuntimeException ex){caught=ex;}
 RuntimeException want=fails?(closeFail?CleanupDecision.CLOSE:CleanupDecision.FAIL):null;
 if(caught!=want||want==null&&got!=token)throw new AssertionError("result or exception identity");
 if(CleanupDecision.works!=calls||CleanupDecision.cleanups!=(fails?1:0)||CleanupDecision.events!=(fails?7+captured:0))throw new AssertionError("cleanup effects");
 CleanupDecision.seed.get().run();if(CleanupDecision.events!=(fails?8+captured:1))throw new AssertionError("nested factory");rows++;
 }System.out.println(rows+":cleanup:lambda");}}
`
	for _, name := range []string{"CleanupDecision", "ReleaseResult"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "CleanupDecision", name), name, name+"Driver", "300:cleanup:lambda\n")
		})
	}
}
