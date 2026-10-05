package javaclassparser

import (
	"strings"
	"testing"
)

const nativeEmptyAnonymousFixture = `class EmptyScope{static final EmptyScope singleton=new EmptyScope(){};interface Factory{EmptyScope create();}static Factory factory(final EmptyScope input){return new Factory(){public EmptyScope create(){return input;}};}}
class EmptyScopeDriver{public static void main(String[]args){EmptyScope token=new EmptyScope();if(EmptyScope.factory(token).create()!=token||EmptyScope.factory(null).create()!=null)throw new AssertionError("identity");if(EmptyScope.singleton.getClass()==EmptyScope.class)throw new AssertionError("missing anonymous class");System.out.println("empty:anonymous:identity:ordinal");}}`

func TestNativeEmptyAnonymousBodyRoundTrip(t *testing.T) {
	testNativePrivateSetterCompiledFixture(t, "EmptyScope", "EmptyScopeDriver", "empty:anonymous:identity:ordinal\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, nativeEmptyAnonymousFixture, debug)
	})
}
func TestNativeEmptyAnonymousBodyRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeEmptyAnonymousFixture, "EmptyScope", "VacantOwner")
	testNativePrivateSetterCompiledFixture(t, "VacantOwner", "VacantOwnerDriver", "empty:anonymous:identity:ordinal\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) })
}

const nativeEmptyCapturedOwnerFixture = `class CapturedVacantOwner{class Child{Child(){}}Object make(){return new Child();}Object other(){return new Object(){};}}
class CapturedVacantDriver{public static void main(String[]args)throws Exception{int rows=0;for(CapturedVacantOwner owner:new CapturedVacantOwner[]{new CapturedVacantOwner(),new CapturedVacantOwner()}){Object child=owner.make(),value=owner.other();if(child.getClass().getDeclaringClass()!=CapturedVacantOwner.class||value.getClass().getEnclosingClass()!=CapturedVacantOwner.class||!value.getClass().getEnclosingMethod().getName().equals("other"))throw new AssertionError("source owner metadata");java.lang.reflect.Field capture=value.getClass().getDeclaredField("this$0");capture.setAccessible(true);if(capture.get(value)!=owner)throw new AssertionError("enclosing identity");rows++;}System.out.println(rows+":empty:anonymous:captured:owner");}}`

func TestNativeEmptyAnonymousCapturedOwnerRoundTrip(t *testing.T) {
	testNativePrivateSetterCompiledFixture(t, "CapturedVacantOwner", "CapturedVacantDriver", "2:empty:anonymous:captured:owner\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, nativeEmptyCapturedOwnerFixture, debug)
	})
}
