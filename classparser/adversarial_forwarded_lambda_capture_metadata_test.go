package javaclassparser

import (
	"strings"
	"testing"
)

const forwardedLambdaCaptureMetadataFixture = `
import java.util.*;import java.util.concurrent.*;import java.util.function.*;
class ForwardedCaptureTrace {
 static int calls,token,step;static long salt;
 static void record(int input,long bits,int delta){calls++;token=input;salt=bits;step=delta;}
}
public class UncapturedLambdaNames {
 public static IntFunction<Callable<String>> wrap(final Callable<String> source,final long salt,final int step){
  return(int token)->()->{ForwardedCaptureTrace.record(token,salt,step);return source.call();};
 }
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;final long salt=Long.MIN_VALUE+0x123456789L;final int step=Integer.MIN_VALUE;
  for(final String value:new String[]{null,"","a","α","\u0000","🌍"})for(final boolean fail:new boolean[]{false,true}){
   ForwardedCaptureTrace.calls=0;final int[] calls={0};final java.io.IOException expected=new java.io.IOException("same checked failure");
   Callable<String> source=new Callable<String>(){public String call()throws Exception{calls[0]++;if(fail)throw expected;return value;}};
   IntFunction<Callable<String>> factory=UncapturedLambdaNames.wrap(source,salt,step);if(factory==null||calls[0]!=0||ForwardedCaptureTrace.calls!=0)throw new AssertionError("outer factory evaluated inner body");
   int i=0;for(int token:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE}){
    Callable<String> target=factory.apply(token);if(target==null||calls[0]!=i||ForwardedCaptureTrace.calls!=i)throw new AssertionError("inner factory evaluated body");
    Object actual=null;Throwable caught=null;try{actual=target.call();}catch(Throwable error){caught=error;}
    if(calls[0]!=++i||ForwardedCaptureTrace.calls!=i||ForwardedCaptureTrace.token!=token||ForwardedCaptureTrace.salt!=salt||ForwardedCaptureTrace.step!=step||(fail?caught!=expected:caught!=null||actual!=value))throw new AssertionError("forwarded physical ports, effects or identity",caught);rows++;
   }
  }
  ForwardedCaptureTrace.calls=0;Callable<String> nil=UncapturedLambdaNames.wrap(null,salt,step).apply(-17);if(nil==null||ForwardedCaptureTrace.calls!=0)throw new AssertionError("null source evaluated eagerly");try{nil.call();throw new AssertionError("missing deferred null failure");}catch(NullPointerException expected){if(ForwardedCaptureTrace.calls!=1||ForwardedCaptureTrace.token!=-17||ForwardedCaptureTrace.salt!=salt||ForwardedCaptureTrace.step!=step)throw new AssertionError("null failure moved before original effects");rows++;}
  List<String> declarations=new ArrayList<>();for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialNestedLambdaMetadataPreservesForwardedPhysicalCaptureOrigins(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, forwardedLambdaCaptureMetadataFixture, 2, "37")
}

func TestAdversarialThreeLevelLambdaMetadataComposesOriginalWideCapturePorts(t *testing.T) {
	source := forwardedLambdaCaptureMetadataFixture
	for _, change := range [][2]string{
		{"public static IntFunction<Callable<String>> wrap", "public static IntFunction<IntFunction<Callable<String>>> wrap"},
		{"return(int token)->()->", "return(int outer)->(int token)->()->"},
		{"ForwardedCaptureTrace.record(token,salt,step)", "ForwardedCaptureTrace.record(token^outer,salt,step)"},
		{"UncapturedLambdaNames.wrap(source,salt,step)", "UncapturedLambdaNames.wrap(source,salt,step).apply(0x1f3)"},
		{"UncapturedLambdaNames.wrap(null,salt,step).apply(-17)", "UncapturedLambdaNames.wrap(null,salt,step).apply(0x1f3).apply(-17)"},
		{"ForwardedCaptureTrace.token!=token", "ForwardedCaptureTrace.token!=(token^0x1f3)"},
		{"ForwardedCaptureTrace.token!=-17", "ForwardedCaptureTrace.token!=(-17^0x1f3)"},
	} {
		if !strings.Contains(source, change[0]) {
			t.Fatal("missing independent fixture variation", change[0])
		}
		source = strings.Replace(source, change[0], change[1], 1)
	}
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 3, "37")
}

func TestAdversarialForwardedSameTypedCapturesKeepDistinctOriginsAndCallOrder(t *testing.T) {
	source := forwardedLambdaCaptureMetadataFixture
	for _, change := range [][2]string{
		{"static int calls,token,step;", "static int calls,token,step,phase;"},
		{"calls++;token=input;", "phase=1;calls++;token=input;"},
		{"wrap(final Callable<String> source,final long salt", "wrap(final Callable<String> source,final Callable<String> other,final long salt"},
		{"return source.call();", "other.call();return source.call();"},
		{"ForwardedCaptureTrace.calls=0;final int[] calls={0};", "ForwardedCaptureTrace.calls=0;ForwardedCaptureTrace.phase=0;final int[] calls={0};final int[] extraCalls={0};"},
		{"public String call()throws Exception{calls[0]++;", "public String call()throws Exception{if(ForwardedCaptureTrace.phase!=2)throw new AssertionError(\"source call order\");ForwardedCaptureTrace.phase=3;calls[0]++;"},
		{"IntFunction<Callable<String>> factory=", "Callable<String> other=new Callable<String>(){public String call(){if(ForwardedCaptureTrace.phase!=1)throw new AssertionError(\"other call order\");ForwardedCaptureTrace.phase=2;extraCalls[0]++;return \"other\";}};IntFunction<Callable<String>> factory="},
		{"UncapturedLambdaNames.wrap(source,salt,step)", "UncapturedLambdaNames.wrap(source,other,salt,step)"},
		{"if(calls[0]!=++i||", "if(calls[0]!=++i||extraCalls[0]!=i||ForwardedCaptureTrace.phase!=3||"},
		{"UncapturedLambdaNames.wrap(null,salt,step)", "UncapturedLambdaNames.wrap(null,new Callable<String>(){public String call(){if(ForwardedCaptureTrace.phase!=1)throw new AssertionError(\"null call order\");ForwardedCaptureTrace.phase=2;return \"other\";}},salt,step)"},
		{"if(ForwardedCaptureTrace.calls!=1||", "if(ForwardedCaptureTrace.phase!=2||ForwardedCaptureTrace.calls!=1||"},
	} {
		if !strings.Contains(source, change[0]) {
			t.Fatal("missing independent fixture variation", change[0])
		}
		source = strings.Replace(source, change[0], change[1], 1)
	}
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 2, "37")
}

func TestAdversarialForwardedCaptureControlKeepsOriginalParameterTableAbsence(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, forwardedLambdaCaptureMetadataFixture, 2, "37", false)
}
