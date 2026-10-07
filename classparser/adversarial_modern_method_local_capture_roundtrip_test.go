package javaclassparser

import (
	"strings"
	"testing"
)

const modernLocalCaptureFixture = `abstract class ModernLocalBase {
 static Object published,seenToken;static long seenWord;static boolean fail;static final RuntimeException failure=new RuntimeException("identity");
 ModernLocalBase(){published=this;seenWord=word(0);seenToken=token();if(fail)throw failure;}
 abstract long word(long delta);abstract Object token();
}
class ModernLocalOwner {
 ModernLocalBase make(final long seed,final Object token){class Entry extends ModernLocalBase {long word(long delta){return seed+delta;}Object token(){return token;}}return new Entry();}
}
class ModernLocalDriver {public static void main(String[]args){int rows=0;Object identity=new Object();for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object token:new Object[]{null,identity})for(boolean fail:new boolean[]{false,true}){
 ModernLocalOwner owner=new ModernLocalOwner();ModernLocalBase.published=null;ModernLocalBase.seenToken=null;ModernLocalBase.fail=fail;ModernLocalBase value=null;
 try{value=owner.make(seed,token);if(fail)throw new AssertionError("no failure");}catch(RuntimeException e){if(!fail||e!=ModernLocalBase.failure)throw new AssertionError("error identity",e);value=(ModernLocalBase)ModernLocalBase.published;}
 if(value==null||ModernLocalBase.published!=value||ModernLocalBase.seenToken!=token||ModernLocalBase.seenWord!=seed||value.token()!=token||value.word(delta)!=java.math.BigInteger.valueOf(seed).add(java.math.BigInteger.valueOf(delta)).longValue())throw new AssertionError("capture before super/callback/overflow/publication");
 Class<?> c=value.getClass();if(!c.isLocalClass()||c.getDeclaringClass()!=null||c.getEnclosingClass()!=ModernLocalOwner.class||c.getEnclosingMethod().getDeclaringClass()!=ModernLocalOwner.class||!c.getEnclosingMethod().getName().equals("make")||!c.getName().equals("ModernLocalOwner$1Entry")||c.getDeclaredConstructors()[0].getParameterCount()!=3)throw new AssertionError("actual lexical owner/method/capture ABI");rows++;
 }System.out.println(rows+":modern-local:wide:identity:pre-super:exception:owner");}}
`

func TestAdversarialModernMethodLocalCaptureRoundTrip(t *testing.T) {
	for _, scope := range []string{"instance", "static"} {
		t.Run(scope, func(t *testing.T) {
			fixture := modernLocalCaptureFixture
			if scope == "static" {
				fixture = strings.Replace(fixture, "ModernLocalBase make(", "static ModernLocalBase make(", 1)
				fixture = strings.Replace(fixture, "getParameterCount()!=3", "getParameterCount()!=2", 1)
			}
			testSourceTargetReleaseFamilyFixture(t, fixture, "ModernLocalOwner", "ModernLocalDriver", "100:modern-local:wide:identity:pre-super:exception:owner\n", "11", []int{11, 16})
		})
	}
}

// The same Signature/hidden-parameter protocol exists before nestmates.
// Exercise actual compiler inputs rather than granting it on a version edit.
func TestAdversarialPreNestmateMethodLocalCaptureRoundTrip(t *testing.T) {
	for _, release := range []string{"9", "10"} {
		t.Run(release, func(t *testing.T) {
			testSourceTargetReleaseFamilyFixture(t, modernLocalCaptureFixture, "ModernLocalOwner", "ModernLocalDriver", "100:modern-local:wide:identity:pre-super:exception:owner\n", release, []int{11})
		})
	}
}
