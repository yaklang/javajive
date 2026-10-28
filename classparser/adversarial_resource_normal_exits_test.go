package javaclassparser

import (
	"strings"
	"testing"
)

// The JVM has separate normal close tails for a selected return and ordinary
// fall-through. Neither tail may become unconditional or protected by the
// body's handler. Assert independent results, effects and exception identity.
func TestAdversarialResourceBranchKeepsReturnAndNormalExit(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class LeaseSelector {
 public static int closes,body,after,events;
 public static RuntimeException BODY=new RuntimeException("body"),CLOSE=new RuntimeException("close");
 public static AutoCloseable lease(final boolean fault){return new AutoCloseable(){public void close(){closes++;events=events*10+2;if(fault)throw CLOSE;}};}
 public static String detect(int count,boolean hit,boolean badBody,boolean badClose,boolean missing,boolean escape){
  try{try(AutoCloseable l=missing?null:lease(badClose)){int steps=0;while(steps<count){body++;events=events*10+1;if(badBody)throw BODY;steps++;}if(hit||steps>3)return "match";}}
  catch(Exception failure){events=events*10+3;if(failure!=BODY&&failure!=CLOSE)throw new AssertionError("identity");if(failure==BODY&&badClose&&!missing){if(failure.getSuppressed().length!=1||failure.getSuppressed()[0]!=CLOSE)throw new AssertionError("suppression");}else if(failure.getSuppressed().length!=0)throw new AssertionError("unexpected suppression");if(escape)throw (RuntimeException)failure;}
  after++;events=events*10+4;return "miss";
 }
}
class LeaseSelectorDriver{public static void main(String[]args){int count=0;
 for(int n=0;n<7;n++)for(boolean hit:new boolean[]{false,true})for(boolean badBody:new boolean[]{false,true})for(boolean badClose:new boolean[]{false,true})for(boolean missing:new boolean[]{false,true})for(boolean escape:new boolean[]{false,true}){
 LeaseSelector.BODY=new RuntimeException("body");LeaseSelector.CLOSE=new RuntimeException("close");
 LeaseSelector.closes=LeaseSelector.body=LeaseSelector.after=LeaseSelector.events=0;
 boolean bodyFails=n>0&&badBody, closeFails=badClose&&!missing, failed=bodyFails||closeFails, thrown=failed&&escape;
 String want=failed?"miss":hit||n>3?"match":"miss";
 String actual=null;RuntimeException observed=null;
 try{actual=LeaseSelector.detect(n,hit,badBody,badClose,missing,escape);}catch(RuntimeException failure){observed=failure;}
 if(thrown){if(observed!=(bodyFails?LeaseSelector.BODY:LeaseSelector.CLOSE))throw new AssertionError("escape identity");}
 else if(observed!=null||!want.equals(actual))throw new AssertionError("result");
 int visits=bodyFails?1:n, expected=0;for(int i=0;i<visits;i++)expected=expected*10+1;
 if(!missing)expected=expected*10+2;if(failed)expected=expected*10+3;
 int falls=!thrown&&want.equals("miss")?1:0;if(falls==1)expected=expected*10+4;
 if(LeaseSelector.closes!=(missing?0:1)||LeaseSelector.body!=visits||LeaseSelector.after!=falls||LeaseSelector.events!=expected)throw new AssertionError("resource effect order");count++;
 }System.out.println(count+":resource:exit");}}
`
	for _, owner := range []string{"LeaseSelector", "HandleRoute"} {
		t.Run(owner, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "LeaseSelector", owner), owner, owner+"Driver", "224:resource:exit\n")
		})
	}
}

func TestAdversarialProtectedConstructorKeepsAbruptAndNormalExits(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
import java.io.IOException;
public class GuardedConstruction {
 public static final IOException IO=new IOException("read");
 public static int reads,stored;
 public int state;
 public static int read(int flags,boolean fail)throws IOException{reads++;if(fail)throw IO;return flags;}
 public GuardedConstruction(int flags,boolean fail){
  try{state=read(flags,fail);stored++;if((state&1)!=0)throw new IllegalArgumentException("first");if((state&4)!=0)throw new IllegalArgumentException("second");return;}
  catch(IOException failure){throw new IllegalArgumentException("io",failure);}
 }
}
class GuardedConstructionDriver{public static void main(String[]args){int rows=0;
 for(int flags=-9;flags<=9;flags++)for(boolean fail:new boolean[]{false,true}){
 GuardedConstruction.reads=GuardedConstruction.stored=0;GuardedConstruction value=null;IllegalArgumentException caught=null;
 try{value=new GuardedConstruction(flags,fail);}catch(IllegalArgumentException failure){caught=failure;}
 String want=fail?"io":(flags&1)!=0?"first":(flags&4)!=0?"second":null;
 if(want==null){if(caught!=null||value==null||value.state!=flags)throw new AssertionError("normal construction");}
 else if(caught==null||!want.equals(caught.getMessage())||caught.getCause()!=(fail?GuardedConstruction.IO:null))throw new AssertionError("abrupt construction");
 if(GuardedConstruction.reads!=1||GuardedConstruction.stored!=(fail?0:1))throw new AssertionError("construction effects");rows++;
 }System.out.println(rows+":protected:constructor");}}
`
	for _, name := range []string{"GuardedConstruction", "CheckedState"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "GuardedConstruction", name), name, name+"Driver", "38:protected:constructor\n")
		})
	}
}
