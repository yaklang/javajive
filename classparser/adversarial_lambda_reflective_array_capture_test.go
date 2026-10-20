package javaclassparser

import "testing"

// A real varargs array follows both the captured reflective receiver and the
// effectful target getter. Retaining the array must preserve physical capture
// discovery without moving the getter, array creation or invocation failures.
func TestAdversarialReflectiveLambdaArrayKeepsCaptureAndFailureFrontiers(t *testing.T) {
	const source = `import java.lang.reflect.*;import java.util.*;import java.util.concurrent.*;
public class UncapturedLambdaNames {
 final ReflectiveCaptureTarget receiver;final boolean fail;
 public UncapturedLambdaNames(ReflectiveCaptureTarget receiver,boolean fail){this.receiver=receiver;this.fail=fail;}
 Object target(){ReflectiveCaptureTarget.trace+="G";if(fail)throw ReflectiveCaptureTarget.GETTER;return receiver;}
 public Callable<Object> factory(Method method,String value){return ()->method.invoke(target(),new Object[]{value});}
}
class ReflectiveCaptureTarget {
 static String trace="";static final RuntimeException GETTER=new RuntimeException("original getter payload"),INVOKED=new RuntimeException("original reflected payload");
 public String echo(String value){trace+="I";if("throw".equals(value))throw INVOKED;return String.valueOf(value)+"!";}
 public static String staticEcho(String value){trace+="I";if("throw".equals(value))throw INVOKED;return "static:"+String.valueOf(value)+"!";}
 public int integer(int value){trace+="I";return value+1;}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{Method implementation=null;int methods=0;for(Method m:UncapturedLambdaNames.class.getDeclaredMethods())if(m.isSynthetic()){implementation=m;methods++;}if(methods!=1||implementation==null||!Arrays.equals(implementation.getParameterTypes(),new Class<?>[]{Method.class,String.class}))throw new AssertionError("original implementation inventory");implementation.setAccessible(true);int rows=0;
 for(int kind=0;kind<4;kind++)for(String value:new String[]{null,"","value","throw"})for(boolean fail:new boolean[]{false,true})for(boolean absent:new boolean[]{false,true})for(int path=0;path<2;path++){
 Method method=kind==0?null:kind==1?ReflectiveCaptureTarget.class.getMethod("echo",String.class):kind==2?ReflectiveCaptureTarget.class.getMethod("staticEcho",String.class):ReflectiveCaptureTarget.class.getMethod("integer",int.class);UncapturedLambdaNames owner=new UncapturedLambdaNames(absent?null:new ReflectiveCaptureTarget(),fail);ReflectiveCaptureTarget.trace="";Callable<Object> call=owner.factory(method,value);if(!ReflectiveCaptureTarget.trace.equals(""))throw new AssertionError("eager effects");Object got=null;Throwable caught=null;
 try{got=path==0?call.call():implementation.invoke(owner,method,value);}catch(Throwable error){caught=path==1&&error instanceof InvocationTargetException?((InvocationTargetException)error).getCause():error;}
 boolean entered=!fail&&method!=null&&(kind==2||!absent)&&kind!=3;String trace="G"+(entered?"I":"");
 if(!ReflectiveCaptureTarget.trace.equals(trace))throw new AssertionError("original getter/reflective order "+rows+":"+ReflectiveCaptureTarget.trace+" expected "+trace,caught);
 if(fail){if(caught!=ReflectiveCaptureTarget.GETTER)throw new AssertionError("getter identity",caught);}
 else if(kind==0||absent&&kind!=2){if(!(caught instanceof NullPointerException))throw new AssertionError("receiver null boundary",caught);}
 else if(kind==3){if(!(caught instanceof IllegalArgumentException))throw new AssertionError("reflective conversion boundary",caught);}
 else if("throw".equals(value)){if(!(caught instanceof InvocationTargetException)||caught.getCause()!=ReflectiveCaptureTarget.INVOKED)throw new AssertionError("reflected payload identity",caught);}
 else {String expected=(kind==2?"static:":"")+String.valueOf(value)+"!";if(caught!=null||!Objects.equals(got,expected))throw new AssertionError("return value "+rows,caught);}rows++;
 }
 if(rows!=128)throw new AssertionError("coverage:"+rows);List<String> declarations=new ArrayList<>();for(Method m:UncapturedLambdaNames.class.getDeclaredMethods()){StringBuilder row=new StringBuilder(m.getName()).append(':').append(m.getReturnType().getName()).append(':').append(m.getModifiers());for(Parameter p:m.getParameters())row.append('/').append(p.getType().getName()).append(':').append(p.getName()).append(':').append(p.getModifiers()).append(':').append(p.isNamePresent());declarations.add(row.toString());}Collections.sort(declarations);java.security.MessageDigest digest=java.security.MessageDigest.getInstance("SHA-256");for(String row:declarations)digest.update((row+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(digest.digest()));
 }
}
`
	for _, named := range []bool{false, true} {
		label := "absent"
		if named {
			label = "named"
		}
		t.Run(label, func(t *testing.T) { testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "128", named) })
	}
}
