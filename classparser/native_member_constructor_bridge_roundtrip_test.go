package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMemberPrivateBridgeFixture = `class BridgeEffects {static String trace="";static Object published;static boolean fail;static final IllegalArgumentException error=new IllegalArgumentException("same");static long arg(long n){trace+="A";return n;}}
class BridgeOwner<T>{
 static Object capture(final Object x){return new Object(){public String toString(){return x==null?"null":"value";}};}
 static class Child<U>{final U value;final long n;private Child(U value,long n){BridgeEffects.trace+="P";BridgeEffects.published=this;if(BridgeEffects.fail)throw BridgeEffects.error;this.value=value;this.n=n;BridgeEffects.trace+="C";}}
 Child<T> make(T value,long n){return new Child<T>(value,BridgeEffects.arg(n));}
}
class BridgeDriver{public static void main(String[]args)throws Exception{BridgeOwner<Object> o=new BridgeOwner<Object>();Object token=new Object();int rows=0;if(!BridgeOwner.capture(null).toString().equals("null")||!BridgeOwner.capture(token).toString().equals("value"))throw new AssertionError("anonymous capture");for(boolean fail:new boolean[]{true,false})for(Object value:new Object[]{null,token,"text"})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){BridgeEffects.trace="";BridgeEffects.published=null;BridgeEffects.fail=fail;try{BridgeOwner.Child<Object> c=o.make(value,n);if(fail||c.value!=value||c.n!=n||BridgeEffects.published!=c||!BridgeEffects.trace.equals("APC"))throw new AssertionError("state");}catch(IllegalArgumentException e){BridgeOwner.Child<?> c=(BridgeOwner.Child<?>)BridgeEffects.published;if(!fail||e!=BridgeEffects.error||c==null||c.value!=null||c.n!=0||!BridgeEffects.trace.equals("AP"))throw new AssertionError("failure state");}rows++;}java.lang.reflect.Constructor<?> ctor=BridgeOwner.Child.class.getDeclaredConstructor(Object.class,long.class);if(!java.lang.reflect.Modifier.isPrivate(ctor.getModifiers())||BridgeOwner.Child.class.getDeclaredConstructors().length!=2)throw new AssertionError("private constructor shape");System.out.println(rows+":"+BridgeEffects.trace);}}`

