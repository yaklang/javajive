package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialClassLiteralMonitorRetainsOriginalClassIdentity(t *testing.T) {
	const fixture = `class ClassLockEffects{static String trace="";static final RuntimeException failure=new RuntimeException("original");static int probe(Object expected,int mode){if(!Thread.holdsLock(expected)||Thread.holdsLock(Class.class))throw new AssertionError("wrong original monitor identity");trace+="P";if(mode==1)throw failure;return mode;}}
class ClassLockOwner{static Class<?> check(Object expected,int mode){synchronized(ClassLockOwner.class){ClassLockEffects.probe(expected,mode);return ClassLockOwner.class;}}}
class ClassLockDriver{public static void main(String[]args){for(int mode:new int[]{0,1,-1,Integer.MIN_VALUE,Integer.MAX_VALUE}){ClassLockEffects.trace="";Class<?> value=null;Throwable actual=null;try{value=ClassLockOwner.check(ClassLockOwner.class,mode);}catch(Throwable e){actual=e;}if(!ClassLockEffects.trace.equals("P")||actual!=(mode==1?ClassLockEffects.failure:null)||value!=(mode==1?null:ClassLockOwner.class)||Thread.holdsLock(ClassLockOwner.class)||Thread.holdsLock(Class.class))throw new AssertionError("class literal identity/monitor release/exception identity");}System.out.println("5:class-literal:monitor-identity:release:abrupt");}}`
	for _, shape := range []string{"simple", "double checked cache"} {
		t.Run(shape, func(t *testing.T) {
			f := fixture
			if shape != "simple" {
				f = strings.ReplaceAll(f, `class ClassLockOwner{static Class<?> check(Object expected,int mode){synchronized(ClassLockOwner.class){ClassLockEffects.probe(expected,mode);return ClassLockOwner.class;}}}`, `class ClassLockOwner{static Class<?> cached;static Class<?> check(Object expected,int mode){Class<?> value=cached;if(value==null){synchronized(ClassLockOwner.class){value=cached;if(value==null){ClassLockEffects.probe(expected,mode);value=ClassLockOwner.class;cached=value;}}}return value;}}`)
				f = strings.ReplaceAll(f, `ClassLockEffects.trace="";Class<?> value`, `ClassLockOwner.cached=null;ClassLockEffects.trace="";Class<?> value`)
			}
			testNativeIndependentFamilyFixture(t, f, []string{"ClassLockOwner"}, "ClassLockDriver", "5:class-literal:monitor-identity:release:abrupt\n", nativeLexicalExactSignatures)
		})
	}
}
