package javaclassparser

import (
	"os"
	"testing"
)

// A checked call after cleanup is outside the original try/catch. Keeping it
// inside that region can unlock twice, replace the IOException, or release an
// enclosing caller's reentrant hold. Compare both exception identity and state.
func TestAdversarialNestedFinallyKeepsPostCleanupCheckedThrowOutsideHandlers(t *testing.T) {
	assertDefaultCompilerRegistrationOracle(t, nestedFinallyPostCleanupThrowFixture, "16:post-cleanup:throw:hold-count:identity\n")
}

func TestAdversarialNestedFinallyPostCleanupThrowCompilerFamilies(t *testing.T) {
	t.Run("modern", func(t *testing.T) {
		testNativeIndependentFamilyFixture(t, nestedFinallyPostCleanupThrowFixture, []string{"DefaultCompilerCases"}, "DefaultCompilerDriver", "16:post-cleanup:throw:hold-count:identity\n", nativeLexicalExactSignatures)
	})
	t.Run("native8", func(t *testing.T) {
		testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
			return localGetterCompileJavac8(t, map[string]string{"DefaultCompilerCases.java": nestedFinallyPostCleanupThrowFixture}, debug)
		}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"DefaultCompilerCases"}, "DefaultCompilerDriver", "16:post-cleanup:throw:hold-count:identity\n", nil, nativeLexicalExactSignatures)
	})
}

const nestedFinallyPostCleanupThrowFixture = `import java.io.*;import java.util.concurrent.locks.ReentrantLock;
class WaitEffects {
 final ReentrantLock lock=new ReentrantLock();final InterruptedException waitFailure;final RuntimeException clearFailure;final IOException afterFailure;
 String trace="";int clears,unlocks;
 WaitEffects(int before,int clear,int after){waitFailure=before==0?null:new InterruptedException("same interrupt");clearFailure=clear==0?null:new IllegalStateException("same clear");afterFailure=after==0?null:new IOException("same after");}
 void awaitOnce()throws InterruptedException{trace+="W";if(waitFailure!=null)throw waitFailure;}
 void clear(){trace+="F";clears++;if(clearFailure!=null)throw clearFailure;}
 void unlock(){trace+="U";unlocks++;lock.unlock();}
 void after()throws IOException{trace+="A";if(afterFailure!=null)throw afterFailure;}
}
class DefaultCompilerCases {
 static void execute(WaitEffects effects)throws IOException{
  effects.lock.lock();
  try{effects.awaitOnce();}
  catch(InterruptedException interrupted){InterruptedIOException converted=new InterruptedIOException(interrupted.getMessage());converted.initCause(interrupted);throw converted;}
  finally{try{effects.clear();}finally{effects.unlock();}}
  effects.after();
 }
}
class DefaultCompilerDriver {public static void main(String[]args){
 int rows=0;
 for(int before=0;before<2;before++)for(int clear=0;clear<2;clear++)for(int after=0;after<2;after++)for(int held=0;held<2;held++){
  WaitEffects effects=new WaitEffects(before,clear,after);if(held!=0)effects.lock.lock();
  Throwable got=null;try{DefaultCompilerCases.execute(effects);}catch(Throwable observed){got=observed;}
  boolean normal=before==0&&clear==0;String trace=normal?"WFUA":"WFU";
  if(effects.clears!=1||effects.unlocks!=1||effects.lock.getHoldCount()!=held||!effects.trace.equals(trace))throw new AssertionError("original cleanup state row="+rows+" trace="+effects.trace+" holds="+effects.lock.getHoldCount(),got);
  if(clear!=0){if(got!=effects.clearFailure)throw new AssertionError("cleanup failure identity",got);}
  else if(before!=0){if(got==null||got.getClass()!=InterruptedIOException.class||got.getCause()!=effects.waitFailure)throw new AssertionError("original checked wrapper cause",got);}
  else if(got!=effects.afterFailure)throw new AssertionError("post cleanup checked identity",got);
  if(held!=0)effects.lock.unlock();rows++;
 }
 System.out.println(rows+":post-cleanup:throw:hold-count:identity");
}}
`
