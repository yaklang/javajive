package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// A typed catch handles the read failure; its surrounding finally runs once.
// Cleanup failures supersede a pending Error without entering that same catch
// again. Throwable handlers emitted by javac for nested finally regions must
// not become parallel duplicate catch clauses or broaden the read region.
func TestAdversarialCaughtLoopNestedFinallyKeepsOriginalExceptionRegionsAndEffects(t *testing.T) {
	assertDefaultCompilerRegistrationOracle(t, caughtLoopNestedFinallyFixture, "360:caught-loop:nested-finally:identity:effects\n")
}

const caughtLoopNestedFinallyFixture = `import java.io.*;import java.math.BigInteger;
class RegionEffects {
 final int[] input;final Throwable failure;final int failAt,cleanup;
 final RuntimeException lockFailure=new IllegalStateException("lock");
 final RuntimeException recordFailure=new IllegalStateException("record");
 final RuntimeException publishFailure=new IllegalStateException("publish");
 final Error unlockFailure=new AssertionError("unlock");
 final RuntimeException afterFailure=new IllegalArgumentException("after");
 int reads,published;Throwable recorded;String trace="";
 RegionEffects(int[] input,Throwable failure,int failAt,int cleanup){this.input=input;this.failure=failure;this.failAt=failAt;this.cleanup=cleanup;}
 int read()throws IOException{trace+="R";int at=reads++;if(at==failAt&&failure!=null){if(failure instanceof IOException)throw(IOException)failure;if(failure instanceof Error)throw(Error)failure;throw(RuntimeException)failure;}return at<input.length?input[at]:-1;}
 void record(Throwable value){trace+="C";recorded=value;if(cleanup==5)throw recordFailure;}
 void lock(){trace+="L";if(cleanup==4)throw lockFailure;}
 void publish(int value){trace+="P";published=value;if(cleanup==1)throw publishFailure;}
 void unlock(){trace+="U";if(cleanup==2)throw unlockFailure;}
 void after(){trace+="A";if(cleanup==3)throw afterFailure;}
}
class DefaultCompilerCases {
 static Runnable task(RegionEffects effects){return ()->{
  int sum=0;
  try{do{int value=effects.read();if(value<=0)break;sum+=value;}while(true);}
  catch(Throwable failure){effects.record(failure);if(failure instanceof Error)throw(Error)failure;}
  finally{effects.lock();try{effects.publish(sum);}finally{effects.unlock();}effects.after();}
 };}
}
class DefaultCompilerDriver {public static void main(String[]args){
 int rows=0;
 int[][] inputs={{-1},{0},{2,3,-1},{1,1,1,0},{Integer.MAX_VALUE,1,-1}};
 for(int[] input:inputs)for(int kind=0;kind<4;kind++)for(int failAt:new int[]{0,1,3})for(int cleanup=0;cleanup<6;cleanup++){
  Throwable failure=kind==0?null:kind==1?new IOException("same checked"):kind==2?new AssertionError("same error"):new IllegalStateException("same runtime");
  RegionEffects effects=new RegionEffects(input,failure,failAt,cleanup);
  BigInteger sum=BigInteger.ZERO;String expectedTrace="";Throwable recorded=null;Throwable expected=null;int reads=0;
  for(int at=0;;at++){
   expectedTrace+="R";reads++;
   if(at==failAt&&failure!=null){expectedTrace+="C";recorded=failure;if(cleanup==5)expected=effects.recordFailure;else if(kind==2)expected=failure;break;}
   int value=at<input.length?input[at]:-1;if(value<=0)break;sum=sum.add(BigInteger.valueOf(value));
  }
  expectedTrace+="L";int expectedPublished=0;
  if(cleanup==4)expected=effects.lockFailure;else{
   expectedTrace+="PU";expectedPublished=sum.intValue();
   if(cleanup==1)expected=effects.publishFailure;else if(cleanup==2)expected=effects.unlockFailure;else{expectedTrace+="A";if(cleanup==3)expected=effects.afterFailure;}
  }
  Throwable got=null;try{DefaultCompilerCases.task(effects).run();}catch(Throwable observed){got=observed;}
  if(got!=expected||effects.recorded!=recorded||effects.reads!=reads||effects.published!=expectedPublished||!effects.trace.equals(expectedTrace))throw new AssertionError("original exception/effect model row="+rows+" expected="+expectedTrace+" actual="+effects.trace,got);
  rows++;
 }
 System.out.println(rows+":caught-loop:nested-finally:identity:effects");
}}`

func TestAdversarialNestedCleanupCompilerFamiliesKeepCatchOwnership(t *testing.T) {
	for _, variant := range []string{"lambda", "direct method", "renamed lambda", "typed short circuit cleanup"} {
		t.Run(variant, func(t *testing.T) {
			source := caughtLoopNestedFinallyFixture
			root, driver := "DefaultCompilerCases", "DefaultCompilerDriver"
			if variant == "direct method" {
				source = strings.Replace(source, "static Runnable task(RegionEffects effects){return ()->{", "static void execute(RegionEffects effects){", 1)
				source = strings.Replace(source, " };}", " }", 1)
				source = strings.ReplaceAll(source, "DefaultCompilerCases.task(effects).run()", "DefaultCompilerCases.execute(effects)")
			}
			if variant == "renamed lambda" {
				root, driver = "NestedRegionMethods", "NestedRegionDriver"
				source = strings.NewReplacer("DefaultCompilerCases", root, "DefaultCompilerDriver", driver, "RegionEffects", "CleanupEvents", "task(", "prepare(", "publish(", "finish(", "unlock(", "release(").Replace(source)
			}
			if variant == "typed short circuit cleanup" {
				source = strings.Replace(source, " void lock()", " Object pending(){trace+=\"T\";return recorded;}\n void lock()", 1)
				source = strings.Replace(source, "try{effects.publish(sum);}", "try{if(sum>=0 && !(effects.pending() instanceof IOException)){effects.publish(sum);}else{effects.publish(~sum);}}", 1)
				// The independent model uses the complementary predicate and
				// records whether the second, effectful operand was evaluated.
				source = strings.Replace(source, "expectedTrace+=\"PU\";expectedPublished=sum.intValue();", "if(sum.intValue()>=0)expectedTrace+=\"T\";expectedTrace+=\"PU\";expectedPublished=sum.intValue();if(expectedPublished<0 || recorded instanceof IOException)expectedPublished=~expectedPublished;", 1)
			}
			const want = "360:caught-loop:nested-finally:identity:effects\n"
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, source, []string{root}, driver, want, nativeLexicalExactSignatures)
			})
			t.Run("native8", func(t *testing.T) {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{root + ".java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{root}, driver, want, nil, nativeLexicalExactSignatures)
			})
		})
	}
}
