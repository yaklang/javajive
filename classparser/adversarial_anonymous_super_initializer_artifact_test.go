package javaclassparser

import (
	"strings"
	"testing"
)

// The parent observes captures and publishes THIS before the instance
// initializer. An abrupt parent must keep those observations but never run
// the initializer. The unchanged caller checks those phases independently.
const artifactAnonymousSuperInitializerFixture = `public class ArtifactLambdaOwner {
 public static String trace="";public static Parent last;public static boolean fail;
 public static final RuntimeException error=new RuntimeException("parent");
 public long seed=7L;
 public static long prepare(long word){trace+="A";return word+11L;}
 public static abstract class Parent {
  public final long argument,seen;public final Object token;
  protected Parent(long argument,Object argumentToken){trace+="P";last=this;this.argument=argument;seen=read();token=peek();if(token!=argumentToken)throw new AssertionError("capture alias");if(fail)throw error;}
  public abstract long read();public abstract Object peek();public abstract boolean flip();
 }
 public MAKE_STATIC Parent make(final long word,final Object capturedToken){trace+="F";return new Parent(prepare(word),capturedToken){
  final java.util.concurrent.atomic.AtomicBoolean onlyOnce=new java.util.concurrent.atomic.AtomicBoolean(false);
  public long read(){trace+="R";return word*31L+OWNER_SEED;}
  public Object peek(){trace+="K";return capturedToken;}
  public boolean flip(){trace+="I";return onlyOnce.compareAndSet(false,true);}
 };}
}
class ArtifactLambdaDriver {
 public static void main(String[]args){int rows=0;Object shared=new Object();
  for(long word:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})for(Object token:new Object[]{null,shared})for(boolean abrupt:new boolean[]{false,true}){
   ArtifactLambdaOwner owner=new ArtifactLambdaOwner();ArtifactLambdaOwner.trace="";ArtifactLambdaOwner.last=null;ArtifactLambdaOwner.fail=abrupt;
   try{ArtifactLambdaOwner.Parent p=FACTORY_CALL.make(word,token);if(abrupt||p!=ArtifactLambdaOwner.last)throw new AssertionError("parent failure/publication");}
   catch(RuntimeException e){if(!abrupt||e!=ArtifactLambdaOwner.error)throw new AssertionError("parent exception identity",e);}
   ArtifactLambdaOwner.Parent p=ArtifactLambdaOwner.last;
   long expected=java.math.BigInteger.valueOf(word).multiply(java.math.BigInteger.valueOf(31L)).add(java.math.BigInteger.valueOf(7L)).longValue();
   long argument=java.math.BigInteger.valueOf(word).add(java.math.BigInteger.valueOf(11L)).longValue();
   if(p==null||p.argument!=argument||p.seen!=expected||p.token!=token||!ArtifactLambdaOwner.trace.equals("FAPRK"))throw new AssertionError("argument/capture/callback phase");
   if(abrupt){try{p.flip();throw new AssertionError("initializer ran after abrupt parent");}catch(NullPointerException required){}}
   else if(!p.flip()||p.flip())throw new AssertionError("instance initializer runs once");
   if(p.read()!=expected||p.peek()!=token)throw new AssertionError("later capture binding");
   rows++;
  }
  System.out.println(rows+":super-arguments:early-captures:initializer:exception:overflow:alias");
 }
}`

func TestAdversarialAnonymousSuperInitializerArtifactKeepsOriginalPhases(t *testing.T) {
	for _, static := range []bool{false, true} {
		name, modifier, seed, call := "instance owner", "", "seed", "owner"
		if static {
			name, modifier, seed, call = "static owner", "static", "7L", "ArtifactLambdaOwner"
		}
		t.Run(name, func(t *testing.T) {
			fixture := strings.NewReplacer("MAKE_STATIC", modifier, "OWNER_SEED", seed, "FACTORY_CALL", call).Replace(artifactAnonymousSuperInitializerFixture)
			testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "20:super-arguments:early-captures:initializer:exception:overflow:alias\n")
		})
	}
}
