package javaclassparser

import (
	"strings"
	"testing"
)

// The unchanged original driver observes both enum initializers before a
// computed selector, including the initializer of the unused second switch.
// A nondefault parent constructor makes losing lexical ownership executable.
func nativeMultipleTableSwitchSources() map[string]string {
	return map[string]string{
		"MultiTableOwner.java": `public class MultiTableOwner {
 private final int base; public MultiTableOwner(int n){base=n;}
 public abstract static class Task {protected final int seed; public Task(int n){seed=n;} public abstract int first();public abstract int second();}
 private static SwitchAxisA select(){SwitchTrace.order+="S";if(SwitchTrace.fail)throw SwitchTrace.marker;return SwitchTrace.nil?null:SwitchAxisA.C;}
 public Task make(final int delta){return new Task(delta){
  public int first(){switch(select()){case C:return base+seed;case A:return base-seed;default:return base;}}
  public int second(){switch(SwitchAxisB.B){case B:return base+delta;case A:return base-delta;default:return base;}}
 };}
}
class MultiTableDriver {public static void main(String[] args){
 MultiTableOwner.Task task=new MultiTableOwner(Integer.MIN_VALUE).make(Integer.MAX_VALUE);
 if(task.first()!=-1||!SwitchTrace.order.equals("BAS"))throw new AssertionError("table initialization/order:"+SwitchTrace.order);
 SwitchTrace.order="";SwitchTrace.nil=true;try{task.first();throw new AssertionError("null selector");}catch(NullPointerException expected){if(!SwitchTrace.order.equals("S"))throw new AssertionError("null order");}
 SwitchTrace.order="";SwitchTrace.fail=true;try{task.first();throw new AssertionError("selector throw");}catch(RuntimeException actual){if(actual!=SwitchTrace.marker||!SwitchTrace.order.equals("S"))throw new AssertionError("throw identity/order");}
 if(task.second()!=-1||task.getClass().getEnclosingMethod()==null||!task.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("second switch/lexical binding");
 System.out.println("4:multi-table:init:selector:null:throw:parent:binding");
}}
class SwitchTrace {static String order="";static boolean nil,fail;static final RuntimeException marker=new IllegalStateException("selector");}`,
		"SwitchAxisA.java": `public enum SwitchAxisA{A,B,C;static{SwitchTrace.order+="A";}}`,
		"SwitchAxisB.java": `public enum SwitchAxisB{A,B,C;static{SwitchTrace.order+="B";}}`,
	}
}

func TestAdversarialEnumSwitchMultipleTablesPreserveInitializationRoundTrip(t *testing.T) {
	sources := nativeMultipleTableSwitchSources()
	testNativeEnumSwitchSourceFixture(t, sources, "MultiTableOwner", "MultiTableDriver", "4:multi-table:init:selector:null:throw:parent:binding\n")
}

func TestAdversarialEnumSwitchMultipleTablesNestedPostorderRoundTrip(t *testing.T) {
	sources := nativeMultipleTableSwitchSources()
	sources["MultiTableOwner.java"] = strings.Replace(sources["MultiTableOwner.java"],
		"case C:return base+seed;", "case C:switch(SwitchAxisB.B){case B:return base+seed;case A:return base-seed;default:return base;}", 1)
	// The nested B switch registers before the containing A switch. javac
	// prepends their initializers, so now A initializes before B and select().
	sources["MultiTableOwner.java"] = strings.ReplaceAll(sources["MultiTableOwner.java"], "BAS", "ABS")
	testNativeEnumSwitchSourceFixture(t, sources, "MultiTableOwner", "MultiTableDriver", "4:multi-table:init:selector:null:throw:parent:binding\n")
}

func TestAdversarialEnumSwitchMultipleTablesInitializerFailureRoundTrip(t *testing.T) {
	sources := nativeMultipleTableSwitchSources()
	source := sources["MultiTableOwner.java"]
	start, end := strings.Index(source, "class MultiTableDriver"), strings.Index(source, "class SwitchTrace")
	sources["MultiTableOwner.java"] = source[:start] + `class MultiTableDriver {public static void main(String[] args){
 MultiTableOwner.Task task=new MultiTableOwner(13).make(17);
 try{task.first();throw new AssertionError("missing init failure");}catch(ExceptionInInitializerError actual){if(actual.getException()!=SwitchTrace.marker||!SwitchTrace.order.equals("B"))throw new AssertionError("failure cause/order:"+SwitchTrace.order);}
 try{task.first();throw new AssertionError("missing erroneous helper");}catch(NoClassDefFoundError expected){if(!SwitchTrace.order.equals("B"))throw new AssertionError("repeated selector/initializer");}
 try{task.second();throw new AssertionError("second table escaped helper failure");}catch(NoClassDefFoundError expected){if(!SwitchTrace.order.equals("B"))throw new AssertionError("second selector/initializer");}
 System.out.println("3:multi-table:failure:cause:erroneous-helper:unused-enum");
}}
` + source[end:]
	sources["SwitchAxisB.java"] = `public enum SwitchAxisB{A,B,C;static{SwitchTrace.order+="B";if(!SwitchTrace.nil)throw SwitchTrace.marker;}}`
	testNativeEnumSwitchSourceFixture(t, sources, "MultiTableOwner", "MultiTableDriver", "3:multi-table:failure:cause:erroneous-helper:unused-enum\n")
}
