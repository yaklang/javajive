package javaclassparser

import (
	"strings"
	"testing"
)

// THIS is an implicit implementation receiver, not a MethodParameters row.
// The unchanged caller exercises short circuiting, mutable receiver state,
// receiver/next aliasing, exact faults and direct physical helper invocation.
const instanceLambdaParameterFixture = `
import java.util.*;import java.util.function.*;import java.lang.reflect.*;
public abstract class UncapturedLambdaNames {
 public abstract boolean matches(int sample);
 public IntPredicate and(UncapturedLambdaNames next){return(int sample)->this.matches(sample)&&next.matches(sample);}
}
class InstanceNode extends UncapturedLambdaNames {
 int mode;final String tag;final RuntimeException fault;InstanceNode(int mode,String tag,RuntimeException fault){this.mode=mode;this.tag=tag;this.fault=fault;}
 public boolean matches(int sample){LambdaParameterDriver.trace+=tag;if(mode<0)throw fault;return mode==1||mode==2&&(sample&1)==0;}
}
class LambdaParameterDriver {
 static String trace="";static final RuntimeException firstFault=new RuntimeException("receiver"),secondFault=new RuntimeException("next");
 static boolean truth(InstanceNode node,int sample){return node.mode==1||node.mode==2&&(sample&1)==0;}
 public static void main(String[]args)throws Exception{
  Method implementation=null;for(Method method:UncapturedLambdaNames.class.getDeclaredMethods())if(method.isSynthetic()){if(implementation!=null||method.getModifiers()!=4098||!Arrays.equals(method.getParameterTypes(),new Class<?>[]{UncapturedLambdaNames.class,int.class}))throw new AssertionError("implicit receiver is not a physical captured parameter");implementation=method;}if(implementation==null)throw new AssertionError("missing instance implementation");implementation.setAccessible(true);
  int rows=0;for(int path=0;path<2;path++)for(int rm:new int[]{-2,-1,0,1,2})for(int nm:new int[]{-3,-2,-1,0,1,2})for(int sample:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   InstanceNode receiver=rm==-2?null:new InstanceNode(rm,"R",firstFault),next=nm==-3?receiver:nm==-2?null:new InstanceNode(nm,"N",secondFault);trace="";IntPredicate operation=null;Throwable factoryFailure=null;
   try{operation=receiver.and(next);}catch(Throwable error){factoryFailure=error;}
   if(!trace.equals("")||(receiver==null?(factoryFailure==null||factoryFailure.getClass()!=NullPointerException.class):(factoryFailure!=null||operation==null)))throw new AssertionError("deferred receiver factory "+rows,factoryFailure);
   if(receiver==null){rows++;continue;}
   if(rm==2)receiver.mode=(sample&1)==0?0:1;
   String expected="R";Throwable expectedFault=null;boolean expectedValue=false;
   if(receiver.mode<0)expectedFault=receiver.fault;
   else if(truth(receiver,sample)){
    if(next==null)expectedFault=new NullPointerException();
    else{expected+=next.tag;if(next.mode<0)expectedFault=next.fault;else expectedValue=truth(next,sample);}
   }
   boolean actual=false;Throwable caught=null;try{actual=path==0?operation.test(sample):((Boolean)implementation.invoke(receiver,next,sample)).booleanValue();}catch(InvocationTargetException error){caught=error.getCause();}catch(Throwable error){caught=error;}
   if(!trace.equals(expected)||(expectedFault==null?caught!=null||actual!=expectedValue:expectedFault instanceof NullPointerException?caught==null||caught.getClass()!=NullPointerException.class:caught!=expectedFault))throw new AssertionError("receiver/physical-capture/SAM binding "+rows+" trace="+trace+" expected="+expected,caught);rows++;
  }
  List<String> declarations=new ArrayList<>();for(Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialInstanceLambdaMetadataKeepsReceiverAndPhysicalCaptureRoles(t *testing.T) {
	for _, kind := range []string{"class", "interface"} {
		t.Run(kind, func(t *testing.T) {
			source := instanceLambdaParameterFixture
			if kind == "interface" {
				source = strings.Replace(source, "public abstract class UncapturedLambdaNames", "public interface UncapturedLambdaNames", 1)
				source = strings.Replace(source, "public IntPredicate and(", "public default IntPredicate and(", 1)
				source = strings.Replace(source, "class InstanceNode extends UncapturedLambdaNames", "class InstanceNode implements UncapturedLambdaNames", 1)
			}
			testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "300")
		})
	}
}
