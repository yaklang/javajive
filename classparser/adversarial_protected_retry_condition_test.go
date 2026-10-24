package javaclassparser

import (
	"strings"
	"testing"
)

const protectedRetryConditionFixture = `class RetryConditionEffects {
 static int remaining,mode,initial;static long start;static String trace;
 static final InterruptedException checked=new InterruptedException("checked");
 static final RuntimeException unchecked=new IllegalStateException("unchecked");
}
class RetryConditionOperation {
 boolean fast(){RetryConditionEffects.trace+="F";return true;}
 boolean $OP(long remaining)throws InterruptedException{long expected=java.math.BigInteger.valueOf(RetryConditionEffects.start).subtract(java.math.BigInteger.valueOf(RetryConditionEffects.initial-RetryConditionEffects.remaining)).longValue();if(remaining!=expected)throw new AssertionError("loop carried remaining");RetryConditionEffects.trace+="A";if(RetryConditionEffects.remaining-->0)throw RetryConditionEffects.checked;if(RetryConditionEffects.mode==2)throw RetryConditionEffects.unchecked;return RetryConditionEffects.mode==1;}
 boolean finish(){RetryConditionEffects.trace+="Z";return true;}
}
public class RetryConditionOwner {
 public static boolean run(RetryConditionOperation operation,boolean shortcut,long timeout){boolean interrupted=false;
  try {if(!shortcut||!operation.fast()){long remaining=timeout;while(true){
   try{if(operation.$OP(remaining))break;return false;}
   catch(InterruptedException caught){if(caught!=RetryConditionEffects.checked)throw new AssertionError("checked identity");RetryConditionEffects.trace+="C";interrupted=true;remaining--;}
  }}return operation.finish();}
  finally{if(interrupted)RetryConditionEffects.trace+="I";}
 }
}
class RetryConditionDriver {public static void main(String[]args){int rows=0;
 for(long timeout:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(int retries=0;retries<3;retries++)for(int mode=0;mode<3;mode++)for(boolean shortcut:new boolean[]{false,true})for(boolean missing:new boolean[]{false,true}){
  RetryConditionEffects.remaining=retries;RetryConditionEffects.initial=retries;RetryConditionEffects.start=timeout;RetryConditionEffects.mode=mode;RetryConditionEffects.trace="";Throwable failure=null;boolean value=false;
  try{value=RetryConditionOwner.run(missing?null:new RetryConditionOperation(),shortcut,timeout);}catch(Throwable caught){failure=caught;}
  if(missing){if(!(failure instanceof NullPointerException)||RetryConditionEffects.remaining!=retries||!RetryConditionEffects.trace.equals(""))throw new AssertionError("receiver/evaluation",failure);}
  else if(shortcut){if(!value||failure!=null||RetryConditionEffects.remaining!=retries||!RetryConditionEffects.trace.equals("FZ"))throw new AssertionError("shortcut",failure);}
  else{String trace="";for(int i=0;i<retries;i++)trace+="AC";trace+="A";if(mode==1)trace+="Z";if(retries>0)trace+="I";
   if(!trace.equals(RetryConditionEffects.trace)||RetryConditionEffects.remaining!=-1||failure!=(mode==2?RetryConditionEffects.unchecked:null)||value!=(mode==1))throw new AssertionError("protected condition "+RetryConditionEffects.trace,failure);}
  rows++;
 }System.out.println(rows+":protected:retry:condition:handler:identity");}}
`

func TestAdversarialProtectedRetryConditionKeepsInvocationInsideHandler(t *testing.T) {
	for _, operation := range []string{"tryLock", "pollResult"} {
		t.Run(operation, func(t *testing.T) {
			fixture := strings.ReplaceAll(protectedRetryConditionFixture, "$OP", operation)
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "RetryConditionOwner", "RetryConditionDriver", "108:protected:retry:condition:handler:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{"RetryConditionOwner.java": fixture}, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
		})
	}
}

