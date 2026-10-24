package javaclassparser

import "testing"

// The original interface has no declared methods. Its initializer allocation
// trees must remain in <clinit>, including every primitive width, reference
// identity and aliases observed by an unchanged binary caller.
const interfaceArrayInitializerFixture = `
class InitPacketEffects {static final Object shared=new Object();}
interface InitPacketContract {
 int[] I={Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE};
 long[] J={Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE};
 byte[] B={Byte.MIN_VALUE,-1,0,1,Byte.MAX_VALUE};
 short[] S={Short.MIN_VALUE,-1,0,1,Short.MAX_VALUE};
 char[] C={0,1,127,32768,65535};
 boolean[] Z={false,true,false,true};
 float[] F={-0.0f,0.0f,Float.NaN,Float.NEGATIVE_INFINITY,Float.POSITIVE_INFINITY};
 double[] D={-0.0d,0.0d,Double.NaN,Double.NEGATIVE_INFINITY,Double.POSITIVE_INFINITY};
 Object[] R={InitPacketEffects.shared,null,InitPacketEffects.shared};
 Object[] A={I,J,R};
 String[] TEXT={"",null,"a\u0000b","\ud83d\ude03"};
}
public class InitPacketDriver {public static void main(String[] args) {
 if(InitPacketContract.class.getDeclaredMethods().length!=0)throw new AssertionError("invented method declaration");
 if(!java.util.Arrays.equals(InitPacketContract.I,new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})||!java.util.Arrays.equals(InitPacketContract.J,new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE}))throw new AssertionError("numeric array values");
 if(!java.util.Arrays.equals(InitPacketContract.B,new byte[]{Byte.MIN_VALUE,-1,0,1,Byte.MAX_VALUE})||!java.util.Arrays.equals(InitPacketContract.S,new short[]{Short.MIN_VALUE,-1,0,1,Short.MAX_VALUE})||!java.util.Arrays.equals(InitPacketContract.C,new char[]{0,1,127,32768,65535})||!java.util.Arrays.equals(InitPacketContract.Z,new boolean[]{false,true,false,true}))throw new AssertionError("narrow array values");
 float[] f={-0.0f,0.0f,Float.NaN,Float.NEGATIVE_INFINITY,Float.POSITIVE_INFINITY};double[] d={-0.0d,0.0d,Double.NaN,Double.NEGATIVE_INFINITY,Double.POSITIVE_INFINITY};
 for(int i=0;i<f.length;i++)if(Float.floatToRawIntBits(f[i])!=Float.floatToRawIntBits(InitPacketContract.F[i])||Double.doubleToRawLongBits(d[i])!=Double.doubleToRawLongBits(InitPacketContract.D[i]))throw new AssertionError("floating representation");
 if(InitPacketContract.R[0]!=InitPacketEffects.shared||InitPacketContract.R[1]!=null||InitPacketContract.R[2]!=InitPacketContract.R[0]||InitPacketContract.A[0]!=InitPacketContract.I||InitPacketContract.A[1]!=InitPacketContract.J||InitPacketContract.A[2]!=InitPacketContract.R||InitPacketContract.I!=InitPacketContract.I)throw new AssertionError("array and element identity");
 if(!java.util.Arrays.equals(InitPacketContract.TEXT,new String[]{"",null,"a\u0000b","\ud83d\ude03"}))throw new AssertionError("string units");
 System.out.println("array:initializer:types:identity:abi");
}}
`

func TestAdversarialInterfaceArrayInitializersKeepOriginalMethodUniverse(t *testing.T) {
	testInterfaceLambdaInitializerFamily(t, interfaceArrayInitializerFixture, "array:initializer:types:identity:abi\n")
}

