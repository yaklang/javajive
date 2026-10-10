package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMemberAnonymousFixture = `class NestedEffects {static String trace="";static Object published;static boolean fail;static final IllegalArgumentException error=new IllegalArgumentException("same");static long arg(long x){trace+="A";return x;}}
class NestedOwner<T> {
 final T token;NestedOwner(T token){this.token=token;}
 private abstract class Parent<U> {final Object observed,observedValue;final long number;Parent(long n){NestedEffects.trace+="P";observed=origin();observedValue=get();number=n;NestedEffects.published=this;if(NestedEffects.fail)throw NestedEffects.error;}abstract Object origin();abstract U get();}
 class Actor<Z> {Parent<T> make(final T value,long n){return new Parent<T>(NestedEffects.arg(n)){Object origin(){return Actor.this;}T get(){return value;}};}Runnable task(final Object value){return new Runnable(){public void run(){if(value!=Actor.this)throw new AssertionError("capture");}};}Object outer(){return NestedOwner.this;}}
 static class StaticActor<X> {Runnable task(final X value){return new Runnable(){public void run(){if(value!=StaticActor.this)throw new AssertionError("static enclosing capture");}};}}
 Actor<String> actor(){return new Actor<String>();}
 Runnable probe(final Object value){return new Runnable(){public void run(){if(value!=NestedOwner.this.token)throw new AssertionError("root capture");}};}
 Object[] sample(Actor<?> actor,T value,long n){Parent<T> parent=actor.make(value,n);return new Object[]{parent,parent.observed,parent.get(),Long.valueOf(parent.number),parent.observedValue};}
}
class NestedDriver {public static void main(String[]args)throws Exception{
 NestedOwner<Object> owner=new NestedOwner<Object>(new Object());Object token=new Object();int rows=0;owner.probe(owner.token).run();
 java.lang.reflect.Constructor<?> ctor=NestedOwner.Actor.class.getDeclaredConstructor(NestedOwner.class);ctor.setAccessible(true);
 for(boolean nullOuter:new boolean[]{false,true}){NestedOwner<Object>.Actor<String> actor=nullOuter?(NestedOwner<Object>.Actor<String>)ctor.newInstance(new Object[]{null}):owner.actor();actor.task(actor).run();
 for(boolean fail:new boolean[]{false,true})for(Object value:new Object[]{null,token})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){NestedEffects.trace="";NestedEffects.published=null;NestedEffects.fail=fail;
 try{Object[] r=owner.sample(actor,value,n);if(fail||r[0]!=NestedEffects.published||r[1]!=actor||r[2]!=value||r[4]!=value||((Long)r[3]).longValue()!=n||!NestedEffects.trace.equals("AP"))throw new AssertionError("state");}
 catch(IllegalArgumentException e){if(!fail||e!=NestedEffects.error||NestedEffects.published==null||!NestedEffects.trace.equals("AP"))throw new AssertionError("failure");java.lang.reflect.Field observed=NestedEffects.published.getClass().getSuperclass().getDeclaredField("observedValue");observed.setAccessible(true);if(observed.get(NestedEffects.published)!=value)throw new AssertionError("pre-super value capture");}rows++;}
 Class<?> anon=NestedEffects.published.getClass();if(!anon.isAnonymousClass()||anon.getEnclosingClass()!=NestedOwner.Actor.class||!anon.getEnclosingMethod().getName().equals("make"))throw new AssertionError("nested origin");}
 NestedOwner.StaticActor<Object> staticActor=new NestedOwner.StaticActor<Object>();staticActor.task(staticActor).run();System.out.println(rows+":"+NestedEffects.trace);
}}`

func TestNativeMemberOwnedAnonymousCaptureRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	for _, bounds := range []string{"unbounded", "Number", "DollarRoot"} {
		fixture := nativeMemberAnonymousFixture
		rootName := "NestedOwner"
		debugModes := []string{"none", "source,lines,vars"}
		policies := []string{"normal", "no-source-rewrites", "no-core-cleanups"}
		if bounds == "Number" {
			fixture = strings.ReplaceAll(fixture, "NestedOwner<T>", "NestedOwner<T extends Number>")
			fixture = strings.ReplaceAll(fixture, "NestedOwner<Object>", "NestedOwner<Number>")
			fixture = strings.ReplaceAll(fixture, "(new Object())", "(Long.valueOf(7))")
			fixture = strings.ReplaceAll(fixture, "Object token=new Object()", "Number token=Long.valueOf(Long.MIN_VALUE)")
			fixture = strings.ReplaceAll(fixture, "for(Object value:new Object[]", "for(Number value:new Number[]")
		}
		if bounds == "DollarRoot" {
			rootName = "Nested$Owner"
			fixture = strings.ReplaceAll(fixture, "NestedOwner", rootName)
			debugModes, policies = []string{"none"}, []string{"normal"}
		}
		for _, debug := range debugModes {
			t.Run(bounds+"/"+debug, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, fixture, debug)
				original := t.TempDir()
				for n, raw := range files {
					if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				oracle := t04RunJava(t, java, original, "NestedDriver")
				if oracle != "24:AP\n" {
					t.Fatalf("original oracle %q", oracle)
				}
				for _, policy := range policies {
					t.Run(policy, func(t *testing.T) {
						if policy == "no-source-rewrites" {
							t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
						}
						if policy == "no-core-cleanups" {
							t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
						}
						z := nativeArchive(t, files)
						defer z.Close()
						for _, n := range []string{"NestedOwner$1", "NestedOwner$Actor", "NestedOwner$Actor$1", "NestedOwner$Actor$2", "NestedOwner$StaticActor$1"} {
							n = strings.Replace(n, "NestedOwner", rootName, 1)
							raw, err := z.ReadFile(n + ".class")
							if err != nil || !strings.Contains(string(raw), "body owned by") {
								t.Fatalf("original joint ownership %s %v\n%s", n, err, raw)
							}
						}
						src, err := z.ReadFile(rootName + ".class")
						if err != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("source %v\n%s", err, src)
						}
						output := t.TempDir()
						for n, raw := range files {
							if strings.HasPrefix(n, rootName) {
								continue
							}
							if err := os.WriteFile(filepath.Join(output, n), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
						file := filepath.Join(output, rootName+".java")
						if err := os.WriteFile(file, src, 0600); err != nil {
							t.Fatal(err)
						}
						if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); err != nil {
							t.Fatalf("rebuilt %v %s\n%s", err, out, src)
						}
						if got := t04RunJava(t, java, output, "NestedDriver"); got != oracle {
							t.Fatalf("JVM mismatch %q != %q", got, oracle)
						}
						for n, want := range files {
							if !strings.HasPrefix(n, rootName) {
								continue
							}
							raw, err := os.ReadFile(filepath.Join(output, n))
							if err != nil {
								t.Fatal(err)
							}
							if got := nativeBinaryShape(t, raw); got != nativeBinaryShape(t, want) {
								t.Fatalf("ABI changed %s\n%s\n%s", n, nativeBinaryShape(t, want), got)
							}
						}
					})
				}
			})
		}
	}
}
