package javaclassparser

import "testing"

const artifactInstanceLambdaMetadataFixture = `import java.util.function.LongUnaryOperator;
public class ArtifactLambdaOwner {
 private final long base;private int calls;
 public ArtifactLambdaOwner(long base){this.base=base;}
 int calls(){return calls;}
 LongUnaryOperator bind(final long delta){return value->{calls++;return base*31L+delta+value;};}
 LongUnaryOperator nested(final long delta){return offset->{LongUnaryOperator next=value->{calls++;return base+delta+offset+value;};return next.applyAsLong(offset);};}
}
class ArtifactLambdaDriver {
 static long plain(long base,long delta,long value){return java.math.BigInteger.valueOf(base).multiply(java.math.BigInteger.valueOf(31L)).add(java.math.BigInteger.valueOf(delta)).add(java.math.BigInteger.valueOf(value)).longValue();}
 static long nested(long base,long delta,long value){return java.math.BigInteger.valueOf(base).add(java.math.BigInteger.valueOf(delta)).add(java.math.BigInteger.valueOf(value).multiply(java.math.BigInteger.valueOf(2L))).longValue();}
 public static void main(String[] args){int rows=0;
  for(long base:new long[]{Long.MIN_VALUE,Integer.MIN_VALUE,-1L,0L,1L,Integer.MAX_VALUE,Long.MAX_VALUE})
   for(long delta:new long[]{Long.MIN_VALUE,0L,Long.MAX_VALUE})
    for(long value:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE}){
     ArtifactLambdaOwner owner=new ArtifactLambdaOwner(base),other=new ArtifactLambdaOwner(base^17L);
     LongUnaryOperator first=owner.bind(delta),second=owner.nested(delta),peer=other.bind(delta);
     if(owner.calls()!=0||other.calls()!=0)throw new AssertionError("eager receiver/capture effect");
     if(first.applyAsLong(value)!=plain(base,delta,value)||owner.calls()!=1||other.calls()!=0)throw new AssertionError("receiver, wide capture or arithmetic");
     if(peer.applyAsLong(value)!=plain(base^17L,delta,value)||owner.calls()!=1||other.calls()!=1)throw new AssertionError("receiver alias leaked");
     if(second.applyAsLong(value)!=nested(base,delta,value)||owner.calls()!=2||other.calls()!=1)throw new AssertionError("nested receiver/entry words");
     if(first.applyAsLong(value)!=plain(base,delta,value)||owner.calls()!=3||other.calls()!=1)throw new AssertionError("repeated closure state");rows++;
    }
  System.out.println(rows+":artifact:instance:wide:nested:alias:overflow");
 }
}`

func TestAdversarialArtifactInstanceLambdaMetadataKeepsReceiverAndWideCaptures(t *testing.T) {
	testArtifactLambdaMetadata(t, artifactInstanceLambdaMetadataFixture, []bool{false, true}, "105:artifact:instance:wide:nested:alias:overflow\n")
}

const artifactInterfaceLambdaMetadataFixture = `public class ArtifactLambdaOwner {
 static int leftCalls,rightCalls;
 public interface Rule {boolean matches(long value);default Rule and(Rule next){return value->matches(value)&&next.matches(value);}}
 static Rule left(boolean result){return value->{leftCalls++;return result;};}
 static Rule right(boolean result){return value->{rightCalls++;return result;};}
}
class ArtifactLambdaDriver {
 public static void main(String[]args){int rows=0;
  for(long value:new long[]{Long.MIN_VALUE,Integer.MIN_VALUE,-1L,0L,1L,Integer.MAX_VALUE,Long.MAX_VALUE}){
   for(boolean a:new boolean[]{false,true})for(boolean b:new boolean[]{false,true}){
    ArtifactLambdaOwner.leftCalls=0;ArtifactLambdaOwner.rightCalls=0;
    ArtifactLambdaOwner.Rule rule=ArtifactLambdaOwner.left(a).and(ArtifactLambdaOwner.right(b));
    if(ArtifactLambdaOwner.leftCalls!=0||ArtifactLambdaOwner.rightCalls!=0)throw new AssertionError("eager bound-interface body");
    if(rule.matches(value)!=(a&&b)||ArtifactLambdaOwner.leftCalls!=1||ArtifactLambdaOwner.rightCalls!=(a?1:0))throw new AssertionError("receiver, short circuit or captured peer");rows++;
   }
   for(boolean a:new boolean[]{false,true}){
    ArtifactLambdaOwner.leftCalls=0;ArtifactLambdaOwner.rightCalls=0;
    ArtifactLambdaOwner.Rule rule=ArtifactLambdaOwner.left(a).and(null);
    if(ArtifactLambdaOwner.leftCalls!=0)throw new AssertionError("eager null capture");
    try{if(rule.matches(value)||a)throw new AssertionError("missing deferred peer null failure");}
    catch(NullPointerException expected){if(!a)throw new AssertionError("short circuit evaluated null peer");}
    if(ArtifactLambdaOwner.leftCalls!=1||ArtifactLambdaOwner.rightCalls!=0)throw new AssertionError("null failure phase");rows++;
   }
  }
  System.out.println(rows+":artifact:interface:receiver:shortcircuit:nullphase");
 }
}`

func TestAdversarialArtifactInterfaceLambdaMetadataKeepsPrivateReceiverAndShortCircuit(t *testing.T) {
	testArtifactLambdaMetadata(t, artifactInterfaceLambdaMetadataFixture, []bool{false, true}, "42:artifact:interface:receiver:shortcircuit:nullphase\n")
}