func TestAdversarialInterfaceArrayInitializersKeepEffectsAndFirstFailure(t *testing.T) {
	fixture := `
class InitPacketEffects {static String trace="";static final IllegalStateException failure=new IllegalStateException("original cause");static Object item(String id,boolean fail){trace+=id;if(fail)throw failure;return new Object();}}
interface InitPacketContract {Object[] FIRST={InitPacketEffects.item("A",false),InitPacketEffects.item("B",true),InitPacketEffects.item("C",false)};Object[] SECOND={InitPacketEffects.item("D",false)};}
public class InitPacketDriver {public static void main(String[] args){
 if(InitPacketContract.class.getDeclaredMethods().length!=0||!InitPacketEffects.trace.equals(""))throw new AssertionError("premature initialization or invented declaration");
 Throwable first=null,second=null;Object cause=InitPacketEffects.failure;
 try{Object ignored=InitPacketContract.FIRST;}catch(Throwable e){first=e;}
 try{Object ignored=InitPacketContract.SECOND;}catch(Throwable e){second=e;}
 if(!(first instanceof ExceptionInInitializerError)||first.getCause()!=cause||!(second instanceof NoClassDefFoundError)||!InitPacketEffects.trace.equals("AB"))throw new AssertionError("effect order, original failure identity, or repeated initialization",first);
 System.out.println("array:initializer:first-failure:identity:no-retry");
}}
`
	testInterfaceLambdaInitializerFamily(t, fixture, "array:initializer:first-failure:identity:no-retry\n")
}

func TestAdversarialInterfaceWrappedArrayKeepsBindingIdentityAndOrder(t *testing.T) {
	fixture := `
class InitPacketEffects {static String trace="";static final Object shared=new Object();static Object item(String id){trace+=id;return shared;}static String select(String[] x){trace+="S";return x[0];}static String select(Object x){throw new AssertionError("changed overload");}}
interface InitPacketContract {
 java.util.List<Object> VIEW=java.util.Collections.unmodifiableList(java.util.Arrays.asList(new Object[]{InitPacketEffects.item("A"),null,InitPacketEffects.item("B")}));
 String SELECTED=InitPacketEffects.select(new String[]{"chosen"});
 java.util.List<int[]> PRIMITIVE=java.util.Arrays.asList(new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE});
}
public class InitPacketDriver {public static void main(String[]args){
 if(InitPacketContract.class.getDeclaredMethods().length!=0||!InitPacketEffects.trace.equals(""))throw new AssertionError("invented declaration or premature initialization");
 java.util.List<Object> values=InitPacketContract.VIEW;
 if(values.size()!=3||values.get(0)!=InitPacketEffects.shared||values.get(1)!=null||values.get(2)!=values.get(0)||values!=InitPacketContract.VIEW)throw new AssertionError("shared element or container identity");
 try{values.set(0,new Object());throw new AssertionError("lost wrapper");}catch(UnsupportedOperationException expected){}
 if(!InitPacketContract.SELECTED.equals("chosen")||!InitPacketEffects.trace.equals("ABS"))throw new AssertionError("binding or evaluation order");
 if(InitPacketContract.PRIMITIVE.size()!=1||!java.util.Arrays.equals(InitPacketContract.PRIMITIVE.get(0),new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE}))throw new AssertionError("primitive array generic argument");
 System.out.println("wrapped-array:binding:identity:order");
}}
`
	testInterfaceLambdaInitializerFamily(t, fixture, "wrapped-array:binding:identity:order\n")
}

func TestAdversarialInterfaceWrappedArrayKeepsWrapperFailureAndNoRetry(t *testing.T) {
	fixture := `
class InitPacketEffects {static String trace="";static final IllegalStateException failure=new IllegalStateException("wrapper cause");static Object item(String id){trace+=id;return new Object();}static Object[] wrap(Object[] x){trace+="W";throw failure;}static Object[] outer(Object[] x){trace+="X";return x;}}
interface InitPacketContract {Object[] FIRST=InitPacketEffects.outer(InitPacketEffects.wrap(new Object[]{InitPacketEffects.item("A"),InitPacketEffects.item("B")}));Object SECOND=InitPacketEffects.item("C");}
public class InitPacketDriver {public static void main(String[]args){
 if(InitPacketContract.class.getDeclaredMethods().length!=0||!InitPacketEffects.trace.equals(""))throw new AssertionError("premature initialization or invented declaration");
 Throwable first=null,second=null;Object cause=InitPacketEffects.failure;
 try{Object ignored=InitPacketContract.FIRST;}catch(Throwable e){first=e;}
 try{Object ignored=InitPacketContract.SECOND;}catch(Throwable e){second=e;}
 if(!(first instanceof ExceptionInInitializerError)||first.getCause()!=cause||!(second instanceof NoClassDefFoundError)||!InitPacketEffects.trace.equals("ABW"))throw new AssertionError("wrapper failure identity or order",first);
 System.out.println("wrapped-array:failure:identity:no-retry");
}}
`
	testInterfaceLambdaInitializerFamily(t, fixture, "wrapped-array:failure:identity:no-retry\n")
}
