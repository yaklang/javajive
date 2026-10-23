package javaclassparser

import (
	"strings"
	"testing"
)

// The catch is the original retry owner, not a source-printing convenience.
// Renaming the owner or checked operation must not change which handler runs.
// The untouched driver checks the original class first, including exception
// identity, multiple retries, successful completion and unchecked failure.
func TestAdversarialInterruptRetryRetainsOriginalHandlerOwnership(t *testing.T) {
	for _, owner := range []string{"Uninterruptibles", "RetryProtocol"} {
		t.Run(owner, func(t *testing.T) {
			for _, operation := range []string{"await", "join", "get", "take", "put", "acquire", "customOperation"} {
				t.Run(operation, func(t *testing.T) {
					testInterruptRetryOwnership(t, owner, operation)
				})
			}
		})
	}
}

func testInterruptRetryOwnership(t *testing.T, owner, operation string) {
	t.Helper()
	fixture := strings.ReplaceAll(interruptRetryOwnershipFixture, "$OWNER", owner)
	fixture = strings.ReplaceAll(fixture, "$OPERATION", operation)
	argument, parameter := "", ""
	if operation == "put" {
		argument, parameter = "token", "Object token"
	}
	fixture = strings.ReplaceAll(fixture, "$ARGUMENT", argument)
	fixture = strings.ReplaceAll(fixture, "$PARAMETER", parameter)
	testNativePrivateSetterCompiledFixtureWithPolicies(t, owner, "InterruptRetryDriver", "12:retry:handler:identity:finally\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
	}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
}

const interruptRetryOwnershipFixture = `class InterruptRetryEffects {
 static int remaining,mode;static String trace;
 static final InterruptedException checked=new InterruptedException("same checked");
 static final RuntimeException unchecked=new IllegalArgumentException("same unchecked");
 static void invoke()throws InterruptedException{trace+="A";if(remaining-->0)throw checked;if(mode==1)throw unchecked;}
}
class InterruptRetryOperation {void $OPERATION($PARAMETER)throws InterruptedException{InterruptRetryEffects.invoke();}}
public class $OWNER {
 public static void run(InterruptRetryOperation operation,Object token){boolean interrupted=false;
  try{while(true){try{operation.$OPERATION($ARGUMENT);return;}
   catch(InterruptedException caught){if(caught!=InterruptRetryEffects.checked)throw new AssertionError("checked identity");InterruptRetryEffects.trace+="C";interrupted=true;}}}
  finally{if(interrupted)InterruptRetryEffects.trace+="I";}
 }
}
class InterruptRetryDriver {public static void main(String[]args){int rows=0;
 for(int retries=0;retries<3;retries++)for(int mode=0;mode<2;mode++)for(boolean missing:new boolean[]{false,true}){
  InterruptRetryEffects.remaining=retries;InterruptRetryEffects.mode=mode;InterruptRetryEffects.trace="";Throwable failure=null;
  try{$OWNER.run(missing?null:new InterruptRetryOperation(),new Object());}catch(Throwable caught){failure=caught;}
  if(missing){if(!(failure instanceof NullPointerException)||InterruptRetryEffects.remaining!=retries||!InterruptRetryEffects.trace.equals(""))throw new AssertionError("null receiver",failure);}
  else{String trace="";for(int i=0;i<retries;i++)trace+="AC";trace+="A";if(retries>0)trace+="I";
   if(!trace.equals(InterruptRetryEffects.trace)||InterruptRetryEffects.remaining!=-1||failure!=(mode==1?InterruptRetryEffects.unchecked:null))throw new AssertionError("retry ownership "+InterruptRetryEffects.trace,failure);}
  rows++;
 }System.out.println(rows+":retry:handler:identity:finally");}}
`
