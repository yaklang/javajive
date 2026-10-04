package javaclassparser

import "testing"

// Writable locals must follow their bytecode value category and consumer,
// while synchronized blocks retain the exact receiver and release on throws.
func TestAdversarialTypedLocalViewsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "TypedLocalViews", `import java.util.*;
class TypedLocalWitness {static String trace="";static final RuntimeException failure=new RuntimeException("same");}
public class TypedLocalViews {
 final Object lock; TypedLocalViews(Object lock){this.lock=lock;}
 int monitor(int mode){synchronized(lock){TypedLocalWitness.trace+="M";if(!Thread.holdsLock(lock))throw new Error("wrong receiver");if(mode==1)throw TypedLocalWitness.failure;return mode+3;}}
 static Class<?> reference(Class<?> input,int mode){Class<?> result=input;if(mode==1)result=null;else if(mode==2)result=result.getSuperclass();return result;}
 static int booleanLocal(boolean enabled,int fail){int value=enabled?1:0;try{if(fail!=0)throw TypedLocalWitness.failure;return value==0?11:19;}catch(RuntimeException e){Class<?> type=e.getClass();return type==RuntimeException.class?value+23:-999;}}
 static int array(int[] input,boolean use,int index){int[] data=use?input:null;TypedLocalWitness.trace+="A";return data==null?-1:data.length+data[index];}
 public static void main(String[]args){
  Object lock=new Object();for(Object receiver:new Object[]{lock,null})for(int mode=0;mode<3;mode++){TypedLocalWitness.trace="";try{System.out.println(new TypedLocalViews(receiver).monitor(mode)+":"+TypedLocalWitness.trace);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+(e==TypedLocalWitness.failure)+":"+TypedLocalWitness.trace);}System.out.println(Thread.holdsLock(lock));}
  for(Class<?> type:new Class<?>[]{Object.class,String.class,null})for(int mode=0;mode<3;mode++){try{Class<?> result=reference(type,mode);System.out.println(result==null?"null":result==type?"same":result.getName());}catch(Throwable e){System.out.println(e.getClass().getName());}}
  for(boolean enabled:new boolean[]{false,true})for(int fail=0;fail<2;fail++)System.out.println(booleanLocal(enabled,fail));
  int[] shared=new int[]{2,7};for(int[] input:new int[][]{shared,null,new int[0]})for(boolean use:new boolean[]{false,true})for(int index:new int[]{-1,0,2}){TypedLocalWitness.trace="";try{System.out.println(array(input,use,index)+":"+TypedLocalWitness.trace);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+TypedLocalWitness.trace);}System.out.println(Arrays.toString(shared));}
 }
}`, Precision, Compatibility, "legacy")
}
