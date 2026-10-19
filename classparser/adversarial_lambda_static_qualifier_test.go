package javaclassparser

import "testing"

// The qualifier is an actual original READ/POP. It is evaluated without a
// null check and still contributes a physical lambda parameter. Both a null
// qualifier and a subclass hiding the static method must select the declared
// class target. The unchanged caller also invokes the private implementation.
func TestAdversarialStaticLambdaQualifierKeepsPhysicalCaptureAndBinding(t *testing.T) {
	const source = `
import java.util.*;import java.util.function.*;import java.lang.reflect.*;
public class UncapturedLambdaNames{
 public static IntUnaryOperator factory(StaticLambdaEngine qualifier,int captured){return(int sample)->qualifier.apply(captured,sample);}
}
class StaticLambdaEngine{static int apply(int base,int sample){return(base^sample)+(base>>>7);}}
class HiddenStaticLambdaEngine extends StaticLambdaEngine{static int apply(int base,int sample){throw new AssertionError("static dispatch became virtual/hiding");}}
class LambdaParameterDriver{
 public static void main(String[]args)throws Exception{
  Method implementation=null;for(Method method:UncapturedLambdaNames.class.getDeclaredMethods())if(method.isSynthetic()){if(implementation!=null||method.getModifiers()!=4106||!Arrays.equals(method.getParameterTypes(),new Class<?>[]{StaticLambdaEngine.class,int.class,int.class}))throw new AssertionError("discarded original qualifier lost physical capture");implementation=method;}if(implementation==null)throw new AssertionError("missing physical implementation");implementation.setAccessible(true);
  int rows=0;for(int path=0;path<2;path++)for(StaticLambdaEngine qualifier:new StaticLambdaEngine[]{null,new StaticLambdaEngine(),new HiddenStaticLambdaEngine()})for(int base:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int sample:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   IntUnaryOperator operation=UncapturedLambdaNames.factory(qualifier,base);int expected=(base^sample)+(base>>>7);int actual=path==0?operation.applyAsInt(sample):((Integer)implementation.invoke(null,qualifier,base,sample)).intValue();if(actual!=expected)throw new AssertionError("qualifier/capture/SAM/static-binding "+rows);rows++;
  }
  List<String> declarations=new ArrayList<>();for(Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "150")
}
