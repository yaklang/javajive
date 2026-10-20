package javaclassparser

import "testing"

const lambdaEarlierArgumentFixture = `
import java.util.*;import java.util.function.*;import java.lang.reflect.*;
public class UncapturedLambdaNames {
 public static IntFunction<String> factory(long count,String prefix,String key,String late){return sample->{
  long delta=count-(long)sample;
  String resolved=prefix==null?null:prefix+delta;
  return NestedCaptureEffects.compute(key,(old,marker)->NestedCaptureEffects.combine(resolved,late,old,marker));
 };}
}
class NestedCaptureEffects {
 static String trace="";static final RuntimeException failure=new RuntimeException("original nested compute payload");
 static String compute(String key,BiFunction<String,String,String> operation){trace+="K";if("throw".equals(key))throw failure;return operation.apply(key,"tail");}
 static String combine(String resolved,String late,String old,String marker){trace+="M";return resolved+late+old+marker;}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  Method outer=null;int implementations=0;for(Method m:UncapturedLambdaNames.class.getDeclaredMethods())if(m.isSynthetic()){implementations++;if(m.getModifiers()!=4106)throw new AssertionError("implementation flags");if(Arrays.equals(m.getParameterTypes(),new Class<?>[]{long.class,String.class,String.class,String.class,int.class})){if(outer!=null)throw new AssertionError("duplicate outer implementation");outer=m;}}if(implementations!=2||outer==null)throw new AssertionError("complete original implementation family");outer.setAccessible(true);int rows=0;
  for(int path=0;path<2;path++)for(long count:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(String prefix:new String[]{null,"","prefix","P"})for(String key:new String[]{null,"","key","throw"})for(String late:new String[]{null,"","late","L"})for(int sample:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   NestedCaptureEffects.trace="";IntFunction<String> op=UncapturedLambdaNames.factory(count,prefix,key,late);if(!NestedCaptureEffects.trace.equals(""))throw new AssertionError("eager nested effects");Object got=null;Throwable caught=null;try{got=path==0?op.apply(sample):outer.invoke(null,count,prefix,key,late,sample);}catch(Throwable error){caught=error instanceof InvocationTargetException?((InvocationTargetException)error).getCause():error;}
   boolean fail="throw".equals(key);long delta=count-(long)sample;String resolved=prefix==null?"null":prefix+Long.toString(delta);String expected=resolved+String.valueOf(late)+String.valueOf(key)+"tail";
   if(fail?caught!=NestedCaptureEffects.failure||!NestedCaptureEffects.trace.equals("K"):caught!=null||!Objects.equals(got,expected)||!NestedCaptureEffects.trace.equals("KM"))throw new AssertionError("nested argument/capture/effect order "+rows,caught);rows++;
  }
  if(rows!=3200)throw new AssertionError("coverage:"+rows);
  List<String> declarations=new ArrayList<>();for(Method m:UncapturedLambdaNames.class.getDeclaredMethods()){StringBuilder row=new StringBuilder(m.getName()).append(':').append(m.getReturnType().getName()).append(':').append(m.getModifiers());for(Parameter p:m.getParameters())row.append('/').append(p.getType().getName()).append(':').append(p.getName()).append(':').append(p.getModifiers()).append(':').append(p.isNamePresent());declarations.add(row.toString());}Collections.sort(declarations);java.security.MessageDigest digest=java.security.MessageDigest.getInstance("SHA-256");for(String row:declarations)digest.update((row+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(digest.digest()));
 }
}`

func TestAdversarialNestedLambdaKeepsEarlierInvocationCaptureDiscovery(t *testing.T) {
	for _, named := range []bool{false, true} {
		label := "absent"
		if named {
			label = "named"
		}
		t.Run(label, func(t *testing.T) {
			testUncapturedLambdaParameterMetadataRoundTrip(t, lambdaEarlierArgumentFixture, 2, "3200", named)
		})
	}
}
