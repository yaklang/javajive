package javaclassparser

import (
	"testing"
)

func TestAdversarialEnumSwitchComputedSelectorKeepsTableInitializationAndThrownIdentityRoundTrip(t *testing.T) {
	for _, root := range []string{"InitSwitchOwner", "RenamedInitSwitchOwner"} {
		t.Run(root, func(t *testing.T) {
			sources := map[string]string{
				root + ".java": `public class ` + root + `{private final int base;public ` + root + `(int value){base=value;}public abstract static class Task{public Task(){}public abstract int run();}public Task make(final int delta){return new Task(){public int run(){switch(Selector.choose()){case A:return base+delta;case B:return base-delta;default:return base;}}};}}
class Trace{static String order="";static boolean nullResult,throwResult;static RuntimeException marker=new IllegalStateException("selector");}
class Selector{static ` + root + `Mode choose(){Trace.order+="C";if(Trace.throwResult)throw Trace.marker;return Trace.nullResult?null:` + root + `Mode.A;}}
class InitSwitchDriver{public static void main(String[]args)throws Exception{` + root + ` owner=new ` + root + `(Integer.MIN_VALUE);` + root + `.Task task=owner.make(Integer.MAX_VALUE);if(task.getClass().getEnclosingMethod()==null||!task.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("lexical identity");if(task.run()!=java.math.BigInteger.valueOf(Integer.MIN_VALUE).add(java.math.BigInteger.valueOf(Integer.MAX_VALUE)).intValue()||!Trace.order.equals("EC"))throw new AssertionError("enum initialization before selector:"+Trace.order);Trace.order="";Trace.nullResult=true;try{task.run();throw new AssertionError("null result");}catch(NullPointerException expected){if(!Trace.order.equals("C"))throw new AssertionError("null selector evaluation:"+Trace.order);}Trace.order="";Trace.throwResult=true;try{task.run();throw new AssertionError("throw result");}catch(RuntimeException actual){if(actual!=Trace.marker||!Trace.order.equals("C"))throw new AssertionError("original selector exception identity/order");}System.out.println("3:enum:init:selector:null:throw:lexical");}}`,
				root + "Mode.java": `public enum ` + root + `Mode{A,B,C;static{Trace.order+="E";}}`,
			}
			testNativeEnumSwitchSourceFixture(t, sources, root, "InitSwitchDriver", "3:enum:init:selector:null:throw:lexical\n")
		})
	}
}
