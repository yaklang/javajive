package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMemberPrivateSuperBridgeFixture = `class SuperEffects{static Object published;static boolean fail;static String trace="";static final IllegalArgumentException failure=new IllegalArgumentException("singleton");static long arg(long n){trace+="A";return n;}}
class SuperOwner<T>{
 static Object capture(final Object x){return new Object(){public String toString(){return x==null?"null":"value";}};}
 private abstract class Parent<V>{final V seen;final long parentN;final Object observed;private Parent(V value,long n){SuperEffects.trace+="P";seen=value;parentN=n;SuperEffects.published=this;observed=origin();if(SuperEffects.fail)throw SuperEffects.failure;}abstract Object origin();}
 class Child<U> extends Parent<U>{final U value;final long n;private Child(U value,long n){super(value,n);this.value=value;this.n=n;SuperEffects.trace+="C";}Object origin(){return SuperOwner.this;}}
 Child<T> make(T value,long n){return new Child<T>(value,SuperEffects.arg(n));}
}
class SuperDriver{public static void main(String[]args)throws Exception{SuperOwner<Object> root=new SuperOwner<Object>();Object token=new Object();int rows=0;if(!SuperOwner.capture(null).toString().equals("null")||!SuperOwner.capture(token).toString().equals("value"))throw new AssertionError("anonymous");for(boolean fail:new boolean[]{true,false})for(Object value:new Object[]{null,token,"text"})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){SuperEffects.fail=fail;SuperEffects.trace="";SuperEffects.published=null;try{Object child=root.make(value,n);if(fail||child!=SuperEffects.published||!SuperEffects.trace.equals("APC"))throw new AssertionError("success");}catch(IllegalArgumentException e){if(!fail||e!=SuperEffects.failure||!SuperEffects.trace.equals("AP"))throw new AssertionError("failure",e);}Object child=SuperEffects.published;Class<?>c=child.getClass(),p=c.getSuperclass();java.lang.reflect.Field v=c.getDeclaredField("value"),cn=c.getDeclaredField("n"),seen=p.getDeclaredField("seen"),pn=p.getDeclaredField("parentN"),outer=p.getDeclaredField("observed");for(java.lang.reflect.Field f:new java.lang.reflect.Field[]{v,cn,seen,pn,outer})f.setAccessible(true);if(seen.get(child)!=value||pn.getLong(child)!=n||outer.get(child)!=root||v.get(child)!=(fail?null:value)||cn.getLong(child)!=(fail?0:n))throw new AssertionError("publication and callback state");rows++;}Class<?>c=Class.forName("SuperOwner$Child"),p=Class.forName("SuperOwner$Parent");java.lang.reflect.Constructor<?>ctor=c.getDeclaredConstructor(SuperOwner.class,Object.class,long.class),pc=p.getDeclaredConstructor(SuperOwner.class,Object.class,long.class);if(!java.lang.reflect.Modifier.isPrivate(ctor.getModifiers())||!java.lang.reflect.Modifier.isPrivate(pc.getModifiers())||c.getDeclaredConstructors().length!=2||p.getDeclaredConstructors().length!=2)throw new AssertionError("private bridge ABI");ctor.setAccessible(true);for(boolean fail:new boolean[]{true,false})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){SuperEffects.fail=fail;SuperEffects.trace="";SuperEffects.published=null;try{Object child=ctor.newInstance(null,token,n);if(fail||child!=SuperEffects.published||!SuperEffects.trace.equals("PC"))throw new AssertionError("null outer success");}catch(java.lang.reflect.InvocationTargetException e){if(!fail||e.getCause()!=SuperEffects.failure||!SuperEffects.trace.equals("P"))throw new AssertionError("null outer failure",e);}Object child=SuperEffects.published;java.lang.reflect.Field outer=p.getDeclaredField("observed"),seen=p.getDeclaredField("seen"),pn=p.getDeclaredField("parentN"),v=c.getDeclaredField("value"),cn=c.getDeclaredField("n");for(java.lang.reflect.Field f:new java.lang.reflect.Field[]{outer,seen,pn,v,cn})f.setAccessible(true);if(outer.get(child)!=null||seen.get(child)!=token||pn.getLong(child)!=n||v.get(child)!=(fail?null:token)||cn.getLong(child)!=(fail?0:n))throw new AssertionError("null enclosing callback state");rows++;}System.out.println(rows+":"+SuperEffects.trace);}}`

func TestNativeMemberPrivateSuperBridgeRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	for _, bounds := range []string{"normal", "empty-marker", "dollar-root", "number-bound", "rival-overload", "checked-exception", "major51", "empty-major51"} {
		fixture := nativeMemberPrivateSuperBridgeFixture
		rootName := "SuperOwner"
		wantOracle := "24:PC\n"
		if bounds == "empty-marker" || bounds == "empty-major51" {
			fixture = strings.Replace(fixture, ` static Object capture(final Object x){return new Object(){public String toString(){return x==null?"null":"value";}};}`, "", 1)
			fixture = strings.Replace(fixture, `if(!SuperOwner.capture(null).toString().equals("null")||!SuperOwner.capture(token).toString().equals("value"))throw new AssertionError("anonymous");`, "", 1)
		}
		if bounds == "dollar-root" {
			rootName = "Dollar$Super"
			fixture = strings.ReplaceAll(fixture, "SuperOwner", rootName)
		}
		if bounds == "number-bound" {
			fixture = strings.ReplaceAll(fixture, "SuperOwner<T>", "SuperOwner<T extends Number>")
			fixture = strings.ReplaceAll(fixture, "Parent<V>", "Parent<V extends Number>")
			fixture = strings.ReplaceAll(fixture, "Child<U> extends", "Child<U extends Number> extends")
			fixture = strings.ReplaceAll(fixture, "SuperOwner<Object>", "SuperOwner<Number>")
			fixture = strings.Replace(fixture, "Object token=new Object()", "Number token=Long.valueOf(-1)", 1)
			fixture = strings.Replace(fixture, `for(Object value:new Object[]{null,token,"text"})`, `for(Number value:new Number[]{null,token,Double.valueOf(-0.0)})`, 1)
			fixture = strings.ReplaceAll(fixture, "SuperOwner.class,Object.class,long.class", "SuperOwner.class,Number.class,long.class")
		}
		if bounds == "rival-overload" {
			fixture = strings.Replace(fixture, "final V seen;final long parentN;", "final V seen;final long parentN;final int selected;", 1)
			fixture = strings.Replace(fixture, `SuperEffects.trace+="P";seen=value;`, `this.selected=1;SuperEffects.trace+="P";seen=value;`, 1)
			fixture = strings.Replace(fixture, "abstract Object origin();}", `private Parent(String value,long n){this.selected=2;seen=null;parentN=n;SuperEffects.published=this;observed=origin();SuperEffects.trace+="S";}abstract Object origin();}`, 1)
			fixture = strings.Replace(fixture, "Object origin(){return SuperOwner.this;}", `private Child(String value,long n){super((String)value,n);this.value=null;this.n=n;}Object origin(){return SuperOwner.this;}`, 1)
			fixture = strings.Replace(fixture, " Child<T> make", ` Child<T> stringNull(long n){return new Child<T>((String)null,n);}Child<T> make`, 1)
			fixture = strings.Replace(fixture, "c.getDeclaredConstructors().length!=2||p.getDeclaredConstructors().length!=2", "c.getDeclaredConstructors().length!=4||p.getDeclaredConstructors().length!=4", 1)
			fixture = strings.Replace(fixture, "ctor.setAccessible(true);", `for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){Object child=root.stringNull(n);java.lang.reflect.Field selected=p.getDeclaredField("selected"),v=c.getDeclaredField("value"),pn=p.getDeclaredField("parentN");for(java.lang.reflect.Field f:new java.lang.reflect.Field[]{selected,v,pn})f.setAccessible(true);if(selected.getInt(child)!=2||v.get(child)!=null||pn.getLong(child)!=n)throw new AssertionError("rival String parent");rows++;}ctor.setAccessible(true);`, 1)
			wantOracle = "27:PC\n"
		}
		if bounds == "checked-exception" {
			fixture = strings.ReplaceAll(fixture, "IllegalArgumentException", "java.io.IOException")
			fixture = strings.Replace(fixture, "private Parent(V value,long n){", "private Parent(V value,long n)throws java.io.IOException{", 1)
			fixture = strings.Replace(fixture, "private Child(U value,long n){", "private Child(U value,long n)throws java.io.IOException{", 1)
			fixture = strings.Replace(fixture, "Child<T> make(T value,long n){", "Child<T> make(T value,long n)throws java.io.IOException{", 1)
		}
		debugModes := []string{"none", "source,lines,vars"}
		policies := []string{"normal", "no-source-rewrites", "no-core-cleanups"}
		for _, debug := range debugModes {
			t.Run(bounds+"/"+debug, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, fixture, debug)
				if bounds == "major51" || bounds == "empty-major51" {
					// This authored Java-8 instruction subset uses no Java-8-only
					// classfile features. Execute the major-51 original first;
					// the test does not require a second installed JDK.
					for n, raw := range files {
						obj, err := Parse(raw)
						if err != nil {
							t.Fatal(err)
						}
						obj.MajorVersion = 51
						files[n] = obj.Bytes()
					}
				}
				original := t.TempDir()
				for n, raw := range files {
					if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				oracle := t04RunJava(t, java, original, "SuperDriver")
				if oracle != wantOracle {
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
						for _, n := range []string{"SuperOwner$1", "SuperOwner$Parent", "SuperOwner$Child"} {
							n = strings.Replace(n, "SuperOwner", rootName, 1)
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
						if got := t04RunJava(t, java, output, "SuperDriver"); got != oracle {
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
