package javaclassparser

import (
	"strings"
	"testing"
)

// A captured checked supplier is not evaluated by the factory. Its declared
// generic SAM result must survive independently of the eager capture's erased
// descriptor and the private implementation's physical return type.
const declaredCheckedSamTargetFixture = `
import java.util.*;import java.util.concurrent.*;
public class UncapturedLambdaNames {
 public static Callable<String> wrap(Callable<String> source){return()->source.call();}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(final String value:new String[]{null,"","a","α","\u0000","🌍"})for(final boolean fail:new boolean[]{false,true}){
   final int[] calls={0};final java.io.IOException expected=new java.io.IOException("same checked failure");
   Callable<String> source=new Callable<String>(){public String call()throws Exception{calls[0]++;if(fail)throw expected;return value;}};
   Callable<String> target=UncapturedLambdaNames.wrap(source);if(target==null||calls[0]!=0)throw new AssertionError("factory evaluated captured supplier");
   for(int i=0;i<3;i++){Object actual=null;Throwable caught=null;try{actual=target.call();}catch(Throwable error){caught=error;}
    if(calls[0]!=i+1||(fail?caught!=expected:caught!=null||actual!=value))throw new AssertionError("deferred count, checked identity or generic return identity",caught);rows++;
   }
  }
  Callable<String> nil=UncapturedLambdaNames.wrap(null);if(nil==null)throw new AssertionError("null operand was not captured");try{nil.call();throw new AssertionError("missing deferred null failure");}catch(NullPointerException expected){rows++;}
  List<String> declarations=new ArrayList<>();for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialGenericSamDeclaredReturnKeepsEagerCaptureAndCheckedFailures(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, declaredCheckedSamTargetFixture, 1, "37")
}

func TestAdversarialErasedSamReturnControlKeepsEagerCaptureAndCheckedFailures(t *testing.T) {
	source := strings.ReplaceAll(declaredCheckedSamTargetFixture, "Callable<String>", "Callable<Object>")
	source = strings.ReplaceAll(source, "public String call()", "public Object call()")
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "37")
}
