package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Original binary parents call the first anonymous object's method during
// construction. Keeping its local declaration must preserve both the capture
// before SUPER and javac's anonymous registration order, including failures.
func testNativeAnonymousLocalRegistration(t *testing.T, native bool) {
	for _, name := range []string{"Receipt", "Ticket", "Inline"} {
		t.Run(name, func(t *testing.T) {
			sources := map[string]string{
				"p/Events.java":  `package p;public class Events{public static String trace;public static int fail;public static Factory first;public static Result second;public static final RuntimeException failure=new IllegalArgumentException("identity");public static void hit(int n){trace+=n;if(fail==trace.length())throw failure;}}`,
				"p/Factory.java": `package p;public abstract class Factory{public final Object observed;public Factory(){Events.first=this;Events.hit(1);observed=get();}public abstract Object get();}`,
				"p/Result.java":  `package p;public abstract class Result{public final Object observed;public Result(Factory f){Events.second=this;Events.hit(3);observed=f.get();}public abstract Object read();}`,
			}
			owner := "q/" + name + "Owner"
			driver := "q." + name + "Driver"
			sources[owner+".java"] = fmt.Sprintf(`package q;public class %sOwner{public p.Result make(final Object value){p.Factory factory=new p.Factory(){public Object get(){p.Events.hit(2);return value;}};return new p.Result(factory){public Object read(){p.Events.hit(4);return observed;}};}}`, name)
			sources["q/"+name+"Driver.java"] = fmt.Sprintf(`package q;public class %sDriver{public static void main(String[]args)throws Exception{int rows=0;for(Object value:new Object[]{null,new Object(),new String("identity")})for(int fail=0;fail<=5;fail++){p.Events.trace="";p.Events.fail=fail;p.Events.first=null;p.Events.second=null;boolean caught=false;try{p.Result r=new %sOwner().make(value);if(r.read()!=value)throw new AssertionError("result identity");}catch(RuntimeException ex){if(ex!=p.Events.failure)throw ex;caught=true;}String full="12324";String want=fail==0?full:full.substring(0,fail);if(!p.Events.trace.equals(want)||caught!=(fail!=0))throw new AssertionError("evaluation/failure order "+p.Events.trace);Object[] instances={p.Events.first,p.Events.second};for(int i=0;i<instances.length;i++){Object instance=instances[i];if(instance==null){if(i==0||fail==0||fail>2)throw new AssertionError("lost construction");continue;}Class<?> c=instance.getClass();if(!c.isAnonymousClass()||!c.getName().equals("q.%sOwner$"+(i+1))||c.getEnclosingMethod().getDeclaringClass()!=%sOwner.class||!c.getEnclosingMethod().getName().equals("make")||c.getSuperclass()!=(i==0?p.Factory.class:p.Result.class))throw new AssertionError("registration/ownership");}if(p.Events.first!=null&&fail!=1&&fail!=2&&p.Events.first.observed!=value)throw new AssertionError("capture before parent callback");rows++;}System.out.println(rows+":registration:captures:callbacks:failure-identity");}}`, name, name, name, name)
			want := "18:registration:captures:callbacks:failure-identity\n"
			if name == "Inline" {
				sources[owner+".java"] = fmt.Sprintf(`package q;public class %sOwner{public p.Result make(final Object value){return new p.Result(new p.Factory(){public Object get(){p.Events.hit(2);return value;}}){public Object read(){p.Events.hit(4);return observed;}};}}`, name)
			}
			if name == "Ticket" {
				// A trivial first parent allows the general effect-motion proof
				// to inline the local. Runtime motion may be safe while anonymous
				// source registration still reverses the two original classes.
				sources["p/Factory.java"] = `package p;public abstract class Factory{public abstract Object get();}`
				sources["p/Result.java"] = `package p;public abstract class Result{public final Object observed;public Result(Factory f){Events.first=f;Events.second=this;Events.hit(1);observed=f.get();}public abstract Object read();}`
				key := "q/" + name + "Driver.java"
				s := sources[key]
				s = strings.ReplaceAll(s, "fail<=5", "fail<=3")
				s = strings.ReplaceAll(s, `String full="12324"`, `String full="124"`)
				s = strings.ReplaceAll(s, `if(p.Events.first!=null&&fail!=1&&fail!=2&&p.Events.first.observed!=value)throw new AssertionError("capture before parent callback");`, "")
				sources[key] = s
				want = "12:registration:captures:callbacks:failure-identity\n"
			}
			if native {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for independent compiler")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return fieldOwnerCompileJava8Sources(t, sources, debug) }, NativeJavac8, compiler, []string{owner}, driver, want, nil, nativeLexicalExactSignatures)
				return
			}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, owner, driver, want, func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialAnonymousLocalRegistration(t *testing.T) {
	testNativeAnonymousLocalRegistration(t, false)
}

func TestAdversarialAnonymousLocalRegistrationNativeCompiler(t *testing.T) {
	testNativeAnonymousLocalRegistration(t, true)
}
