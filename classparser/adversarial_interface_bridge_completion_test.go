package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// An interface bridge is a default method in modern classfiles. In addition
// to reflection/dispatch, its accidental presence changes interface
// initialization when an implementing class is initialized (JVMS 5.5).
func TestAdversarialInterfaceBridgePreservesInitializationAndDispatch(t *testing.T) {
	const source = `interface GenericContract<T> {T read(long n,T value);}
class InterfaceEffects {static String trace="";static Object seen;static long number;static int mark(){trace+="I";return 7;}}
public interface BridgeContract extends GenericContract<String> {int TOKEN=InterfaceEffects.mark();String read(long n,String value);class Impl implements BridgeContract {public Impl(){InterfaceEffects.trace+="C";}public String read(long n,String value){InterfaceEffects.trace+="R";InterfaceEffects.seen=value;InterfaceEffects.number=n;return value;}}}
class InterfaceDriver {public static void main(String[]args)throws Exception {if(!InterfaceEffects.trace.equals(""))throw new AssertionError("eager setup");BridgeContract instance=new BridgeContract.Impl();if(!InterfaceEffects.trace.equals("EXPECTED_INITIAL"))throw new AssertionError("default bridge changed initialization:"+InterfaceEffects.trace);int count=0;for(java.lang.reflect.Method m:BridgeContract.class.getDeclaredMethods())if(m.getName().equals("read"))count++;if(count!=EXPECTED_METHODS)throw new AssertionError("declared method universe:"+count);int rows=0;for(String value:new String[]{null,new String("same identity")})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){InterfaceEffects.trace="";if(instance.read(n,value)!=value||InterfaceEffects.seen!=value||InterfaceEffects.number!=n||!InterfaceEffects.trace.equals("R"))throw new AssertionError("typed call");GenericContract raw=instance;InterfaceEffects.trace="";if(raw.read(n,value)!=value||!InterfaceEffects.trace.equals("R"))throw new AssertionError("erased dispatch");InterfaceEffects.trace="";try{raw.read(n,new Object());throw new AssertionError("missing cast");}catch(ClassCastException expected){}if(!InterfaceEffects.trace.equals(""))throw new AssertionError("cast happened after side effect");rows++;}InterfaceEffects.trace="";if(BridgeContract.TOKEN!=7||!InterfaceEffects.trace.equals("EXPECTED_LATE"))throw new AssertionError("explicit interface initialization");System.out.println(rows+":interface:bridge:initialization:dispatch:cast:identity");}}`
	for _, owner := range []string{"BridgeContract", "RenamedBridgeContract"} {
		for _, original := range []int{5, 7, 8} {
			for _, target := range []int{8, 16} {
				t.Run(fmt.Sprintf("%s/original%d/target%d", owner, original, target), func(t *testing.T) {
					initial, late, methods := "C", "I", "1"
					if original == 8 {
						initial, late, methods = "IC", "", "2"
					}
					fixture := strings.NewReplacer("BridgeContract", owner, "EXPECTED_INITIAL", initial, "EXPECTED_LATE", late, "EXPECTED_METHODS", methods).Replace(source)
					roundTripConstructorBridgeArtifact(t, fixture, owner, "InterfaceDriver", "6:interface:bridge:initialization:dispatch:cast:identity\n", "none", target, original)
				})
			}
		}
	}
}

