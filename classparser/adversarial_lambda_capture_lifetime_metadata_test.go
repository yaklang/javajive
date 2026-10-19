package javaclassparser

import (
	"strings"
	"testing"
)

// The compiler reuses one physical slot for a captured local, a later catch
// parameter and a fallback local. Their source lifetimes are distinct. The
// unchanged binary driver checks selection/catch order, deferred execution,
// return aliases, null and checked-exception identity, independently of names.
const lambdaCaptureLifetimeMetadataFixture = `
import java.util.*;import java.util.concurrent.*;
class CaptureLifetimeEffects {
 static String trace="";static int calls;
 static final java.io.IOException selectionFailure=new java.io.IOException("selection");
 static Callable<String> select(Callable<String> source,int stage)throws java.io.IOException{trace+="Q";if(stage<0)throw selectionFailure;return source;}
 static void caught(java.io.IOException error){if(error!=selectionFailure)throw new AssertionError("selection failure identity");trace+="C";}
 static void body(){trace+="B";calls++;}
}
public class UncapturedLambdaNames {
 public static Callable<String> wrap(final Callable<String> source,final int stage){
  try{final Callable<String> saved=CaptureLifetimeEffects.select(source,stage);return()->{CaptureLifetimeEffects.body();return saved.call();};}
  catch(java.io.IOException error){CaptureLifetimeEffects.caught(error);}
  Callable<String> fallback=source;return fallback;
 }
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(final String value:new String[]{null,"","a","α","\u0000","🌍"})for(final boolean fail:new boolean[]{false,true})for(int stage:new int[]{-1,0,1}){
   CaptureLifetimeEffects.trace="";CaptureLifetimeEffects.calls=0;final int[] calls={0};final java.io.IOException expected=new java.io.IOException("same checked failure");
   Callable<String> source=new Callable<String>(){public String call()throws Exception{CaptureLifetimeEffects.trace+="S";calls[0]++;if(fail)throw expected;return value;}};
   Callable<String> target=UncapturedLambdaNames.wrap(source,stage);
   if(target==null||calls[0]!=0||CaptureLifetimeEffects.calls!=0||!CaptureLifetimeEffects.trace.equals(stage<0?"QC":"Q")||(target==source)!=(stage<0))throw new AssertionError("factory selection, alias or eager effects");
   Object actual=null;Throwable caught=null;try{actual=target.call();}catch(Throwable error){caught=error;}
   if(calls[0]!=1||CaptureLifetimeEffects.calls!=(stage<0?0:1)||!CaptureLifetimeEffects.trace.equals(stage<0?"QCS":"QBS")||(fail?caught!=expected:caught!=null||actual!=value))throw new AssertionError("lifetime, call order or result/failure identity",caught);rows++;
  }
  for(int stage:new int[]{-1,0,1}){CaptureLifetimeEffects.trace="";CaptureLifetimeEffects.calls=0;Callable<String> target=UncapturedLambdaNames.wrap(null,stage);if(CaptureLifetimeEffects.calls!=0||!CaptureLifetimeEffects.trace.equals(stage<0?"QC":"Q"))throw new AssertionError("eager null evaluation");if(stage<0){if(target!=null)throw new AssertionError("fallback null alias");}else{if(target==null)throw new AssertionError("missing deferred factory");try{target.call();throw new AssertionError("missing deferred null failure");}catch(NullPointerException expected){if(CaptureLifetimeEffects.calls!=1||!CaptureLifetimeEffects.trace.equals("QB"))throw new AssertionError("null moved before original body effect");}}rows++;}
  List<String> declarations=new ArrayList<>();for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialCapturedLocalMetadataSeparatesCatchAndFallbackSlotLifetimes(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, lambdaCaptureLifetimeMetadataFixture, 1, "39")
}

func TestAdversarialWideCapturedLocalMetadataKeepsBothWordsBeforeLaterSlotReuse(t *testing.T) {
	source := lambdaCaptureLifetimeMetadataFixture
	for _, change := range [][2]string{
		{"static String trace=\"\";static int calls;", "static String trace=\"\";static int calls;static long word;"},
		{"static Callable<String> select", "static long select"},
		{"throw selectionFailure;return source;", "throw selectionFailure;return Long.MIN_VALUE^((long)stage<<32)^0x1234ABCDEFL;"},
		{"static void body(){trace+=\"B\";calls++;}", "static void body(long bits){trace+=\"B\";calls++;word=bits;}"},
		{"final Callable<String> saved=", "final long saved="},
		{"CaptureLifetimeEffects.body();return saved.call();", "CaptureLifetimeEffects.body(saved);return source.call();"},
		{"CaptureLifetimeEffects.trace=\"\";CaptureLifetimeEffects.calls=0;", "CaptureLifetimeEffects.trace=\"\";CaptureLifetimeEffects.calls=0;CaptureLifetimeEffects.word=0;"},
		{"if(calls[0]!=1||", "long expectedWord=stage<0?0:java.math.BigInteger.valueOf(Long.MIN_VALUE).xor(java.math.BigInteger.valueOf(stage).shiftLeft(32)).xor(new java.math.BigInteger(\"1234abcdef\",16)).longValue();if(CaptureLifetimeEffects.word!=expectedWord||calls[0]!=1||"},
		{"if(CaptureLifetimeEffects.calls!=1||", "long expectedWord=java.math.BigInteger.valueOf(Long.MIN_VALUE).xor(java.math.BigInteger.valueOf(stage).shiftLeft(32)).xor(new java.math.BigInteger(\"1234abcdef\",16)).longValue();if(CaptureLifetimeEffects.word!=expectedWord||CaptureLifetimeEffects.calls!=1||"},
	} {
		if !strings.Contains(source, change[0]) {
			t.Fatal("missing independent wide-lifetime variation", change[0])
		}
		source = strings.ReplaceAll(source, change[0], change[1])
	}
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "39")
}

func TestAdversarialCapturedSlotLifetimeControlKeepsOriginalParameterTableAbsence(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, lambdaCaptureLifetimeMetadataFixture, 1, "39", false)
}

func TestAdversarialNestedForwardedAndLocalCapturesKeepDistinctLifetimeBinders(t *testing.T) {
	source := lambdaCaptureLifetimeMetadataFixture
	for _, change := range [][2]string{
		{"import java.util.*;import java.util.concurrent.*;", "import java.util.*;import java.util.concurrent.*;import java.util.function.*;"},
		{"public static Callable<String> wrap(final Callable<String> source,final int stage){", "public static IntFunction<Callable<String>> wrap(final Callable<String> source,final int ignored){return(int stage)->{"},
		{"CaptureLifetimeEffects.body();return saved.call();", "CaptureLifetimeEffects.body();if(saved!=source)throw new AssertionError(\"same-typed distinct capture origins\");return saved.call();"},
		{"Callable<String> fallback=source;return fallback;\n }", "Callable<String> fallback=source;return fallback;\n }; }"},
		{"UncapturedLambdaNames.wrap(source,stage)", "UncapturedLambdaNames.wrap(source,0).apply(stage)"},
		{"UncapturedLambdaNames.wrap(null,stage)", "UncapturedLambdaNames.wrap(null,0).apply(stage)"},
	} {
		if !strings.Contains(source, change[0]) {
			t.Fatal("missing independent nested-lifetime variation", change[0])
		}
		source = strings.ReplaceAll(source, change[0], change[1])
	}
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 2, "39")
}
