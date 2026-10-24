package javaclassparser

import (
	"strings"
	"testing"
)

// The helper deterministically interrupts its first N calls. This exercises
// retry, result publication, exception identity and finally without scheduling
// races or sleeping. Only the owner is rebuilt; the oracle remains original.
func TestAdversarialCheckedRetryKeepsOriginalHandlerAndFinally(t *testing.T) {
	const source = `class RetryEffects {
 static int remaining,mode;static String trace;static final InterruptedException interrupted=new InterruptedException("original");static final RuntimeException fatal=new IllegalArgumentException("same");
 static void prefix(){trace+="B";}static long delay(){trace+="D";return Long.MIN_VALUE;}
 static boolean poll(long value,boolean first)throws InterruptedException{trace+="A";if(value!=Long.MIN_VALUE)throw new AssertionError("argument");if(remaining-->0)throw interrupted;if(mode==2)throw fatal;return mode==0;}
}
public class RetryOwner {
 private boolean awaitNanos(long value,boolean first)throws InterruptedException{return RetryEffects.poll(value,first);}
 public boolean run(){boolean interrupted=false;boolean first=true;
  try{while(true){try{RetryEffects.prefix();long delay=RetryEffects.delay();boolean result=this.awaitNanos(delay,first);return result;}
   catch(InterruptedException caught){if(caught!=RetryEffects.interrupted)throw new AssertionError("identity");RetryEffects.trace+="C";interrupted=true;first=false;}}}
  finally{if(interrupted)RetryEffects.trace+="I";}
 }
}
class RetryDriver {public static void main(String[]args){int rows=0;for(int count=0;count<3;count++)for(int mode=0;mode<3;mode++){
 RetryEffects.remaining=count;RetryEffects.mode=mode;RetryEffects.trace="";boolean result=false;Throwable failure=null;
 try{result=new RetryOwner().run();}catch(Throwable error){failure=error;}
 String expected="";for(int i=0;i<count;i++)expected+="BDAC";expected+="BDA";if(count>0)expected+="I";
 if(!expected.equals(RetryEffects.trace)||RetryEffects.remaining!=-1)throw new AssertionError("retry count/order "+RetryEffects.trace);
 if(mode==2){if(failure!=RetryEffects.fatal)throw new AssertionError("unchecked identity");}else if(failure!=null||result!=(mode==0))throw new AssertionError("result/handler",failure);rows++;
 }System.out.println(rows+":retry:checked:identity:finally");}}
`
	for _, name := range []string{"awaitNanos", "retryOperation"} {
		t.Run(name, func(t *testing.T) {
			for _, shape := range []string{"direct", "nested handler", "local join"} {
				t.Run(shape, func(t *testing.T) {
					fixture := strings.ReplaceAll(source, "awaitNanos", name)
					if shape == "nested handler" {
						fixture = strings.Replace(fixture, "try{RetryEffects.prefix();", "try{try{RetryEffects.prefix();}catch(IllegalArgumentException earlier){throw earlier;}", 1)
					}
					if shape == "local join" {
						fixture = strings.Replace(fixture, "long delay=RetryEffects.delay();", "long delay;if(first){delay=RetryEffects.delay();}else{long prior=RetryEffects.delay();delay=prior==Long.MIN_VALUE?prior:(prior^17L);}", 1)
					}
					testNativePrivateSetterCompiledFixtureWithPolicies(t, "RetryOwner", "RetryDriver", "9:retry:checked:identity:finally\n", func(t *testing.T, debug string) map[string][]byte {
						return nativeCompileSourceReleaseClasses(t, map[string]string{"RetryOwner.java": fixture}, debug, "8")
					}, nil, []string{"normal"})
				})
			}
		})
	}
}
