package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialAnonymousHiddenFieldLambdaKeepsNamedCaptureMetadata(t *testing.T) {
	testArtifactLambdaMetadata(t, anonymousLambdaScopeFixture, []bool{false, true}, "15:anonymous-class:outer-argument:inner-lambda:nested-order:overflow\n")
}

func TestAdversarialAnonymousMixedFieldAndFormalLambdaKeepsPhysicalPortOrder(t *testing.T) {
	for _, expression := range []string{"delta*7L+input", "input+delta*7L"} {
		t.Run(expression, func(t *testing.T) {
			fixture := strings.ReplaceAll(anonymousLambdaScopeFixture, "class Parent implements Runnable {", "class Parent {public abstract void accept(long input);")
			fixture = strings.ReplaceAll(fixture, "public void run(){LongUnaryOperator step=x->{trace+=\"A\";return x*31L+delta;}", "public void accept(final long input){LongUnaryOperator step=x->{trace+=\"A\";return x*31L+"+expression+";}")
			fixture = strings.ReplaceAll(fixture, "p.run();p.run();", "p.accept(seed);p.accept(seed);")
			fixture = strings.ReplaceAll(fixture, "long state=step(step(add(seed,delta),delta),delta);", "long capture=add(java.math.BigInteger.valueOf(delta).multiply(java.math.BigInteger.valueOf(7L)).longValue(),seed);long state=step(step(add(seed,delta),capture),capture);")
			testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "15:anonymous-class:outer-argument:inner-lambda:nested-order:overflow\n")
		})
	}
}

// Reference identity and IEEE arithmetic exercise physical capture ports with
// different widths and source types. The oracle uses exact decimal products
// of the binary operands, plus explicit IEEE zero/infinity/NaN rules.
const anonymousTypedFieldLambdaFixture = `import java.util.function.*;
public class ArtifactLambdaOwner {
 public static String trace="";public double state;
 public static abstract class Parent {public abstract Object accept(double input);}
 public Parent make(final Object token,final double factor){return new Parent(){
  public Object accept(final double input){
   Supplier<Object> reference=()->{trace+="R";return token;};
   DoubleUnaryOperator scale=x->{trace+="D";return x*factor;};
   state=scale.applyAsDouble(input);return reference.get();
  }
 };}
}
class ArtifactLambdaDriver {
 static double product(double a,double b){
  if(Double.isNaN(a)||Double.isNaN(b))return Double.NaN;
  long sign=(Double.doubleToRawLongBits(a)^Double.doubleToRawLongBits(b))&Long.MIN_VALUE;
  if(Double.isInfinite(a)||Double.isInfinite(b)){
   if(a==0.0||b==0.0)return Double.NaN;
   return Double.longBitsToDouble(sign|0x7ff0000000000000L);
  }
  if(a==0.0||b==0.0)return Double.longBitsToDouble(sign);
  return new java.math.BigDecimal(a).multiply(new java.math.BigDecimal(b)).doubleValue();
 }
 public static void main(String[]args){int rows=0;
  for(Object token:new Object[]{null,new Object()})
   for(double factor:new double[]{0.0,-0.0,1.0,-2.0,Double.MIN_VALUE,Double.MAX_VALUE,Double.POSITIVE_INFINITY,Double.NaN})
    for(double input:new double[]{0.0,-0.0,1.0,-2.0,Double.MIN_VALUE,Double.MAX_VALUE,Double.POSITIVE_INFINITY,Double.NEGATIVE_INFINITY,Double.NaN}){
     ArtifactLambdaOwner owner=new ArtifactLambdaOwner();ArtifactLambdaOwner.trace="";
     ArtifactLambdaOwner.Parent value=owner.make(token,factor);
     if(value.accept(input)!=token||Double.doubleToLongBits(owner.state)!=Double.doubleToLongBits(product(input,factor)))throw new AssertionError("first reference/IEEE capture");
     if(value.accept(-input)!=token||Double.doubleToLongBits(owner.state)!=Double.doubleToLongBits(product(-input,factor))||!ArtifactLambdaOwner.trace.equals("DRDR"))throw new AssertionError("second reference/IEEE capture/order");
     rows++;
    }
  System.out.println(rows+":anonymous:double:reference:identity:ieee");
 }
}`

func TestAdversarialAnonymousTypedFieldLambdaKeepsReferenceIdentityAndIEEEValues(t *testing.T) {
	testArtifactLambdaMetadata(t, anonymousTypedFieldLambdaFixture, []bool{false, true}, "144:anonymous:double:reference:identity:ieee\n")
}
