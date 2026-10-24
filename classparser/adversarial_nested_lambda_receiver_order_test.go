package javaclassparser

import (
	"strings"
	"testing"
)

const nestedLambdaReceiverOrderFixture = `import java.util.function.LongUnaryOperator;
public class ArtifactLambdaOwner {
 public static String trace="";
 public static LongUnaryOperator make(){return value->java.util.stream.LongStream.of(value)
  .map(a->{trace+="A";return a*31L;}).map(b->{trace+="B";return b+7L;}).sum();}
}
class ArtifactLambdaDriver {
 static long expected(long value,int stage){java.math.BigInteger x=java.math.BigInteger.valueOf(value);if(stage!=1)x=x.multiply(java.math.BigInteger.valueOf(31L));if(stage!=0)x=x.add(java.math.BigInteger.valueOf(7L));return x.longValue();}
 public static void main(String[]args)throws Exception {
  int rows=0;
  for(long value:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE}){
   ArtifactLambdaOwner.trace="";LongUnaryOperator operator=ArtifactLambdaOwner.make();
   if(!ArtifactLambdaOwner.trace.equals(""))throw new AssertionError("eager nested body");
   if(operator.applyAsLong(value)!=expected(value,2)||!ArtifactLambdaOwner.trace.equals("AB"))throw new AssertionError("receiver/argument execution order or overflow");rows++;
   for(int stage=0;stage<3;stage++){
    java.lang.reflect.Method found=null;
    for(java.lang.reflect.Method m:ArtifactLambdaOwner.class.getDeclaredMethods())
     if(m.isSynthetic()&&java.lang.reflect.Modifier.isPrivate(m.getModifiers())&&java.lang.reflect.Modifier.isStatic(m.getModifiers())&&m.getName().startsWith("lambda$")&&m.getName().endsWith("$"+stage)&&m.getReturnType()==long.class&&java.util.Arrays.equals(m.getParameterTypes(),new Class<?>[]{long.class})){if(found!=null)throw new AssertionError("ambiguous original implementation identity");found=m;}
    if(found==null)throw new AssertionError("original private implementation missing");found.setAccessible(true);ArtifactLambdaOwner.trace="";
    long actual=((Long)found.invoke(null,Long.valueOf(value))).longValue();String trace=stage==0?"A":stage==1?"B":"AB";
    if(actual!=expected(value,stage)||!ArtifactLambdaOwner.trace.equals(trace))throw new AssertionError("private helper name/body binding "+found.getName()+":"+ArtifactLambdaOwner.trace);rows++;
   }
  }
  System.out.println(rows+":nested:receiver:argument:private-identity:overflow");
 }
}`

func TestAdversarialNestedLambdaReceiverAndArgumentOrder(t *testing.T) {
	for _, shape := range []string{"stream chain", "functional composition"} {
		t.Run(shape, func(t *testing.T) {
			fixture := nestedLambdaReceiverOrderFixture
			if shape == "functional composition" {
				fixture = strings.Replace(fixture, "java.util.stream.LongStream.of(value)\n  .map(a->{trace+=\"A\";return a*31L;}).map(b->{trace+=\"B\";return b+7L;}).sum()", "((LongUnaryOperator)(a->{trace+=\"A\";return a*31L;})).andThen(b->{trace+=\"B\";return b+7L;}).applyAsLong(value)", 1)
				if fixture == nestedLambdaReceiverOrderFixture {
					t.Fatal("independent functional-composition shape was not constructed")
				}
			}
			testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "20:nested:receiver:argument:private-identity:overflow\n")
		})
	}
}
