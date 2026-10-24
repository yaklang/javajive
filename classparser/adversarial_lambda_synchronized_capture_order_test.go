package javaclassparser

import "testing"

const lambdaSynchronizedCaptureFixture = `
import java.util.*;import java.util.function.*;import java.lang.reflect.*;
public class UncapturedLambdaNames {
 public static BiFunction<Object,Object,Object> factory(Object expected,Object[] box,Object key,int[] code,long stamp){return(ignored,current)->{
  if(current!=expected)return current;
  synchronized(current){
   box[0]=key;
   if(key==null||box[0]==null){code[0]=1;}
   else if(stamp==0L){code[0]=2;}
   else{code[0]=3;}
   CaptureMonitorEvents.visit(current);
   return box[0];
  }
 };}
}
class CaptureMonitorEvents {static int calls;static void visit(Object lock){if(!Thread.holdsLock(lock))throw new AssertionError("lost monitor");calls++;}}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  Method implementation=null;for(Method m:UncapturedLambdaNames.class.getDeclaredMethods())if(m.isSynthetic()){if(implementation!=null||m.getModifiers()!=4106||!Arrays.equals(m.getParameterTypes(),new Class<?>[]{Object.class,Object[].class,Object.class,int[].class,long.class,Object.class,Object.class}))throw new AssertionError("physical capture identity/order");implementation=m;}if(implementation==null)throw new AssertionError("missing original implementation");implementation.setAccessible(true);
  int rows=0;Object lock=new Object(),other=new Object(),keyValue=new String("identity"),sentinel=new Object();
  for(int path=0;path<2;path++)for(Object expected:new Object[]{null,lock,other})for(Object current:new Object[]{null,lock,other})for(Object key:new Object[]{null,keyValue})for(int boxKind=0;boxKind<3;boxKind++)for(int codeKind=0;codeKind<3;codeKind++)for(long stamp:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){
   Object[] box=boxKind==0?null:boxKind==1?new Object[0]:new Object[]{sentinel};int[] code=codeKind==0?null:codeKind==1?new int[0]:new int[]{-7};CaptureMonitorEvents.calls=0;
   BiFunction<Object,Object,Object> op=UncapturedLambdaNames.factory(expected,box,key,code,stamp);if(CaptureMonitorEvents.calls!=0||boxKind==2&&box[0]!=sentinel||codeKind==2&&code[0]!=-7)throw new AssertionError("eager lambda effects");
   Object result=null;Throwable caught=null;try{result=path==0?op.apply(sentinel,current):implementation.invoke(null,expected,box,key,code,stamp,sentinel,current);}catch(Throwable error){caught=error instanceof InvocationTargetException?((InvocationTargetException)error).getCause():error;}
   boolean enters=current==expected;boolean boxWritten=enters&&current!=null&&boxKind==2;boolean codeWritten=boxWritten&&codeKind==2;Class<?> failure=!enters?null:current==null||boxKind==0?NullPointerException.class:boxKind==1?ArrayIndexOutOfBoundsException.class:codeKind==0?NullPointerException.class:codeKind==1?ArrayIndexOutOfBoundsException.class:null;
   Object wanted=enters?key:current;int branch=key==null?1:stamp==0?2:3;
   if((caught==null)!=(failure==null)||caught!=null&&caught.getClass()!=failure||caught==null&&result!=wanted||CaptureMonitorEvents.calls!=(codeWritten?1:0)||boxKind==2&&box[0]!=(boxWritten?key:sentinel)||codeKind==2&&code[0]!=(codeWritten?branch:-7))throw new AssertionError("capture/monitor/array/failure frontier "+rows,caught);
   if(current!=null&&Thread.holdsLock(current))throw new AssertionError("monitor leaked");rows++;
  }
  if(rows!=972)throw new AssertionError("coverage:"+rows);
  List<String> declarations=new ArrayList<>();for(Method m:UncapturedLambdaNames.class.getDeclaredMethods()){StringBuilder row=new StringBuilder(m.getName()).append(':').append(m.getReturnType().getName()).append(':').append(m.getModifiers());for(Parameter p:m.getParameters())row.append('/').append(p.getType().getName()).append(':').append(p.getName()).append(':').append(p.getModifiers()).append(':').append(p.isNamePresent());declarations.add(row.toString());}Collections.sort(declarations);java.security.MessageDigest digest=java.security.MessageDigest.getInstance("SHA-256");for(String row:declarations)digest.update((row+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(digest.digest()));
 }
}`

func TestAdversarialLambdaSynchronizedBranchesKeepPhysicalCaptureDiscovery(t *testing.T) {
	for _, named := range []bool{false, true} {
		label := "absent"
		if named {
			label = "named"
		}
		t.Run(label, func(t *testing.T) {
			testUncapturedLambdaParameterMetadataRoundTrip(t, lambdaSynchronizedCaptureFixture, 1, "972", named)
		})
	}
}
