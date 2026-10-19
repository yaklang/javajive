package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialInheritedSamDeclaredReturnKeepsEagerCaptureAndCheckedFailures(t *testing.T) {
	source := strings.Replace(declaredCheckedSamTargetFixture, "public static Callable<String> wrap", "public static Resumable<String> wrap", 1)
	source = strings.Replace(source, "class LambdaParameterDriver", "interface Resumable<T> extends Callable<T>{}\nclass LambdaParameterDriver", 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "37")
}

func TestAdversarialDeepInheritedSamDeclaredReturnKeepsEagerCaptureAndCheckedFailures(t *testing.T) {
	source := strings.Replace(declaredCheckedSamTargetFixture, "public static Callable<String> wrap", "public static Resumable<String> wrap", 1)
	source = strings.Replace(source, "class LambdaParameterDriver", "interface OriginalResult<R> extends Callable<R>{}\ninterface IntermediateResult<X> extends OriginalResult<X>{}\ninterface Resumable<T> extends IntermediateResult<T>{}\nclass LambdaParameterDriver", 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "37")
}

func TestAdversarialDiamondInheritedSamKeepsOneOriginalGenericDeclaration(t *testing.T) {
	source := strings.Replace(declaredCheckedSamTargetFixture, "public static Callable<String> wrap", "public static Resumable<String> wrap", 1)
	source = strings.Replace(source, "class LambdaParameterDriver", "interface LeftResult<L> extends Callable<L>{}\ninterface RightResult<R> extends Callable<R>{}\ninterface Resumable<T> extends LeftResult<T>,RightResult<T>{}\nclass LambdaParameterDriver", 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "37")
}

const inheritedReorderedCheckedSamFixture = `
import java.util.*;
interface OriginalPair<A,B,R>{R call(A first,B second)throws Exception;}
interface FlippedPair<A,B> extends OriginalPair<B,A,A>{}
public class UncapturedLambdaNames {
 public static FlippedPair<String,Integer> wrap(OriginalPair<Integer,String,String> source){return(Integer number,String value)->source.call(number,value);}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(final String value:new String[]{null,"","a","α","\u0000","🌍"})for(final boolean fail:new boolean[]{false,true}){
   final int[]calls={0};final Integer[] expectedNumber={null};final java.io.IOException expected=new java.io.IOException("same checked failure");
   OriginalPair<Integer,String,String> source=new OriginalPair<Integer,String,String>(){public String call(Integer number,String input)throws Exception{if(number!=expectedNumber[0]||input!=value)throw new AssertionError("SAM argument identity/order");calls[0]++;if(fail)throw expected;return value;}};
   FlippedPair<String,Integer> target=UncapturedLambdaNames.wrap(source);if(target==null||calls[0]!=0)throw new AssertionError("eager SAM evaluation");
   int i=0;for(int number:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE}){expectedNumber[0]=Integer.valueOf(number);Object actual=null;Throwable caught=null;try{actual=target.call(expectedNumber[0],value);}catch(Throwable error){caught=error;}
    if(calls[0]!=++i||(fail?caught!=expected:caught!=null||actual!=value))throw new AssertionError("deferred count/checked identity/generic result",caught);rows++;
   }
  }
  FlippedPair<String,Integer> nil=UncapturedLambdaNames.wrap(null);if(nil==null)throw new AssertionError("null source evaluated eagerly");try{nil.call(7,null);throw new AssertionError("missing deferred null failure");}catch(NullPointerException expected){rows++;}
  List<String> declarations=new ArrayList<>();for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialReorderedInheritedSamKeepsInputAndResultVariableIdentity(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, inheritedReorderedCheckedSamFixture, 1, "37")
}