// The successful continuation throws the same checked exception class as the
// retry predicate. It must propagate once, not re-enter the retry handler.
// Cleanup failure then replaces that pending exception, preserving Java's
// finally precedence and the exact original exception objects.
func TestAdversarialProtectedRetryKeepsDownstreamCheckedEffectsOutsideHandler(t *testing.T) {
	fixture := strings.ReplaceAll(protectedRetryConditionFixture, "$OP", "pollResult")
	fixture = strings.Replace(fixture, "static int remaining,mode,initial;static long start;static String trace;", "static int remaining,mode,initial;static long start;static String trace;static boolean late,cleanup;static final RuntimeException cleanupFailure=new IllegalArgumentException(\"cleanup\");", 1)
	fixture = strings.Replace(fixture, "boolean finish(){RetryConditionEffects.trace+=\"Z\";return true;}", "boolean finish()throws InterruptedException{RetryConditionEffects.trace+=\"Z\";if(RetryConditionEffects.late)throw RetryConditionEffects.checked;return true;}", 1)
	fixture = strings.Replace(fixture, "long timeout){boolean interrupted=false;", "long timeout)throws InterruptedException{boolean interrupted=false;", 1)
	fixture = strings.Replace(fixture, "finally{if(interrupted)RetryConditionEffects.trace+=\"I\";}", "finally{if(interrupted)RetryConditionEffects.trace+=\"I\";if(RetryConditionEffects.cleanup){RetryConditionEffects.trace+=\"K\";throw RetryConditionEffects.cleanupFailure;}}", 1)
	fixture = fixture[:strings.Index(fixture, "class RetryConditionDriver")] + `class RetryConditionDriver {public static void main(String[]args){int rows=0;
 for(long timeout:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(int retries=0;retries<3;retries++)for(int mode=0;mode<3;mode++)for(boolean shortcut:new boolean[]{false,true})for(boolean missing:new boolean[]{false,true})for(boolean late:new boolean[]{false,true})for(boolean cleanup:new boolean[]{false,true}){
  RetryConditionEffects.remaining=retries;RetryConditionEffects.initial=retries;RetryConditionEffects.start=timeout;RetryConditionEffects.mode=mode;RetryConditionEffects.trace="";RetryConditionEffects.late=late;RetryConditionEffects.cleanup=cleanup;Throwable failure=null;boolean value=false;
  try{value=RetryConditionOwner.run(missing?null:new RetryConditionOperation(),shortcut,timeout);}catch(Throwable caught){failure=caught;}
  String trace="";Throwable expected=null;boolean result=false;int remaining=retries;
  if(missing){if(!cleanup&&!(failure instanceof NullPointerException))throw new AssertionError("receiver",failure);expected=failure;}
  else if(shortcut){trace="FZ";expected=late?RetryConditionEffects.checked:null;result=!late;}
  else{for(int i=0;i<retries;i++)trace+="AC";trace+="A";if(mode==1)trace+="Z";if(retries>0)trace+="I";remaining=-1;expected=mode==2?RetryConditionEffects.unchecked:mode==1&&late?RetryConditionEffects.checked:null;result=mode==1&&!late;}
  if(cleanup){trace+="K";expected=RetryConditionEffects.cleanupFailure;result=false;}
  if(!trace.equals(RetryConditionEffects.trace)||RetryConditionEffects.remaining!=remaining||failure!=expected||value!=result)throw new AssertionError("downstream domain "+RetryConditionEffects.trace,failure);
  rows++;
 }System.out.println(rows+":retry:downstream:cleanup:identity");}}`
	testNativePrivateSetterCompiledFixtureWithPolicies(t, "RetryConditionOwner", "RetryConditionDriver", "432:retry:downstream:cleanup:identity\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, map[string]string{"RetryConditionOwner.java": fixture}, debug, "8")
	}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
}
