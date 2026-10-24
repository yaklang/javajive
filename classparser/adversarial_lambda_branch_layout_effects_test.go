package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialLambdaBranchLayoutKeepsSinglePredicateEvaluationAndFailure(t *testing.T) {
	source := lambdaSynchronizedCaptureFixture
	replace := func(old, replacement string) {
		t.Helper()
		if strings.Count(source, old) != 1 {
			t.Fatal("missing unique independent oracle boundary", old)
		}
		source = strings.Replace(source, old, replacement, 1)
	}
	replace(`if(key==null||box[0]==null)`, `if(CaptureMonitorEvents.isNull(key,current)||box[0]==null)`)
	replace(`class CaptureMonitorEvents {static int calls;static void visit(Object lock){if(!Thread.holdsLock(lock))throw new AssertionError("lost monitor");calls++;}}`, `class CaptureMonitorEvents {
 static int calls,checks;static boolean fail;static final RuntimeException predicateFailure=new RuntimeException("original predicate payload");
 static boolean isNull(Object key,Object lock){if(!Thread.holdsLock(lock))throw new AssertionError("predicate outside monitor");checks++;if(fail)throw predicateFailure;return key==null;}
 static void visit(Object lock){if(!Thread.holdsLock(lock))throw new AssertionError("lost monitor");calls++;}
}`)
	replace(`for(int path=0;path<2;path++)`, `for(boolean fail:new boolean[]{false,true})for(int path=0;path<2;path++)`)
	replace(`CaptureMonitorEvents.calls=0;`, `CaptureMonitorEvents.calls=0;CaptureMonitorEvents.checks=0;CaptureMonitorEvents.fail=fail;`)
	replace(`if(CaptureMonitorEvents.calls!=0||`, `if(CaptureMonitorEvents.calls!=0||CaptureMonitorEvents.checks!=0||`)
	replace(`boolean codeWritten=boxWritten&&codeKind==2;`, `boolean codeWritten=boxWritten&&!fail&&codeKind==2;`)
	replace(`:codeKind==0?NullPointerException.class`, `:fail?RuntimeException.class:codeKind==0?NullPointerException.class`)
	replace(`CaptureMonitorEvents.calls!=(codeWritten?1:0)||`, `CaptureMonitorEvents.calls!=(codeWritten?1:0)||CaptureMonitorEvents.checks!=(boxWritten?1:0)||boxWritten&&fail&&caught!=CaptureMonitorEvents.predicateFailure||`)
	replace(`if(rows!=972)`, `if(rows!=1944)`)
	for _, named := range []bool{false, true} {
		label := "absent"
		if named {
			label = "named"
		}
		t.Run(label, func(t *testing.T) { testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "1944", named) })
	}
}
