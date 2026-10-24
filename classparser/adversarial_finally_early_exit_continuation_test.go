package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialFinallyEarlyReturnKeepsNormalContinuationOutsideHandlers(t *testing.T) {
	assertDefaultCompilerRegistrationOracle(t, finallyEarlyExitContinuationFixture, "64:early-return:continuation:cleanup:identity\n")
}

func TestAdversarialFinallyEarlyReturnContinuationCompilerFamilies(t *testing.T) {
	for _, variant := range []string{"original", "enclosing handler"} {
		t.Run(variant, func(t *testing.T) {
			source := finallyEarlyExitContinuationFixture
			if variant == "enclosing handler" {
				source = strings.Replace(source, "String trace=\"\";int unlocks,closes;", "String trace=\"\";int unlocks,closes;IOException recorded;", 1)
				source = strings.Replace(source, " void unlock()", " void record(IOException failure){trace+=\"X\";recorded=failure;}void last(){trace+=\"Z\";}\n void unlock()", 1)
				source = strings.Replace(source, "boolean close=false;effects.lock.lock();", "try{boolean close=false;effects.lock.lock();", 1)
				source = strings.Replace(source, " }\n}\nclass DefaultCompilerDriver", " }catch(IOException caught){effects.record(caught);throw caught;}finally{effects.last();}\n }\n}\nclass DefaultCompilerDriver", 1)
				source = strings.Replace(source, "+(closing?\"C\":\"\");", "+(closing?\"C\":\"\");boolean expectedIO=closing&&close!=0||later&&wait!=0;trace+=(expectedIO?\"X\":\"\")+\"Z\";if(effects.recorded!=(expectedIO?got:null))throw new AssertionError(\"enclosing handler identity\",got);", 1)
			}
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, source, []string{"DefaultCompilerCases"}, "DefaultCompilerDriver", "64:early-return:continuation:cleanup:identity\n", nativeLexicalExactSignatures)
			})
			t.Run("native8", func(t *testing.T) {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{"DefaultCompilerCases.java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"DefaultCompilerCases"}, "DefaultCompilerDriver", "64:early-return:continuation:cleanup:identity\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}

const finallyEarlyExitContinuationFixture = `import java.io.*;import java.util.concurrent.locks.ReentrantLock;
class CloseEffects {
 final ReentrantLock lock=new ReentrantLock();boolean closed,started;final boolean busy,shutdown;final InterruptedException waitFailure;final IOException closeFailure;
 String trace="";int unlocks,closes;
 CloseEffects(int closed,int busy,int shutdown,int wait,int close){this.closed=closed!=0;this.busy=busy!=0;this.shutdown=shutdown!=0;waitFailure=wait==0?null:new InterruptedException("same wait");closeFailure=close==0?null:new IOException("same close");}
 void unlock(){trace+="U";unlocks++;lock.unlock();}
 void shutdownNow(){trace+="S";}
 void await()throws InterruptedException{trace+="W";if(waitFailure!=null)throw waitFailure;}
 void closeInput()throws IOException{trace+="C";closes++;if(closeFailure!=null)throw closeFailure;}
}
class DefaultCompilerCases {
 static void execute(CloseEffects effects)throws IOException{
  boolean close=false;effects.lock.lock();
  try{if(effects.closed)return;effects.closed=true;if(!effects.busy){close=true;effects.started=true;}}
  finally{effects.unlock();}
  if(effects.shutdown){
   try{effects.shutdownNow();effects.await();}
   catch(InterruptedException interrupted){InterruptedIOException converted=new InterruptedIOException(interrupted.getMessage());converted.initCause(interrupted);throw converted;}
   finally{if(close)effects.closeInput();}
  }
 }
}
class DefaultCompilerDriver {public static void main(String[]args){
 int rows=0;
 for(int closed=0;closed<2;closed++)for(int busy=0;busy<2;busy++)for(int shutdown=0;shutdown<2;shutdown++)for(int wait=0;wait<2;wait++)for(int close=0;close<2;close++)for(int held=0;held<2;held++){
  CloseEffects effects=new CloseEffects(closed,busy,shutdown,wait,close);if(held!=0)effects.lock.lock();
  Throwable got=null;try{DefaultCompilerCases.execute(effects);}catch(Throwable observed){got=observed;}
  boolean later=closed==0&&shutdown!=0,closing=later&&busy==0;String trace="U"+(later?"SW":"")+(closing?"C":"");
  if(!effects.closed||effects.started!=(closed==0&&busy==0)||effects.unlocks!=1||effects.lock.getHoldCount()!=held||effects.closes!=(closing?1:0)||!effects.trace.equals(trace))throw new AssertionError("original branch/effect model row="+rows+" trace="+effects.trace+" holds="+effects.lock.getHoldCount(),got);
  if(closing&&close!=0){if(got!=effects.closeFailure)throw new AssertionError("close failure identity",got);}
  else if(later&&wait!=0){if(got==null||got.getClass()!=InterruptedIOException.class||got.getCause()!=effects.waitFailure)throw new AssertionError("wait wrapper identity",got);}
  else if(got!=null)throw new AssertionError("unexpected continuation failure",got);
  if(held!=0)effects.lock.unlock();rows++;
 }
 System.out.println(rows+":early-return:continuation:cleanup:identity");
}}
`
