package javaclassparser

import (
	"strings"
	"testing"
)

// Pre/post update order is an operand-stack property, not a loop-body shape.
// The driver computes every expected visit/index and the final counter itself.
const decrementGuardFixture = `class DecrementGuardOwner {
 static String pre(int n){int count=0;StringBuilder visits=new StringBuilder();while(--n>=0){visits.append(n).append(',');count++;}return count+":"+n+":"+visits;}
 static String post(int n){int count=0;StringBuilder visits=new StringBuilder();while(n-->0){visits.append(n).append(',');count++;}return count+":"+n+":"+visits;}
 static String bodyUpdate(int n){int count=0;StringBuilder visits=new StringBuilder();while(n>=0){visits.append(n).append(',');n--;count++;}return count+":"+n+":"+visits;}
 static int reverse(int[]a){int i=a.length;int sum=0;while(--i>=0)sum+=a[i];return sum;}
}
class DecrementGuardDriver{public static void main(String[]args){int rows=0;for(int n=-2;n<9;n++){StringBuilder visits=new StringBuilder();for(int j=n-1;j>=0;j--)visits.append(j).append(',');String expected=Math.max(0,n)+":"+(n>0?-1:n-1)+":"+visits;if(!DecrementGuardOwner.pre(n).equals(expected)||!DecrementGuardOwner.post(n).equals(expected))throw new AssertionError("pre/post visits "+n);StringBuilder body=new StringBuilder();for(int j=n;j>=0;j--)body.append(j).append(',');String bodyExpected=Math.max(0,n+1)+":"+(n>=0?-1:n)+":"+body;if(!DecrementGuardOwner.bodyUpdate(n).equals(bodyExpected))throw new AssertionError("body update "+n);rows++;}for(int[]a:new int[][]{{},{1},{-1,3,8},{Integer.MAX_VALUE,1},{Integer.MIN_VALUE,-1}}){int expected=0;for(int value:a)expected+=value;if(DecrementGuardOwner.reverse(a)!=expected)throw new AssertionError("array bounds/overflow");rows++;}System.out.println(rows+":decrement:stack-order:visits:final-counter:array-bounds");}}
`

func TestAdversarialDecrementGuardPreservesLoadedValueRoundTrip(t *testing.T) {
	for _, owner := range []string{"DecrementGuardOwner", "RenamedDecrementOwner"} {
		t.Run(owner, func(t *testing.T) {
			testNativePrivateSetterFixture(t, strings.ReplaceAll(decrementGuardFixture, "DecrementGuardOwner", owner), owner, "DecrementGuardDriver", "16:decrement:stack-order:visits:final-counter:array-bounds\n")
		})
	}
}
