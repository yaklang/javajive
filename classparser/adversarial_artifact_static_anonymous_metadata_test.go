package javaclassparser

import (
	"strings"
	"testing"
)

// Both compilers must preserve the capture stores before the superclass can
// call virtual methods, publish the receiver, or fail. The native compiler's
// final class/static InnerClasses metadata is an additional, separate contract.
const artifactStaticAnonymousMetadataFixture = `public class ArtifactLambdaOwner {
 public static String trace="";public static Parent last;public static boolean fail;
 public static final RuntimeException error=new RuntimeException("identity");
 public static abstract class Parent {
  public final long seen;public final Object token;
  protected Parent(){trace+="P";last=this;seen=read();token=peek();if(fail)throw error;}
  public abstract long read();public abstract Object peek();
 }
 public static Parent make(final long word,final Object capturedToken){trace+="F";return new Parent(){
  public long read(){trace+="R";return word*31L+7L;}
  public Object peek(){trace+="K";return capturedToken;}
 };}
}
class ArtifactLambdaDriver {
 public static void main(String[]args)throws Exception {
  int rows=0;Object shared=new Object();
  for(long word:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})
   for(Object token:new Object[]{null,shared})for(boolean abrupt:new boolean[]{false,true}){
    ArtifactLambdaOwner.trace="";ArtifactLambdaOwner.last=null;ArtifactLambdaOwner.fail=abrupt;
    try {ArtifactLambdaOwner.Parent p=ArtifactLambdaOwner.make(word,token);if(abrupt||p!=ArtifactLambdaOwner.last)throw new AssertionError("factory failure/publication");}
    catch(RuntimeException e){if(!abrupt||e!=ArtifactLambdaOwner.error)throw new AssertionError("original exception identity",e);}
    ArtifactLambdaOwner.Parent p=ArtifactLambdaOwner.last;
    long expected=java.math.BigInteger.valueOf(word).multiply(java.math.BigInteger.valueOf(31L)).add(java.math.BigInteger.valueOf(7L)).longValue();
    if(p==null||p.seen!=expected||p.token!=token||!ArtifactLambdaOwner.trace.equals("FPRK"))throw new AssertionError("early capture/callback/failure phase");
    if(p.read()!=expected||p.peek()!=token||!ArtifactLambdaOwner.trace.equals("FPRKRK"))throw new AssertionError("wide binding/overflow/alias");
    Class<?> c=p.getClass();if(!c.isAnonymousClass()||c.getDeclaringClass()!=null||c.getEnclosingMethod()==null||!c.getEnclosingMethod().getName().equals("make")||c.getEnclosingMethod().getDeclaringClass()!=ArtifactLambdaOwner.class)throw new AssertionError("original anonymous ownership");
    rows++;
   }
  System.out.println(rows+":artifact:static:early-capture:callback:exception:overflow:alias");
 }
}`

func TestAdversarialArtifactStaticAnonymousMetadataKeepsEarlyCaptureOrder(t *testing.T) {
	for _, shadow := range []bool{false, true} {
		name, fixture := "captured reference", artifactStaticAnonymousMetadataFixture
		if shadow {
			name = "inherited field shadows lexical parameter"
			// An inherited member wins this source lookup. Both independent
			// original compilers read Parent.token instead of capturing the
			// same-spelled factory parameter. Its early/default value is null.
			fixture = strings.ReplaceAll(fixture, "capturedToken", "token")
			fixture = strings.ReplaceAll(fixture, "p.token!=token", "p.token!=null")
			fixture = strings.ReplaceAll(fixture, "p.peek()!=token", "p.peek()!=null")
		}
		t.Run(name, func(t *testing.T) {
			testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "20:artifact:static:early-capture:callback:exception:overflow:alias\n")
		})
	}
}
