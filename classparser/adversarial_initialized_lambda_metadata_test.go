package javaclassparser

import (
	"strings"
	"testing"
)

// A fresh defining loader separates successful initialization from sticky
// failures at each factory. The retained binary caller checks cause identity,
// once-only initializer effects, deferred predicate effects and all physical
// implementation declarations/parameters, including those in <clinit>.
const initializedLambdaMetadataFixture = `
import java.util.*;import java.util.function.*;
public class UncapturedLambdaNames {
 static Predicate<Object> first=StaticLambdaProbe.record(1,(Object item)->{StaticLambdaProbe.trace+="P";return item==null;});
 static Predicate<Object> second;
 static {second=StaticLambdaProbe.record(2,(Object item)->{StaticLambdaProbe.trace+="Q";return item instanceof String;});}
 public static Supplier<Object> pass(Object saved){return()->saved;}
}
class StaticLambdaProbe {
 static int failAt;static String trace="";static final RuntimeException fault=new RuntimeException("original initializer");
 static <T>T record(int stage,T value){trace+=stage;if(failAt==stage)throw fault;return value;}
}
class LambdaParameterDriver {
 static java.lang.reflect.Field field(Class<?> owner,String name)throws Exception{java.lang.reflect.Field field=owner.getDeclaredField(name);field.setAccessible(true);return field;}
 public static void main(String[]args)throws Exception{
  int rows=0;List<String> declarations=new ArrayList<>();java.net.URL root=LambdaParameterDriver.class.getProtectionDomain().getCodeSource().getLocation();
  for(int mode=0;mode<3;mode++)try(java.net.URLClassLoader loader=new java.net.URLClassLoader(new java.net.URL[]{root},null)){
   Class<?> probe=Class.forName("StaticLambdaProbe",true,loader);field(probe,"failAt").setInt(null,mode);Throwable fault=(Throwable)field(probe,"fault").get(null);
   Class<?> target=Class.forName("UncapturedLambdaNames",false,loader);
   for(java.lang.reflect.Method method:target.getDeclaredMethods()){
    StringBuilder row=new StringBuilder("mode=").append(mode).append('/').append(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
   }
   Throwable failure=null;try{Class.forName("UncapturedLambdaNames",true,loader);}catch(Throwable error){failure=error;}
   String expected=mode==1?"1":"12";if(!field(probe,"trace").get(null).equals(expected))throw new AssertionError("initializer factory evaluation order");
   if(mode!=0){if(!(failure instanceof ExceptionInInitializerError)||failure.getCause()!=fault)throw new AssertionError("initialization cause identity",failure);Throwable repeated=null;try{Class.forName("UncapturedLambdaNames",true,loader);}catch(Throwable error){repeated=error;}if(!(repeated instanceof NoClassDefFoundError)||!field(probe,"trace").get(null).equals(expected))throw new AssertionError("same defining loader must retain failed initialization",repeated);rows++;continue;}
   if(failure!=null)throw new AssertionError("successful initialization",failure);rows++;
   @SuppressWarnings("unchecked")Predicate<Object> first=(Predicate<Object>)field(target,"first").get(null);
   @SuppressWarnings("unchecked")Predicate<Object> second=(Predicate<Object>)field(target,"second").get(null);
   for(Object item:new Object[]{null,"value",Integer.valueOf(7),new Object(),String.class}){if(first.test(item)!=(item==null)||second.test(item)!=(item instanceof String))throw new AssertionError("static factory SAM/parameter binding");expected+="PQ";if(!field(probe,"trace").get(null).equals(expected))throw new AssertionError("deferred body order/once-only effects");rows++;}
   for(Object saved:new Object[]{null,new Object()}){@SuppressWarnings("unchecked")Supplier<Object> operation=(Supplier<Object>)target.getMethod("pass",Object.class).invoke(null,saved);if(operation.get()!=saved||!field(probe,"trace").get(null).equals(expected))throw new AssertionError("ordinary capture and static registration");rows++;}
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialInitializedLambdaMetadataKeepsDefiningLoaderAndRegistration(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, initializedLambdaMetadataFixture, 3, "10")
}

func TestAdversarialInitializedLambdaMetadataPreservesEarlierOrdinaryRegistration(t *testing.T) {
	const method = " public static Supplier<Object> pass(Object saved){return()->saved;}"
	source := strings.Replace(initializedLambdaMetadataFixture, method, "", 1)
	source = strings.Replace(source, "public class UncapturedLambdaNames {", "public class UncapturedLambdaNames {\n"+method, 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 3, "10")
}