func TestAdversarialInterfaceBridgePreservesArrayPortsAndVoidReturn(t *testing.T) {
	const source = `interface ArrayPort<T>{void write(double word,T...values);}
class ArrayEffects {static Object seen;static long bits;static int calls;}
public interface ArrayProtocol extends ArrayPort<String>{void write(double word,String...values);class Impl implements ArrayProtocol {public void write(double word,String...values){ArrayEffects.seen=values;ArrayEffects.bits=Double.doubleToRawLongBits(word);ArrayEffects.calls++;}}}
class ArrayDriver {public static void main(String[]args){ArrayProtocol owner=new ArrayProtocol.Impl();ArrayPort raw=owner;int rows=0;for(String[]values:new String[][]{null,new String[]{"x",null},new String[0]})for(double word:new double[]{-0.0d,0.0d,Double.NaN,Double.NEGATIVE_INFINITY,Double.POSITIVE_INFINITY,Double.MIN_VALUE}){ArrayEffects.calls=0;owner.write(word,values);if(ArrayEffects.calls!=1||ArrayEffects.seen!=values||ArrayEffects.bits!=Double.doubleToRawLongBits(word))throw new AssertionError("typed array/void/double");raw.write(word,values);if(ArrayEffects.calls!=2||ArrayEffects.seen!=values||ArrayEffects.bits!=Double.doubleToRawLongBits(word))throw new AssertionError("erased array identity");try{raw.write(word,new Object[]{new Object()});throw new AssertionError("missing array cast");}catch(ClassCastException expected){}if(ArrayEffects.calls!=2)throw new AssertionError("array cast after effect");rows++;}int count=0;for(java.lang.reflect.Method m:ArrayProtocol.class.getDeclaredMethods())if(m.getName().equals("write"))count++;if(count!=EXPECTED_METHODS)throw new AssertionError("method universe:"+count);System.out.println(rows+":interface:array:varargs:void:double:identity");}}`
	for _, original := range []int{5, 7, 8} {
		for _, target := range []int{8, 16} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("original%d/target%d/%s", original, target, debug), func(t *testing.T) {
					methods := "1"
					if original == 8 {
						methods = "2"
					}
					fixture := strings.ReplaceAll(source, "EXPECTED_METHODS", methods)
					roundTripConstructorBridgeArtifact(t, fixture, "ArrayProtocol", "ArrayDriver", "18:interface:array:varargs:void:double:identity\n", debug, target, original)
				})
			}
		}
	}
}

// Multiple erased return contracts can produce multiple fresh default bridges.
// A diamond must be resolved by declaration identity, not by traversal count;
// retained declaration annotations and the implementation's result identity
// remain observable after removal of only the added forwarding declarations.
func TestAdversarialInterfaceBridgePreservesDiamondCovarianceAndAnnotations(t *testing.T) {
	const source = `@java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME) @interface BridgeMark {String value();}
interface GenericRoot<T>{T value();}
interface NumericContract extends GenericRoot<Number>{Number value();}
interface LeftContract extends NumericContract{}
interface RightContract extends NumericContract{}
class DiamondEffects {static int calls;static Integer result;}
public interface DiamondProtocol extends LeftContract,RightContract {@BridgeMark("leaf") Integer value();class Impl implements DiamondProtocol {public Integer value(){DiamondEffects.calls++;return DiamondEffects.result;}}}
class DiamondDriver {public static void main(String[]args)throws Exception{DiamondProtocol p=new DiamondProtocol.Impl();int rows=0;for(Integer n:new Integer[]{null,new Integer(-129),new Integer(128),new Integer(Integer.MIN_VALUE),new Integer(Integer.MAX_VALUE)}){DiamondEffects.result=n;DiamondEffects.calls=0;if(p.value()!=n||((LeftContract)p).value()!=n||((RightContract)p).value()!=n||((GenericRoot)p).value()!=n||DiamondEffects.calls!=4)throw new AssertionError("diamond dispatch or result identity");rows++;}int count=0;for(java.lang.reflect.Method m:DiamondProtocol.class.getDeclaredMethods()){if(!m.getName().equals("value"))throw new AssertionError("extra method");BridgeMark mark=m.getAnnotation(BridgeMark.class);if(mark==null||!mark.value().equals("leaf"))throw new AssertionError("annotation");if(m.getReturnType()==Integer.class&&(m.isBridge()||m.isSynthetic()))throw new AssertionError("original declaration flags");count++;}if(count!=EXPECTED_METHODS)throw new AssertionError("method universe:"+count);System.out.println(rows+":interface:diamond:covariance:annotations:identity");}}`
	for _, original := range []int{5, 7, 8} {
		for _, target := range []int{8, 16} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("original%d/target%d/%s", original, target, debug), func(t *testing.T) {
					methods := "1"
					if original == 8 {
						methods = "3"
					}
					fixture := strings.ReplaceAll(source, "EXPECTED_METHODS", methods)
					roundTripConstructorBridgeArtifact(t, fixture, "DiamondProtocol", "DiamondDriver", "5:interface:diamond:covariance:annotations:identity\n", debug, target, original)
				})
			}
		}
	}
}
