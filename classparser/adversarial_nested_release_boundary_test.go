package javaclassparser

import (
	"strings"
	"testing"
)

// The copied normal cleanup and continuation are outside the original catch
// domain. A failure in either must not enter the exceptional cleanup copy.
func TestAdversarialNestedReleaseKeepsCleanupAndContinuationOutsideHandler(t *testing.T) {
	const fixture = `
class ReleaseCell {
 final java.util.concurrent.locks.ReentrantLock lock=new java.util.concurrent.locks.ReentrantLock();
 boolean pending,aborted;int count;
 void acquire(){lock.lock();ReleaseEffects.events.append("L;");}
 void release(){ReleaseEffects.events.append("U;");lock.unlock();ReleaseEffects.fail(4);}
 boolean isPending(){return pending;}boolean isAborted(){return aborted;}
 boolean isHeld(){return lock.isHeldByCurrentThread();}
 boolean checkState(){ReleaseEffects.events.append("Q;");ReleaseEffects.fail(7);return !isHeld()&&ReleaseEffects.mode!=6;}
 long work(){ReleaseEffects.events.append("W;");ReleaseEffects.fail(1);count++;return 19;}
}
class ReleaseToken {final boolean remove;ReleaseToken(boolean remove){this.remove=remove;}boolean isRemoval(){return remove;}}
class ReleaseEffects {
 static int mode;static StringBuilder events=new StringBuilder();
 static final RuntimeException FAILURE=new IllegalArgumentException("identity");
 static void fail(int phase){if(mode==phase)throw FAILURE;}
 static void abort(){events.append("A;");}
 static Object message(){events.append("M;");return "release must finish";}
 static void account(int delta){events.append("C").append(delta).append(';');fail(2);}
 static Object select(ReleaseCell cell,boolean remove){events.append(remove?"D;":"K;");fail(3);return cell;}
 static void recycle(ReleaseCell cell){events.append("F;");cell.release();}
 static boolean after(Object result){events.append("P;");fail(5);return result!=null;}
}
public class ReleaseBoundary {
 static long run(ReleaseCell cell,ReleaseToken token){
  cell.acquire();long result;Object next;
  try {
   int before=cell.count;
   try {result=cell.work();}finally {if(cell.isAborted())ReleaseEffects.abort();ReleaseEffects.account(cell.count-before);}
   boolean remove=token!=null&&token.isRemoval();next=ReleaseEffects.select(cell,remove);
  } finally {
   if(cell.isPending()||cell.isAborted())cell.release();else ReleaseEffects.recycle(cell);
   assert cell.checkState():ReleaseEffects.message();
  }
  if(ReleaseEffects.after(next))result=-result;return result;
 }
}
class ReleaseDriver {public static void main(String[]args){int rows=0;
 final boolean enabled=ASSERT_ENABLED;ClassLoader.getSystemClassLoader().setClassAssertionStatus("ReleaseBoundary",enabled);
 for(int mode=0;mode<8;mode++)for(boolean pending:new boolean[]{false,true})for(boolean aborted:new boolean[]{false,true})for(int tokenKind=0;tokenKind<3;tokenKind++){
  ReleaseCell cell=new ReleaseCell();cell.pending=pending;cell.aborted=aborted;
  ReleaseToken token=tokenKind==0?null:new ReleaseToken(tokenKind==2);
  ReleaseEffects.mode=mode;ReleaseEffects.events.setLength(0);long result=0;Throwable failure=null;
  try{result=ReleaseBoundary.run(cell,token);}catch(Throwable thrown){failure=thrown;}
  String expected="L;W;"+(aborted?"A;":"")+"C"+(mode==1?0:1)+";";
  if(mode!=1&&mode!=2)expected+=tokenKind==2?"D;":"K;";
  expected+=(pending||aborted?"":"F;")+"U;";
  if(enabled&&mode!=4)expected+="Q;";
  if(enabled&&mode==6)expected+="M;";
  if(mode==0||mode==5||!enabled&&mode>=6)expected+="P;";
  boolean normal=mode==0||!enabled&&mode>=6;
  boolean correctFailure=normal?failure==null&&result==-19:enabled&&mode==6?failure instanceof AssertionError&&"release must finish".equals(failure.getMessage()):failure==ReleaseEffects.FAILURE;
  if(!expected.equals(ReleaseEffects.events.toString())||cell.isHeld()||cell.count!=(mode==1?0:1)||!correctFailure)throw new AssertionError("phase="+mode+" events="+ReleaseEffects.events+" expected="+expected,failure);
  rows++;
 }System.out.println(rows+":nested:release:handler-domain:exception-identity");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		for _, enabled := range []string{"false", "true"} {
			t.Run(spelling+"/assert="+enabled, func(t *testing.T) {
				source, owner := strings.ReplaceAll(fixture, "ASSERT_ENABLED", enabled), "ReleaseBoundary"
				if spelling == "renamed" {
					source = strings.NewReplacer("ReleaseBoundary", "CompletionPhase", "ReleaseCell", "Lease", "ReleaseToken", "Instruction", "isPending", "isRetired", "isAborted", "isInvalid", "release()", "finish()", "run(", "evaluate(").Replace(source)
					owner = "CompletionPhase"
				}
				testAuthoredOwnerAcrossSourceModes(t, source, owner, "ReleaseDriver", "96:nested:release:handler-domain:exception-identity\n")
			})
		}
	}
}
