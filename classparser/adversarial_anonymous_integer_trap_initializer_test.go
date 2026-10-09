package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousIntegerTrapInitializerFixture = `interface TrapRead {long[] get();}
class TrapState {
 static String trace="";static final RuntimeException fault=new RuntimeException("original");
 static int left(int n){trace+="L";if(n==77)throw fault;return n;}
 static int right(int n){trace+="R";if(n==77)throw fault;return n;}
 static int afterInt(int n){trace+="M";return n;}
 static long leftWord(long n){trace+="X";if(n==77)throw fault;return n;}
 static long rightWord(long n){trace+="Y";if(n==77)throw fault;return n;}
 static long afterWord(long n){trace+="Z";return n;}
 static int done(){trace+="D";return 7;}
}
class TrapInitializerOwner {
 TrapRead make(final int numerator,final int divisor,final long word,final long denominator){
  return new TrapRead(){
   final int quotient=TrapState.left(numerator)/TrapState.right(divisor);
   final int remainder=TrapState.afterInt(numerator)%divisor;
   final long wideQuotient=TrapState.leftWord(word)/TrapState.rightWord(denominator);
   final long wideRemainder=TrapState.afterWord(word)%denominator;
   final int complete=TrapState.done();
   public long[] get(){return new long[]{quotient,remainder,wideQuotient,wideRemainder,complete};}
  };
 }
}
class TrapInitializerDriver {public static void main(String[] args){
 int[] ints={0,1,-1,Integer.MIN_VALUE,Integer.MAX_VALUE,77};long[] longs={0,1,-1,Long.MIN_VALUE,Long.MAX_VALUE,77};int rows=0;
 for(int a:ints)for(int b:ints)for(long p:longs)for(long q:longs){
  TrapState.trace="";boolean fault=a==77||b==77||b!=0&&p==77||b!=0&&q==77;
  String expected=a==77?"L":b==77||b==0?"LR":p==77?"LRMX":q==77||q==0?"LRMXY":"LRMXYZD";
  try{
   TrapRead value=new TrapInitializerOwner().make(a,b,p,q);long[] got=value.get();
   if(fault||b==0||q==0||!value.getClass().isAnonymousClass()||got.length!=5||got[0]!=a/b||got[1]!=a%b||got[2]!=p/q||got[3]!=p%q||got[4]!=7)throw new AssertionError("integer trap result/width/original ownership");
  }catch(RuntimeException e){
   if(fault?e!=TrapState.fault:!(e instanceof ArithmeticException)||b!=0&&q!=0)throw new AssertionError("integer trap abrupt identity",e);
  }
  if(!TrapState.trace.equals(expected))throw new AssertionError("integer trap evaluation order "+TrapState.trace+" != "+expected);rows++;
 }
 System.out.println(rows+":initializer:integer:traps");
}}`

func TestAdversarialAnonymousIntegerTrapInitializerPreservesOriginalOrder(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			source, owner, driver := nativeAnonymousIntegerTrapInitializerFixture, "TrapInitializerOwner", "TrapInitializerDriver"
			if rename == "renamed" {
				r := strings.NewReplacer("TrapInitializerOwner", "ArithmeticEnvelope", "numerator", "leftInput", "divisor", "rightInput", "denominator", "rightWordInput")
				source, owner = r.Replace(source), r.Replace(owner)
			}
			testNativePrivateSetterFixture(t, source, owner, driver, "1296:initializer:integer:traps\n")
		})
	}
}

// Floating / and % follow IEEE 754, including signed zero, NaN and infinities.
// The new integer abrupt-event certificate must not manufacture a zero trap
// for either floating width or lose the original field computation.
func TestAdversarialAnonymousFloatingInitializerKeepsIeee754(t *testing.T) {
	const fixture = `interface FloatingRead {double[] get();}
class FloatingInitializerOwner {
 FloatingRead make(final float a,final float b,final double p,final double q){return new FloatingRead(){
 final float quotient=a/b;final float remainder=a%b;
 final double wideQuotient=p/q;final double wideRemainder=p%q;
 public double[] get(){return new double[]{quotient,remainder,wideQuotient,wideRemainder};}
 };}
}
class FloatingInitializerDriver {public static void main(String[]args){
 float[] words={0f,-0f,1f,Float.MIN_VALUE,Float.POSITIVE_INFINITY,Float.NEGATIVE_INFINITY,Float.NaN};
 double[] wide={0d,-0d,1d,Double.MIN_VALUE,Double.POSITIVE_INFINITY,Double.NEGATIVE_INFINITY,Double.NaN};int rows=0;
 for(float a:words)for(float b:words)for(double p:wide)for(double q:wide){
  FloatingRead read=new FloatingInitializerOwner().make(a,b,p,q);double[] got=read.get();
  if(!read.getClass().isAnonymousClass()||got.length!=4||Float.floatToIntBits((float)got[0])!=Float.floatToIntBits(a/b)||Float.floatToIntBits((float)got[1])!=Float.floatToIntBits(a%b)||Double.doubleToLongBits(got[2])!=Double.doubleToLongBits(p/q)||Double.doubleToLongBits(got[3])!=Double.doubleToLongBits(p%q))throw new AssertionError("floating initializer width/sign/NaN/infinity");rows++;
 }System.out.println(rows+":initializer:floating:bits");
}}`
	testNativePrivateSetterFixture(t, fixture, "FloatingInitializerOwner", "FloatingInitializerDriver", "2401:initializer:floating:bits\n")
}
