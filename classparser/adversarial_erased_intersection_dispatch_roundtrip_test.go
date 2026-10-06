package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialErasedIntersectionOverloadKeepsDispatchAndEffectsRoundTrip(t *testing.T) {
	for _, owner := range []string{"IntersectionDispatchOwner", "RenamedIntersectionDispatchOwner"} {
		t.Run(owner, func(t *testing.T) {
			fixture := `class IntersectionDispatchOwner {
 static int calls;
 public interface Tag{void mix(int n);}
 public static class Base{public int sum;}
 public static final class Narrow extends Base implements Tag{public final RuntimeException failure=new IllegalStateException("sentinel");public void mix(int n){sum=31*sum+n;if(n==13)throw failure;}}
 static <A extends Base & Tag> A write(A input,int delta){calls++;input.mix(delta);return input;}
 static Narrow write(Narrow input,int delta){write((Base & Tag)input,delta);return input;}
}
class IntersectionDispatchDriver{public static void main(String[]args){int rows=0;for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int delta:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE,13}){IntersectionDispatchOwner.Narrow input=new IntersectionDispatchOwner.Narrow();input.sum=seed;IntersectionDispatchOwner.calls=0;int expected=java.math.BigInteger.valueOf(seed).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(delta)).intValue();try{if(IntersectionDispatchOwner.write(input,delta)!=input)throw new AssertionError("original result identity");if(delta==13)throw new AssertionError("missing original failure");}catch(RuntimeException failure){if(delta!=13||failure!=input.failure)throw new AssertionError("original exception identity");}if(input.sum!=expected||IntersectionDispatchOwner.calls!=1)throw new AssertionError("original generic dispatch and partial effects");rows++;}System.out.println(rows+":intersection:dispatch:identity:effects");}}
`
			fixture = strings.ReplaceAll(fixture, "IntersectionDispatchOwner", owner)
			testNativePrivateSetterFixture(t, fixture, owner, "IntersectionDispatchDriver", "30:intersection:dispatch:identity:effects\n")
		})
	}
}
