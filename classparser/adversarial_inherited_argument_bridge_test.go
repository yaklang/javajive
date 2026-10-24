package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const inheritedArgumentBridgeFixture = `interface BridgePort<T>{T exchange(long n,double d,T value,T[] array)throws java.io.IOException;}
class BridgeEffects{static int calls;static boolean fail;static final java.io.IOException error=new java.io.IOException("identity");}
public class BridgeOuter implements BridgePort<String>{
 static class Leaf extends BridgeOuter{public Leaf(){}}
 static Object leaf(){return new Leaf();}
 public String exchange(long n,double d,String value,String[] array)throws java.io.IOException{
 BridgeEffects.calls++;if(BridgeEffects.fail)throw BridgeEffects.error;
 return n+":"+Long.toHexString(Double.doubleToRawLongBits(d))+":"+value+":"+(array==null?"null":java.util.Arrays.toString(array));}
}
class BridgeDerived extends BridgeOuter.Leaf{
 public String exchange(long n,double d,String value,String[] array){throw new AssertionError("super bridge retargeted virtually");}
 public Object special(long n,double d,Object value,Object[] array)throws Throwable{
 java.lang.invoke.MethodHandle h=java.lang.invoke.MethodHandles.lookup().findSpecial(BridgeOuter.Leaf.class,"exchange",java.lang.invoke.MethodType.methodType(Object.class,long.class,double.class,Object.class,Object[].class),BridgeDerived.class);
 return (Object)h.bindTo(this).invokeExact(n,d,value,array);}}
class BridgeDriver{public static void main(String[]args)throws Exception{
 Object leaf=BridgeOuter.leaf();java.lang.reflect.Method bridge=null;
 for(java.lang.reflect.Method m:Class.forName("BridgeOuter$Leaf").getDeclaredMethods()){
 if(bridge!=null||!m.isBridge()||!m.isSynthetic()||m.getReturnType()!=Object.class||!java.util.Arrays.equals(m.getParameterTypes(),new Class<?>[]{long.class,double.class,Object.class,Object[].class}))throw new AssertionError("exact original bridge universe");bridge=m;}
 if(bridge==null)throw new AssertionError("lost argument bridge");
 if(args.length>0&&args[0].equals("special")){leaf=new BridgeDerived();bridge=BridgeDerived.class.getDeclaredMethod("special",long.class,double.class,Object.class,Object[].class);}
 bridge.setAccessible(true);int rows=0;
 for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(double d:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY})for(Object v:new Object[]{null,"",new String("word"),new Object()})for(Object[] a:new Object[][]{null,new String[]{null,"item"},new Object[]{"item"},new Integer[]{1}})for(boolean fail:new boolean[]{false,true}){
 boolean castFailure=v!=null&&!(v instanceof String)||a!=null&&!(a instanceof String[]);BridgeEffects.calls=0;BridgeEffects.fail=fail;Throwable caught=null;Object result=null;
 try{result=bridge.invoke(leaf,n,d,v,a);}catch(java.lang.reflect.InvocationTargetException e){caught=e.getCause();}
 if(castFailure){if(!(caught instanceof ClassCastException)||BridgeEffects.calls!=0)throw new AssertionError("cast boundary before parent effects");}
 else if(fail){if(caught!=BridgeEffects.error||BridgeEffects.calls!=1)throw new AssertionError("checked exception identity");}
 else{String expected=n+":"+Long.toHexString(Double.doubleToRawLongBits(d))+":"+v+":"+(a==null?"null":java.util.Arrays.toString(a));if(caught!=null||!expected.equals(result)||BridgeEffects.calls!=1)throw new AssertionError("slots/result/dispatch");}
 rows++;}System.out.println(rows+":inherited:argument:casts:wide:dispatch:exception");}}
`

func TestAdversarialInheritedArgumentBridgePreservesChecksAndSuperDispatch(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, root := range []string{"BridgeOuter", "RenamedBridgeOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, parameters := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/parameters=%t", root, debug, parameters), func(t *testing.T) {
					testIndependentInheritedBridge(t, modern, java, native, strings.ReplaceAll(inheritedArgumentBridgeFixture, "BridgeOuter", root), root, debug, parameters, "288:inherited:argument:casts:wide:dispatch:exception\n")
				})
			}
		}
	}
}

func TestAdversarialInheritedArgumentBridgeKeepsPrimitiveResult(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	fixture := strings.NewReplacer("{T exchange(", "{boolean exchange(", "public String exchange(", "public boolean exchange(", "public Object special(", "public boolean special(", "methodType(Object.class,", "methodType(boolean.class,", "return (Object)h.bindTo(this).invokeExact", "return (boolean)h.bindTo(this).invokeExact", "m.getReturnType()!=Object.class", "m.getReturnType()!=boolean.class").Replace(inheritedArgumentBridgeFixture)
	old := `return n+":"+Long.toHexString(Double.doubleToRawLongBits(d))+":"+value+":"+(array==null?"null":java.util.Arrays.toString(array));`
	next := `return (n<0)^(Double.doubleToRawLongBits(d)<0)^(value==null)^(array==null);`
	if !strings.Contains(fixture, old) {
		t.Fatal("fixture return")
	}
	fixture = strings.Replace(fixture, old, next, 1)
	old = `String expected=n+":"+Long.toHexString(Double.doubleToRawLongBits(d))+":"+v+":"+(a==null?"null":java.util.Arrays.toString(a));if(caught!=null||!expected.equals(result)||BridgeEffects.calls!=1)`
	next = `boolean expected=(n<0)^(Double.doubleToRawLongBits(d)<0)^(v==null)^(a==null);if(caught!=null||!Boolean.valueOf(expected).equals(result)||BridgeEffects.calls!=1)`
	if !strings.Contains(fixture, old) {
		t.Fatal("fixture oracle")
	}
	fixture = strings.Replace(fixture, old, next, 1)
	for _, root := range []string{"BridgeOuter", "RenamedPrimitiveBridge"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(root+"/"+debug, func(t *testing.T) {
				testIndependentInheritedBridge(t, modern, java, native, strings.ReplaceAll(fixture, "BridgeOuter", root), root, debug, false, "288:inherited:argument:casts:wide:dispatch:exception\n")
			})
		}
	}
}
