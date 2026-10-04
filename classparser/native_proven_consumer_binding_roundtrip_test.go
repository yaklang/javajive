package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// An unavailable consumer declaration must not borrow a formal from a separate
// field. The original erased call accepts arbitrary objects, and the oracle
// observes their identity. The miss is deliberately tested in compatibility
// mode too, where the old text recovery used to add the unrelated R cast.
func TestConsumerBindingMissKeepsErasedArgumentIdentity(t *testing.T) {
	const source = `interface ValueObserver<R>{void onNext(R n);}
class RawConsumer {Object seen;void onNext(Object n){seen=n;}}
class UnrelatedProgram<T,R> {final ValueObserver<? super R> unrelated=null;void emit(RawConsumer box,Object n){box.onNext(n);}}
public class ConsumerMissDriver {public static void main(String[]args){UnrelatedProgram<Object,String> p=new UnrelatedProgram<>();RawConsumer box=new RawConsumer();for(Object n:new Object[]{null,"text",Long.valueOf(Long.MIN_VALUE),new Object()}){p.emit(box,n);if(box.seen!=n)throw new AssertionError("erased argument identity");}System.out.println(4);}}`
	roundTripGenericFlowUnitsClasspath(t, "ConsumerMissDriver", source, func(name string) bool { return name != "RawConsumer" }, []string{"UnrelatedProgram"}, true, Precision, Compatibility, "legacy")
}

func TestNativeConsumerBindingIgnoresUnrelatedSameScopeVariable(t *testing.T) {
	javac, java := t04Tools(t)
	const fixture = `package mixed.scope;
interface ValueObserver<T>{void onNext(T n);}
class MixedOwner{public static abstract class Box<T> implements ValueObserver<T>{Object seen;}public static final class Source<T,R> implements ValueObserver<T>{final Box<T> box;final ValueObserver<? super R> unrelated=null;Source(Box<T> b){box=b;}public void onNext(T n){box.onNext(n);}}public static final class Target<T,R> implements ValueObserver<R>{final ValueObserver<? super R> sink;Target(ValueObserver<? super R> b){sink=b;}public void onNext(R n){sink.onNext(n);}}}
class MixedBox<A> extends MixedOwner.Box<A>{public void onNext(A n){seen=n;}}
class MixedExternal{static <T,R> MixedOwner.Source<T,R> source(MixedOwner.Box<T> b){return new MixedOwner.Source<T,R>(b);}static <T,R> MixedOwner.Target<T,R> target(ValueObserver<R> s){return new MixedOwner.Target<T,R>(s);}}
public class MixedDriver{public static void main(String[]args){for(Object n:new Object[]{null,"text",Long.valueOf(Long.MIN_VALUE),Long.valueOf(Long.MAX_VALUE)}){MixedOwner.Box<Object> a=new MixedBox<>();MixedOwner.Box<String> b=new MixedBox<>();MixedOwner.Source<Object,String> source=MixedExternal.source(a);MixedOwner.Target<Object,String> target=MixedExternal.target(b);source.onNext(n);target.onNext("marker");if(a.seen!=n||b.seen!="marker")throw new AssertionError("peer generic binding");}System.out.println(4);}}
`
	for _, variant := range []string{"same scope", "method formal shadow", "renamed formals"} {
		source := fixture
		if variant == "method formal shadow" {
			source = strings.Replace(source, "public void onNext(T n){box.onNext(n);}", "public void onNext(T n){box.onNext(n);}public <R> void again(T n,R token){box.onNext(n);}", 1)
			source = strings.Replace(source, "source.onNext(n);", "source.onNext(n);source.again(n,new Object());", 1)
		}
		if variant == "renamed formals" {
			source = strings.NewReplacer("<T,R>", "<Element,Result>", "<T>", "<Element>", "<R>", "<Result>", "super R", "super Result", "(T n)", "(Element n)", "(R n)", "(Result n)").Replace(source)
		}
		for _, debug := range []string{"-g", "-g:none"} {
			t.Run(variant+"/"+debug, func(t *testing.T) {
				original := t.TempDir()
				file := filepath.Join(original, "MixedDriver.java")
				if e := os.WriteFile(file, []byte(source), 0600); e != nil {
					t.Fatal(e)
				}
				if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, file).CombinedOutput(); e != nil {
					t.Fatalf("original %v %s", e, out)
				}
				oracle := t04RunJava(t, java, original, "mixed.scope.MixedDriver")
				files := map[string][]byte{}
				if e := filepath.Walk(original, func(p string, info os.FileInfo, e error) error {
					if e != nil {
						return e
					}
					if !info.IsDir() && strings.HasSuffix(p, ".class") {
						r, e := os.ReadFile(p)
						if e != nil {
							return e
						}
						rel, e := filepath.Rel(original, p)
						if e != nil {
							return e
						}
						files[filepath.ToSlash(rel)] = r
					}
					return nil
				}); e != nil {
					t.Fatal(e)
				}
				for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
					t.Run(mode, func(t *testing.T) {
						if mode == "no-source-rewrites" {
							t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
						}
						if mode == "no-core-cleanups" {
							t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
						}
						z := nativeArchive(t, files)
						for _, n := range []string{"mixed/scope/MixedOwner$Box.class", "mixed/scope/MixedOwner$Source.class", "mixed/scope/MixedOwner$Target.class"} {
							s, e := z.ReadFile(n)
							if e != nil || !strings.Contains(string(s), "body owned by") {
								t.Fatalf("unowned %s %v %s", n, e, s)
							}
						}
						output := t.TempDir()
						for n, b := range files {
							if strings.HasPrefix(n, "mixed/scope/MixedOwner") || strings.HasPrefix(n, "mixed/scope/MixedExternal") {
								continue
							}
							p := filepath.Join(output, filepath.FromSlash(n))
							if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
								t.Fatal(e)
							}
							if e := os.WriteFile(p, b, 0600); e != nil {
								t.Fatal(e)
							}
						}
						paths := []string{}
						for _, n := range []string{"mixed/scope/MixedOwner", "mixed/scope/MixedExternal"} {
							src, e := z.ReadFile(n + ".class")
							if e != nil || strings.Contains(string(src), DecompileStubMarker) {
								t.Fatalf("source %v %s", e, src)
							}
							p := filepath.Join(output, filepath.FromSlash(n)+".java")
							if e := os.WriteFile(p, src, 0600); e != nil {
								t.Fatal(e)
							}
							paths = append(paths, p)
						}
						if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", output, "-d", output}, paths...)...).CombinedOutput(); e != nil {
							t.Fatalf("rebuilt %v %s", e, out)
						}
						for _, n := range []string{"mixed/scope/MixedOwner", "mixed/scope/MixedOwner$Box", "mixed/scope/MixedOwner$Source", "mixed/scope/MixedOwner$Target"} {
							b, e := os.ReadFile(filepath.Join(output, filepath.FromSlash(n)+".class"))
							if e != nil {
								t.Fatal(e)
							}
							if want, got := nativeBinaryShape(t, files[n+".class"]), nativeBinaryShape(t, b); want != got {
								t.Fatalf("ABI %s\n%s\n!=\n%s", n, want, got)
							}
						}
						if got := t04RunJava(t, java, output, "mixed.scope.MixedDriver"); got != oracle {
							t.Fatalf("%q != %q", got, oracle)
						}
					})
				}
			})
		}
	}
}
