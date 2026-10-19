package javaclassparser

import (
	"strings"
	"testing"
)

// The ordinary reference accepts a wider implementation parameter than the
// instantiated SAM. A raw caller must still check String before any effect.
// Its enclosing, parameter-bearing lambda must not acquire an extra synthetic
// declaration merely because the decompiler chose a checked adapter lambda.
const methodReferenceParameterMetadataFixture = `
import java.util.*;import java.util.function.*;
class MethodReferenceEffects {
 static int calls;static Object last;static String trace;static boolean fail;
 static final IllegalArgumentException failure=new IllegalArgumentException("same failure");
 static void opened(int token){trace="O"+token;}
 static void consume(CharSequence value){calls++;last=value;trace+="S";if(fail)throw failure;}
}
public class UncapturedLambdaNames {
 public static IntFunction<Consumer<String>> wrap(){return(int token)->{
  MethodReferenceEffects.opened(token);
  Consumer<String> selected=UncapturedLambdaNames::consume;return selected;
 };}
 public static void consume(CharSequence value){MethodReferenceEffects.consume(value);}
 public static void consume(Object value){throw new AssertionError("wrong overload");}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(int token:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE}){
   MethodReferenceEffects.trace="";MethodReferenceEffects.calls=0;
   IntFunction<Consumer<String>> factory=UncapturedLambdaNames.wrap();
   if(factory==null||!MethodReferenceEffects.trace.equals("")||MethodReferenceEffects.calls!=0)throw new AssertionError("eager outer body");
   Consumer<String> selected=factory.apply(token);
   if(selected==null||!MethodReferenceEffects.trace.equals("O"+token)||MethodReferenceEffects.calls!=0)throw new AssertionError("method reference creation effects");
   for(String value:new String[]{null,"","a","α","\u0000","🌍"})for(boolean fail:new boolean[]{false,true}){
    MethodReferenceEffects.trace="";MethodReferenceEffects.calls=0;MethodReferenceEffects.last=new Object();MethodReferenceEffects.fail=fail;
    Throwable caught=null;try{selected.accept(value);}catch(Throwable error){caught=error;}
    if(MethodReferenceEffects.calls!=1||MethodReferenceEffects.last!=value||!MethodReferenceEffects.trace.equals("S")||(fail?caught!=MethodReferenceEffects.failure:caught!=null))throw new AssertionError("input identity/selected member/effect order/failure identity",caught);rows++;
   }
   for(Object invalid:new Object[]{new StringBuilder("a"),Integer.valueOf(7),new Object()}){
    MethodReferenceEffects.trace="";MethodReferenceEffects.calls=0;MethodReferenceEffects.fail=false;
    try{((Consumer)selected).accept(invalid);throw new AssertionError("lost original String check");}catch(ClassCastException expected){}
    if(MethodReferenceEffects.calls!=0||!MethodReferenceEffects.trace.equals(""))throw new AssertionError("implementation ran before SAM input check");rows++;
   }
  }
  List<String> declarations=new ArrayList<>();for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialMethodReferencePreservesNarrowedSamAndEnclosingLambdaMetadata(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, methodReferenceParameterMetadataFixture, 1, "45")
}

func TestAdversarialCustomMethodReferencePreservesOriginalDeclaredSamAndMetadata(t *testing.T) {
	source := strings.ReplaceAll(methodReferenceParameterMetadataFixture, "Consumer", "OriginalEntry")
	source = strings.Replace(source, "public class UncapturedLambdaNames", "interface OriginalEntry<T>{void accept(T input);}\npublic class UncapturedLambdaNames", 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "45")
}

func TestAdversarialInheritedMethodReferencePreservesOriginalDeclaredSamAndMetadata(t *testing.T) {
	source := strings.ReplaceAll(methodReferenceParameterMetadataFixture, "Consumer", "OriginalEntry")
	source = strings.Replace(source, "public class UncapturedLambdaNames", "interface OriginalParent<T>{void accept(T input);}\ninterface OriginalEntry<V> extends OriginalParent<V>{}\npublic class UncapturedLambdaNames", 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "45")
}

func TestAdversarialMethodReferenceCannotInventAbsentParameterMetadata(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, methodReferenceParameterMetadataFixture, 1, "45", false)
}
