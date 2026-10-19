package javaclassparser

import "testing"

const constructorInstanceLambdaFixture = `
import java.util.*;import java.util.function.*;import java.lang.reflect.*;
public class UncapturedLambdaNames{
 int mode;final String tag;final RuntimeException fault;public final IntPredicate operation;
 public UncapturedLambdaNames(int mode,String tag,UncapturedLambdaNames next,RuntimeException fault){this.mode=mode;this.tag=tag;this.fault=fault;this.operation=(int sample)->this.matches(sample)&&next.matches(sample);}
 public boolean matches(int sample){LambdaParameterDriver.trace+=tag;if(mode<0)throw fault;return mode==1||mode==2&&(sample&1)==0;}
}
class LambdaParameterDriver{
 static String trace="";static final RuntimeException firstFault=new RuntimeException("receiver"),secondFault=new RuntimeException("next");
 static boolean truth(UncapturedLambdaNames node,int sample){return node.mode==1||node.mode==2&&(sample&1)==0;}
 public static void main(String[]args)throws Exception{
  Method implementation=null;for(Method method:UncapturedLambdaNames.class.getDeclaredMethods())if(method.isSynthetic()){if(implementation!=null||method.getModifiers()!=4098||!Arrays.equals(method.getParameterTypes(),new Class<?>[]{UncapturedLambdaNames.class,int.class}))throw new AssertionError("constructor receiver is not physical capture");implementation=method;}if(implementation==null)throw new AssertionError("missing implementation");implementation.setAccessible(true);
  int rows=0;for(int path=0;path<2;path++)for(int rm:new int[]{-1,0,1,2,3})for(int nm:new int[]{-2,-1,1})for(int sample:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   UncapturedLambdaNames next=nm==-2?null:new UncapturedLambdaNames(nm,"N",null,secondFault);trace="";UncapturedLambdaNames receiver=new UncapturedLambdaNames(rm,"R",next,firstFault);if(!trace.equals(""))throw new AssertionError("eager construction match");if(rm==3)receiver.mode=(sample&1)==0?0:1;
   String expected="R";Throwable expectedFault=null;boolean expectedValue=false;if(receiver.mode<0)expectedFault=firstFault;else if(truth(receiver,sample)){if(next==null)expectedFault=new NullPointerException();else{expected+="N";if(next.mode<0)expectedFault=secondFault;else expectedValue=truth(next,sample);}}
   boolean actual=false;Throwable caught=null;try{actual=path==0?receiver.operation.test(sample):((Boolean)implementation.invoke(receiver,next,sample)).booleanValue();}catch(InvocationTargetException error){caught=error.getCause();}catch(Throwable error){caught=error;}
   if(!trace.equals(expected)||(expectedFault==null?caught!=null||actual!=expectedValue:expectedFault instanceof NullPointerException?caught==null||caught.getClass()!=NullPointerException.class:caught!=expectedFault))throw new AssertionError("initialized receiver/capture/SAM "+rows+" trace="+trace,caught);rows++;
  }
  List<String> declarations=new ArrayList<>();for(Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialConstructorLambdaUsesInitializedReceiverAndSeparatePhysicalRows(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, constructorInstanceLambdaFixture, 1, "150")
}

func TestAdversarialImplicitReceiverStaticQualifierKeepsImplementationRole(t *testing.T) {
	const source = `
import java.util.*;import java.util.function.*;import java.lang.reflect.*;
public class UncapturedLambdaNames{
 public IntUnaryOperator factory(){return(int sample)->this.apply(sample);}
 public static int apply(int sample){return(sample^0x539)+(sample>>>7);}
}
class LambdaParameterDriver{
 public static void main(String[]args)throws Exception{
  Method implementation=null;for(Method method:UncapturedLambdaNames.class.getDeclaredMethods())if(method.isSynthetic()){if(implementation!=null||method.getModifiers()!=4098||!Arrays.equals(method.getParameterTypes(),new Class<?>[]{int.class}))throw new AssertionError("implicit qualifier implementation role");implementation=method;}if(implementation==null)throw new AssertionError("missing implementation");implementation.setAccessible(true);
  int rows=0;for(int path=0;path<2;path++)for(int sample:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){UncapturedLambdaNames receiver=new UncapturedLambdaNames();IntUnaryOperator operation=receiver.factory();int expected=(sample^0x539)+(sample>>>7);int actual=path==0?operation.applyAsInt(sample):((Integer)implementation.invoke(receiver,sample)).intValue();if(actual!=expected)throw new AssertionError("receiver/static qualifier "+rows);rows++;}
  List<String> declarations=new ArrayList<>();for(Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "10")
}
