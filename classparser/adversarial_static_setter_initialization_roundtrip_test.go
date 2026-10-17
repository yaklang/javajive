package javaclassparser

import (
	"strings"
	"testing"
)

const staticSetterInitialization = `class StaticWriteEffects{static String trace="";static boolean failArgument,failInitialization;static final RuntimeException failure=new RuntimeException("original");static StaticWriteOwner receiver(){trace+="R";return null;}static StaticWriteOwnerSafe safeReceiver(){trace+="R";return null;}static Object argument(Object v){trace+="V";if(failArgument)throw failure;return v;}static Object init(){trace+="I";if(failInitialization)throw failure;return new Object();}}
class StaticWriteOwner{private static volatile Object value=StaticWriteEffects.init();static class Writer{static Object put(Object v){return StaticWriteEffects.receiver().value=StaticWriteEffects.argument(v);}}}
class StaticWriteOwnerSafe{private static volatile Object value=StaticWriteEffects.init();static class Writer{static Object put(Object v){return StaticWriteEffects.safeReceiver().value=StaticWriteEffects.argument(v);}static void store(Object v){StaticWriteEffects.safeReceiver().value=StaticWriteEffects.argument(v);}static Object get(){return value;}static Object chain(Object v){return value=(value=StaticWriteEffects.argument(v));}}}
class StaticWriteDriver{public static void main(String[]args){Object token=new Object();StaticWriteEffects.failArgument=true;StaticWriteEffects.failInitialization=true;StaticWriteEffects.trace="";try{StaticWriteOwner.Writer.put(token);throw new AssertionError("missing RHS failure");}catch(RuntimeException e){if(e!=StaticWriteEffects.failure||!StaticWriteEffects.trace.equals("RV"))throw new AssertionError("RHS before class initialization/identity",e);}StaticWriteEffects.failArgument=false;StaticWriteEffects.trace="";try{StaticWriteOwner.Writer.put(token);throw new AssertionError("missing initializer failure");}catch(ExceptionInInitializerError e){if(e.getCause()!=StaticWriteEffects.failure||!StaticWriteEffects.trace.equals("RVI"))throw new AssertionError("null qualifier/RHS/class initialization/order",e);}StaticWriteEffects.trace="";try{StaticWriteOwner.Writer.put(token);throw new AssertionError("missing erroneous class state");}catch(NoClassDefFoundError e){if(!StaticWriteEffects.trace.equals("RV"))throw new AssertionError("RHS still evaluated before erroneous-class failure");}StaticWriteEffects.failInitialization=false;StaticWriteEffects.trace="";if(StaticWriteOwnerSafe.Writer.put(token)!=token||StaticWriteOwnerSafe.Writer.get()!=token||!StaticWriteEffects.trace.equals("RVI"))throw new AssertionError("success/volatile storage/initialization/identity");StaticWriteEffects.trace="";StaticWriteOwnerSafe.Writer.store(null);if(StaticWriteOwnerSafe.Writer.get()!=null||!StaticWriteEffects.trace.equals("RV"))throw new AssertionError("discarded result/null receiver/initializer repeated");StaticWriteEffects.trace="";if(StaticWriteOwnerSafe.Writer.chain(token)!=token||StaticWriteOwnerSafe.Writer.get()!=token||!StaticWriteEffects.trace.equals("V"))throw new AssertionError("nested write/registration/identity");System.out.println("static:write:original:argument:init:failure:identity:once:storage");}}`

func TestAdversarialStaticSetterKeepsInitializationAndOperandEffects(t *testing.T) {
	for _, formal := range []bool{false, true} {
		name := "ordinary"
		f := staticSetterInitialization
		if formal {
			name = "formal class collision"
			i := strings.Index(f, "class StaticWriteOwnerSafe{")
			f = f[:i] + strings.Replace(f[i:], "static Object put(Object v)", "static <StaticWriteOwnerSafe extends Number> Object put(Object v)", 1)
		}
		t.Run(name, func(t *testing.T) {
			testNativePrivateSetterFixtureDefaultPolicy(t, f, "StaticWriteOwner", "StaticWriteDriver", "static:write:original:argument:init:failure:identity:once:storage\n", nil)
		})
	}
}