func TestNativeMemberPrivateConstructorBridgeRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	for _, bounds := range []string{"unbounded", "Number", "varargs", "DollarRoot", "emptyMarker", "emptyMarkerNumber", "emptyMarkerVarargs", "emptyMarkerDollar", "major51", "emptyMarker51", "nonstatic", "nonstaticEmpty", "nonstaticCallback", "nonstaticOverload"} {
		fixture := nativeMemberPrivateBridgeFixture
		rootName := "BridgeOwner"
		debugModes := []string{"none", "source,lines,vars"}
		policies := []string{"normal", "no-source-rewrites", "no-core-cleanups"}
		if bounds == "Number" || bounds == "emptyMarkerNumber" {
			fixture = strings.ReplaceAll(fixture, "BridgeOwner<T>", "BridgeOwner<T extends Number>")
			fixture = strings.ReplaceAll(fixture, "BridgeOwner<Object>", "BridgeOwner<Number>")
			fixture = strings.ReplaceAll(fixture, "Child<Object>", "Child<Number>")
			fixture = strings.ReplaceAll(fixture, "Object token=new Object()", "Number token=Long.valueOf(7)")
			fixture = strings.ReplaceAll(fixture, "for(Object value:new Object[]{null,token,\"text\"})", "for(Number value:new Number[]{null,token,Integer.valueOf(3)})")
		}
		if bounds == "varargs" || bounds == "emptyMarkerVarargs" {
			fixture = strings.ReplaceAll(fixture, "private Child(U value,long n)", "private Child(U value,long n,Object...tail)")
			fixture = strings.ReplaceAll(fixture, "new Child<T>(value,BridgeEffects.arg(n))", "new Child<T>(value,BridgeEffects.arg(n),(Object[])null)")
			fixture = strings.ReplaceAll(fixture, "getDeclaredConstructor(Object.class,long.class)", "getDeclaredConstructor(Object.class,long.class,Object[].class)")
		}
		if strings.HasPrefix(bounds, "emptyMarker") {
			fixture = strings.Replace(fixture, `static Object capture(final Object x){return new Object(){public String toString(){return x==null?"null":"value";}};}`, "", 1)
			fixture = strings.Replace(fixture, `if(!BridgeOwner.capture(null).toString().equals("null")||!BridgeOwner.capture(token).toString().equals("value"))throw new AssertionError("anonymous capture");`, "", 1)
		}
		if strings.HasPrefix(bounds, "emptyMarker") {
			fixture = strings.Replace(fixture, `System.out.println(rows+":"+BridgeEffects.trace);`, `Class<?>marker=Class.forName("BridgeOwner$1");if(!marker.isSynthetic()||marker.getDeclaredFields().length!=0||marker.getDeclaredMethods().length!=0||marker.getDeclaredConstructors().length!=0||marker.getEnclosingClass()!=BridgeOwner.class||marker.getEnclosingMethod()!=null)throw new AssertionError("empty marker role");System.out.println(rows+":"+BridgeEffects.trace);`, 1)
		}
		if bounds == "nonstatic" || bounds == "nonstaticEmpty" || bounds == "nonstaticCallback" || bounds == "nonstaticOverload" {
			fixture = strings.Replace(fixture, "static class Child<U>", "class Child<U>", 1)
			fixture = strings.ReplaceAll(fixture, "BridgeOwner.Child<Object>", "BridgeOwner<Object>.Child<Object>")
			fixture = strings.ReplaceAll(fixture, "BridgeOwner.Child<?>", "BridgeOwner<?>.Child<?>")
			fixture = strings.Replace(fixture, "getDeclaredConstructor(Object.class,long.class)", "getDeclaredConstructor(BridgeOwner.class,Object.class,long.class)", 1)
			if bounds == "nonstaticEmpty" {
				fixture = strings.Replace(fixture, `static Object capture(final Object x){return new Object(){public String toString(){return x==null?"null":"value";}};}`, "", 1)
				fixture = strings.Replace(fixture, `if(!BridgeOwner.capture(null).toString().equals("null")||!BridgeOwner.capture(token).toString().equals("value"))throw new AssertionError("anonymous capture");`, "", 1)
			}
		}
		if bounds == "nonstaticCallback" {
			fixture += ` abstract class BridgeParent{final Object observed;final long parentNumber;BridgeParent(long n){BridgeEffects.trace+="P";BridgeEffects.published=this;observed=origin();parentNumber=n;if(BridgeEffects.fail)throw BridgeEffects.error;}abstract Object origin();}`
			fixture = strings.Replace(fixture, "class Child<U>{", "class Child<U> extends BridgeParent{", 1)
			fixture = strings.Replace(fixture, `private Child(U value,long n){BridgeEffects.trace+="P";BridgeEffects.published=this;if(BridgeEffects.fail)throw BridgeEffects.error;`, `private Child(U value,long n){super(n);`, 1)
			fixture = strings.Replace(fixture, `BridgeEffects.trace+="C";}}`, `BridgeEffects.trace+="C";}Object origin(){return BridgeOwner.this;}}`, 1)
			fixture = strings.Replace(fixture, `if(fail||c.value!=value`, `if(fail||c.observed!=o||c.origin()!=o||c.parentNumber!=n||c.value!=value`, 1)
			fixture = strings.Replace(fixture, `if(!fail||e!=BridgeEffects.error||c==null||c.value!=null`, `if(!fail||e!=BridgeEffects.error||c==null||c.observed!=o||c.origin()!=o||c.parentNumber!=n||c.value!=null`, 1)
		}
		if bounds == "nonstaticCallback" {
			fixture = strings.Replace(fixture, `System.out.println(rows+":"+BridgeEffects.trace);`, `ctor.setAccessible(true);for(boolean fail:new boolean[]{true,false})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){BridgeEffects.trace="";BridgeEffects.published=null;BridgeEffects.fail=fail;try{BridgeOwner<?>.Child<?> c=(BridgeOwner<?>.Child<?>)ctor.newInstance(null,token,n);if(fail||c.observed!=null||c.origin()!=null||c.parentNumber!=n||c.value!=token||c.n!=n||!BridgeEffects.trace.equals("PC"))throw new AssertionError("null outer state");}catch(java.lang.reflect.InvocationTargetException e){BridgeOwner<?>.Child<?>c=(BridgeOwner<?>.Child<?>)BridgeEffects.published;if(!fail||e.getCause()!=BridgeEffects.error||c==null||c.observed!=null||c.origin()!=null||c.parentNumber!=n||c.value!=null||c.n!=0||!BridgeEffects.trace.equals("P"))throw new AssertionError("null outer failure");}rows++;}System.out.println(rows+":"+BridgeEffects.trace);`, 1)
		}
		if bounds == "nonstaticOverload" {
			fixture = strings.Replace(fixture, "final U value;final long n;", "final U value;final long n;final int selected;", 1)
			fixture = strings.Replace(fixture, "this.value=value;this.n=n;", "this.selected=1;this.value=value;this.n=n;", 1)
			fixture = strings.Replace(fixture, `BridgeEffects.trace+="C";}}`, `BridgeEffects.trace+="C";}private Child(String value,long n){this.selected=2;this.value=null;this.n=n;}}`, 1)
			fixture = strings.Replace(fixture, `Child<T> make(T value,long n)`, `Child<T> nothing(long n){return new Child<T>((String)null,n);}Child<T> make(T value,long n)`, 1)
			fixture = strings.Replace(fixture, `if(fail||c.value!=value`, `if(fail||c.selected!=1||c.value!=value`, 1)
			fixture = strings.Replace(fixture, `c==null||c.value!=null`, `c==null||c.selected!=0||c.value!=null`, 1)
			fixture = strings.Replace(fixture, `getDeclaredConstructors().length!=2`, `getDeclaredConstructors().length!=4`, 1)
			fixture = strings.Replace(fixture, `System.out.println(rows+":"+BridgeEffects.trace);`, `for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){BridgeOwner<Object>.Child<Object> c=o.nothing(n);if(c.selected!=2||c.value!=null||c.n!=n)throw new AssertionError("original String constructor");rows++;}System.out.println(rows+":"+BridgeEffects.trace);`, 1)
		}
		if bounds == "DollarRoot" || bounds == "emptyMarkerDollar" {
			rootName = "Bridge$Owner"
			fixture = strings.ReplaceAll(fixture, "BridgeOwner", rootName)
			debugModes = []string{"none"}
			policies = []string{"normal"}
		}
		for _, debug := range debugModes {
			t.Run(bounds+"/"+debug, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, fixture, debug)
				if bounds == "major51" || bounds == "emptyMarker51" {
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
				oracle := t04RunJava(t, java, original, "BridgeDriver")
				wantOracle := "18:APC\n"
				if bounds == "nonstaticCallback" {
					wantOracle = "24:PC\n"
				}
				if bounds == "nonstaticOverload" {
					wantOracle = "21:APC\n"
				}
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
						for _, n := range []string{"BridgeOwner$1", "BridgeOwner$Child"} {
							n = strings.Replace(n, "BridgeOwner", rootName, 1)
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
						if got := t04RunJava(t, java, output, "BridgeDriver"); got != oracle {
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
