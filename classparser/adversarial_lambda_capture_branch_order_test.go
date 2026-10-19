package javaclassparser

import (
	"strings"
	"testing"
)

// Equivalent early-return layouts can discover free variables in different
// source order. Captures include a wide word and three equal-typed references;
// the unchanged caller also invokes the original physical implementation.
const lambdaCaptureBranchFixture = `
import java.util.*;import java.util.function.*;import java.lang.reflect.*;
public class UncapturedLambdaNames{
 public static IntFunction<String> factory(long count,String chosen,String prefix,String suffix){return(int sample)->{
  if((long)sample==count-1L)return chosen;
  String lower=BranchCaptureEffects.make("L",prefix,suffix,sample);if((sample&1)==0)return lower;
  String upper=BranchCaptureEffects.make("U",prefix,suffix,sample);if((sample&2)==0)return upper;return lower;
 };}
}
class BranchCaptureEffects{
 static String trace="";static String make(String tag,String prefix,String suffix,int sample){trace+=tag;return tag+":"+prefix+suffix+sample;}
}
class LambdaParameterDriver{
 public static void main(String[]args)throws Exception{
  Method implementation=null;for(Method method:UncapturedLambdaNames.class.getDeclaredMethods())if(method.isSynthetic()){if(implementation!=null||method.getModifiers()!=4106||!Arrays.equals(method.getParameterTypes(),new Class<?>[]{long.class,String.class,String.class,String.class,int.class}))throw new AssertionError("branch capture declaration order");implementation=method;}if(implementation==null)throw new AssertionError("missing implementation");implementation.setAccessible(true);
  int rows=0;for(int path=0;path<2;path++)for(long count:new long[]{Long.MIN_VALUE,0,1,2,Long.MAX_VALUE})for(String chosen:new String[]{null,"","chosen",new String("identity")})for(String prefix:new String[]{null,"","p","P"})for(String suffix:new String[]{null,":","s"})for(int sample:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   BranchCaptureEffects.trace="";IntFunction<String> operation=UncapturedLambdaNames.factory(count,chosen,prefix,suffix);if(!BranchCaptureEffects.trace.equals(""))throw new AssertionError("eager branch effects");boolean shortcut=(long)sample==count-1L;String trace=shortcut?"":(sample&1)==0?"L":"LU";String tag=(sample&1)!=0&&(sample&2)==0?"U":"L";String expected=shortcut?chosen:tag+":"+(prefix==null?"null":prefix)+(suffix==null?"null":suffix)+Integer.toString(sample);String got=path==0?operation.apply(sample):(String)implementation.invoke(null,count,chosen,prefix,suffix,sample);if(!Objects.equals(got,expected)||shortcut&&got!=chosen||!BranchCaptureEffects.trace.equals(trace))throw new AssertionError("capture/branch/identity/effects "+rows+" got="+got+" expected="+expected+" trace="+BranchCaptureEffects.trace);rows++;
  }
  List<String> declarations=new ArrayList<>();for(Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialLambdaEarlyReturnsKeepPhysicalCaptureDiscovery(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, lambdaCaptureBranchFixture, 1, "2400")
}

// Receiver evaluation precedes allocation and initialization of its later
// argument. A separate array statement must not silently move capture discovery
// ahead of that receiver, even when all captured references have the same type.
func TestAdversarialLambdaReceiverBeforeArrayKeepsPhysicalCaptureDiscovery(t *testing.T) {
	source := strings.Replace(lambdaCaptureBranchFixture,
		`String lower=BranchCaptureEffects.make("L",prefix,suffix,sample);if((sample&1)==0)return lower;
  String upper=BranchCaptureEffects.make("U",prefix,suffix,sample);if((sample&2)==0)return upper;return lower;`,
		`String lower=prefix.concat(String.format("%s.z%02d",BranchCaptureEffects.argument("L",suffix),Integer.valueOf(sample+1)));if((sample&1)==0)return lower;
  String upper=prefix.concat(String.format("%s.Z%02d",BranchCaptureEffects.argument("U",suffix),Integer.valueOf(sample+1)));if((sample&2)==0)return upper;return lower;`, 1)
	source = strings.Replace(source,
		`static String trace="";static String make(String tag,String prefix,String suffix,int sample){trace+=tag;return tag+":"+prefix+suffix+sample;}`,
		`static String trace="";static String argument(String tag,String suffix){trace+=tag;return suffix;}`, 1)
	start := strings.Index(source, `   BranchCaptureEffects.trace="";IntFunction`)
	if start < 0 {
		t.Fatal("missing independent branch oracle boundary")
	}
	endOffset := strings.Index(source[start:], `rows++;`)
	if endOffset < 0 {
		t.Fatal("missing independent branch oracle end")
	}
	end := endOffset + start + len(`rows++;`)
	source = source[:start] + `   BranchCaptureEffects.trace="";IntFunction<String> operation=UncapturedLambdaNames.factory(count,chosen,prefix,suffix);if(!BranchCaptureEffects.trace.equals(""))throw new AssertionError("eager branch effects");boolean shortcut=(long)sample==count-1L;String expectedTrace=shortcut?"":prefix==null||(sample&1)==0?"L":"LU";
   int next=sample+1;String decimal=Integer.toString(next);if(next>=0&&next<10)decimal="0"+decimal;String part=(sample&1)!=0&&(sample&2)==0?".Z":".z";String expected=shortcut?chosen:prefix+(suffix==null?"null":suffix)+part+decimal;Throwable failure=null;String got=null;
   try{got=path==0?operation.apply(sample):(String)implementation.invoke(null,count,chosen,prefix,suffix,sample);}catch(Throwable problem){failure=problem instanceof InvocationTargetException?((InvocationTargetException)problem).getCause():problem;}
   if(!BranchCaptureEffects.trace.equals(expectedTrace))throw new AssertionError("array/receiver evaluation order "+rows+":"+BranchCaptureEffects.trace);
   if(!shortcut&&prefix==null){if(failure==null||failure.getClass()!=NullPointerException.class)throw new AssertionError("null receiver after argument effects");}else if(failure!=null||!Objects.equals(got,expected)||shortcut&&got!=chosen)throw new AssertionError("array/capture/branch/identity "+rows+" got="+got+" expected="+expected,failure);rows++;` + source[end:]
	source = strings.Replace(source, `  Method implementation=null;`, `  Locale.setDefault(Locale.ROOT);Method implementation=null;`, 1)
	for _, named := range []bool{false, true} {
		name := "without-parameters"
		if named {
			name = "with-parameters"
		}
		t.Run(name, func(t *testing.T) {
			testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "2400", named)
		})
	}
}
