package javaclassparser

import (
	"strings"
	"testing"
)

// The lambda in the allocation argument belongs to the outer class. A lambda
// in the anonymous method belongs to the allocated class instead. Both execute
// observable effects, while an unrelated nested lambda exercises postorder
// registration in the outer class.
const anonymousLambdaScopeFixture = `import java.util.function.LongUnaryOperator;
public class ArtifactLambdaOwner {
 public static String trace="";public long state;
 public static abstract class Parent implements Runnable {
  public final long seeded;
  protected Parent(LongUnaryOperator initializer,long seed){trace+="P";seeded=initializer.applyAsLong(seed);}
 }
 public Parent make(final long delta,long seed){return new Parent(x->{trace+="B";return x+delta;},seed){
  public void run(){LongUnaryOperator step=x->{trace+="A";return x*31L+delta;};state=step.applyAsLong(state);}
 };}
 public static LongUnaryOperator nested(final long offset){return x->((LongUnaryOperator)(y->y+offset)).applyAsLong(x);}
 public long run(long seed,long delta){Parent p=make(delta,seed);state=p.seeded;p.run();p.run();return nested(7L).applyAsLong(state);}
}
class ArtifactLambdaDriver {
 static long add(long a,long b){return java.math.BigInteger.valueOf(a).add(java.math.BigInteger.valueOf(b)).longValue();}
 static long step(long a,long b){return java.math.BigInteger.valueOf(a).multiply(java.math.BigInteger.valueOf(31L)).add(java.math.BigInteger.valueOf(b)).longValue();}
 public static void main(String[]args){int rows=0;
  for(long seed:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,0L,Long.MAX_VALUE}){
   ArtifactLambdaOwner owner=new ArtifactLambdaOwner();ArtifactLambdaOwner.trace="";long result=owner.run(seed,delta);
   long state=step(step(add(seed,delta),delta),delta);
   if(owner.state!=state||result!=add(state,7L)||!ArtifactLambdaOwner.trace.equals("PBAA"))throw new AssertionError("class-local lambda binding/order/overflow");
   rows++;
  }
  System.out.println(rows+":anonymous-class:outer-argument:inner-lambda:nested-order:overflow");
 }
}`

func TestAdversarialAnonymousMethodLambdaKeepsItsOwnClassScope(t *testing.T) {
	// Isolate class ownership from the separate hidden-field capture metadata
	// domain. The callback receives its own original ordinary method parameter.
	fixture := strings.ReplaceAll(anonymousLambdaScopeFixture, "class Parent implements Runnable {", "class Parent {public abstract void accept(long input);")
	fixture = strings.ReplaceAll(fixture, "public void run(){LongUnaryOperator step=x->{trace+=\"A\";return x*31L+delta;}", "public void accept(final long input){LongUnaryOperator step=x->{trace+=\"A\";return x*31L+input;}")
	fixture = strings.ReplaceAll(fixture, "p.run();p.run();", "p.accept(delta);p.accept(delta);")
	testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "15:anonymous-class:outer-argument:inner-lambda:nested-order:overflow\n")
}
