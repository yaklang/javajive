package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMemberRawBoundedFactoryFixture = `interface BridgeFactory<X>{X get();}
class BridgeOwner {
 static int created;
 static class Box<T extends java.io.Serializable> implements BridgeFactory<T>{final T value;final java.util.function.Function<java.math.BigDecimal,T> function;private Box(T value){created++;this.value=value;this.function=null;}private Box(java.util.function.Function<java.math.BigDecimal,T> function){created++;this.value=null;this.function=function;}public T get(){return function==null?value:function.apply(new java.math.BigDecimal("1.25"));}}
 @SuppressWarnings("unchecked") static <T> BridgeFactory<T> make(T value){if(value instanceof java.io.Serializable)return (BridgeFactory<T>)(BridgeFactory<?>)new Box<java.io.Serializable>((java.io.Serializable)value);return null;}
 static BridgeFactory<String> functional(){return new Box<String>(java.math.BigDecimal::toPlainString);}
}
class BridgeDriver{public static void main(String[]args)throws Exception{int rows=0;Object token=new Object();for(Object value:new Object[]{null,token,"text",Long.valueOf(Long.MIN_VALUE),Double.valueOf(-0.0)}){BridgeFactory<Object> f=BridgeOwner.make(value);if(value instanceof java.io.Serializable){if(f==null||f.get()!=value)throw new AssertionError("identity");rows++;}else if(f!=null)throw new AssertionError("nonserializable");}BridgeFactory<String> f=BridgeOwner.functional();if(!f.get().equals("1.25")||BridgeOwner.created!=4)throw new AssertionError("functional constructor");java.lang.reflect.Constructor<?>a=BridgeOwner.Box.class.getDeclaredConstructor(java.io.Serializable.class),b=BridgeOwner.Box.class.getDeclaredConstructor(java.util.function.Function.class);if(!java.lang.reflect.Modifier.isPrivate(a.getModifiers())||!java.lang.reflect.Modifier.isPrivate(b.getModifiers())||BridgeOwner.Box.class.getDeclaredConstructors().length!=4)throw new AssertionError("ABI");System.out.println((rows+1)+":"+BridgeOwner.created+":"+f.get());}}`

func TestNativeMemberRawBoundedFactoryRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	for _, bounds := range []string{"static", "nonstatic", "parameterized-outer", "dollar-root", "number-bound"} {
		fixture := nativeMemberRawBoundedFactoryFixture
		rootName := "BridgeOwner"
		if bounds == "nonstatic" || bounds == "parameterized-outer" {
			fixture = strings.Replace(fixture, " static class Box<", " class Box<", 1)
			fixture = strings.Replace(fixture, " static <T> BridgeFactory<T> make", " <T> BridgeFactory<T> make", 1)
			fixture = strings.Replace(fixture, " static BridgeFactory<String> functional", " BridgeFactory<String> functional", 1)
			fixture = strings.ReplaceAll(fixture, "BridgeOwner.make", "root.make")
			fixture = strings.ReplaceAll(fixture, "BridgeOwner.functional", "root.functional")
			if bounds == "parameterized-outer" {
				fixture = strings.Replace(fixture, "class BridgeOwner {", "class BridgeOwner<T> {", 1)
				fixture = strings.Replace(fixture, "int rows=0;", "BridgeOwner<String> root=new BridgeOwner<>();int rows=0;", 1)
			} else {
				fixture = strings.Replace(fixture, "int rows=0;", "BridgeOwner root=new BridgeOwner();int rows=0;", 1)
			}
			fixture = strings.Replace(fixture, "getDeclaredConstructor(java.io.Serializable.class)", "getDeclaredConstructor(BridgeOwner.class,java.io.Serializable.class)", 1)
			fixture = strings.Replace(fixture, "getDeclaredConstructor(java.util.function.Function.class)", "getDeclaredConstructor(BridgeOwner.class,java.util.function.Function.class)", 1)
		}
		if bounds == "dollar-root" {
			rootName = "Dollar$Bridge"
			fixture = strings.ReplaceAll(fixture, "BridgeOwner", rootName)
		}
		wantOracle := "4:4:1.25\n"
		if bounds == "number-bound" {
			fixture = strings.ReplaceAll(fixture, "java.io.Serializable", "java.lang.Number")
			fixture = strings.Replace(fixture, "BridgeFactory<String> functional(){return new Box<String>(java.math.BigDecimal::toPlainString);}", "BridgeFactory<Number> functional(){return new Box<Number>(java.math.BigDecimal::longValue);}", 1)
			fixture = strings.Replace(fixture, "BridgeFactory<String> f=BridgeOwner.functional();if(!f.get().equals(\"1.25\")||BridgeOwner.created!=4)", "BridgeFactory<Number> f=BridgeOwner.functional();if(f.get().longValue()!=1||BridgeOwner.created!=3)", 1)
			wantOracle = "3:3:1\n"
		}
		debugModes := []string{"none", "source,lines,vars"}
		policies := []string{"normal", "no-source-rewrites", "no-core-cleanups"}
		for _, debug := range debugModes {
			t.Run(bounds+"/"+debug, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, fixture, debug)
				original := t.TempDir()
				for n, raw := range files {
					if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				oracle := t04RunJava(t, java, original, "BridgeDriver")
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
						for _, n := range []string{"BridgeOwner$1", "BridgeOwner$Box"} {
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
