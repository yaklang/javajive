package javaclassparser

import (
	"strings"
	"testing"
)

const constructorSilentLoopCaptureFixture = `class FixedCaptureParent{final int n,sum;FixedCaptureParent(int n){if(n!=7)throw new IllegalArgumentException("input:"+n);int total=0;for(int i=0;i<n;i++){total+=i;}this.n=n;sum=total;}}final class FixedCaptureChild extends FixedCaptureParent{final Object value;FixedCaptureChild(Object value){super(7);this.value=value;}}class FixedCaptureDriver{public static void main(String[]args){Object x=new Object();for(Object v:new Object[]{null,x}){FixedCaptureChild c=new FixedCaptureChild(v);int expected=c.n<0?0:(int)(((long)c.n*(c.n-1))/2);if(c.n!=7||c.value!=v||c.sum!=expected)throw new AssertionError("capture/loop");System.out.println("7:"+(c.value==x));}for(int n:new int[]{-1,8})try{new FixedCaptureParent(n);throw new AssertionError("failure lost");}catch(IllegalArgumentException e){System.out.println(e.getMessage());}}}`

// Iteration counts include 70000: the movement certificate must establish a
// receiver/effect invariant, rather than unroll or specialize the loop. The
// parent and driver remain original bytes, including their failure behavior.
func TestAdversarialConstructorReceiverSilentLoopCaptureRoundTrip(t *testing.T) {
	testConstructorOriginalFixedCapturePathRoundTrip(t, constructorSilentLoopCaptureFixture)
}

func TestAdversarialConstructorReceiverSilentDoLoopCaptureRoundTrip(t *testing.T) {
	f := strings.Replace(constructorSilentLoopCaptureFixture, "for(int i=0;i<n;i++){total+=i;}", "int i=0;do{total+=i;i++;}while(i<n);", 1)
	testConstructorOriginalFixedCapturePathRoundTrip(t, f)
}

func TestAdversarialConstructorReceiverSilentWhileLoopCaptureRoundTrip(t *testing.T) {
	f := strings.Replace(constructorSilentLoopCaptureFixture, "for(int i=0;i<n;i++){total+=i;}", "int i=0;while(i<n){total+=i++;}", 1)
	testConstructorOriginalFixedCapturePathRoundTrip(t, f)
}
