package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const independentPrivateScopeFixture = `import java.util.function.IntUnaryOperator;abstract class PrivateScopeParent{final int seen;PrivateScopeParent(){seen=read();}abstract int read();}
class ScopeOwner{static class Capsule{PRIVATE final int internal;final int base;Capsule(int base,int hidden){this.base=base;this.internal=hidden;}int saved(){return internal;}final class Worker extends PrivateScopeParent{int read(){return Capsule.this.base;}}PrivateScopeParent make(){return new Worker();}}static IntUnaryOperator unrelated(final String text){return value->value+text.length();}}
class PrivateScopeDriver{public static void main(String[]args)throws Exception{int rows=0;for(int base:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int hidden:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){ScopeOwner.Capsule owner=new ScopeOwner.Capsule(base,hidden);PrivateScopeParent worker=owner.make();if(worker.seen!=base||worker.read()!=base||owner.saved()!=hidden||worker.getClass().getDeclaringClass()!=owner.getClass())throw new AssertionError("early capture or own field");if(java.lang.reflect.Modifier.isPrivate(ScopeOwner.Capsule.class.getDeclaredField("internal").getModifiers())!=EXPECTPRIVATE)throw new AssertionError("field visibility");if(ScopeOwner.unrelated("abc").applyAsInt(4)!=7)throw new AssertionError("unchanged external caller");rows++;}System.out.println("25:independent:private:component");}}
`

func TestAdversarialIndependentPrivateScopeRetainsNamedCaptureComponent(t *testing.T) {
	testIndependentPrivateScopeComponent(t, independentPrivateScopeFixture, "25:independent:private:component\n")
}

func testIndependentPrivateScopeComponent(t *testing.T, source, want string) {
	t.Helper()

	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("original native8 metadata compiler")
	}
	for _, private := range []bool{false, true} {
		label := "package field"
		visibility := ""
		expect := "false"
		if private {
			label = "private field"
			visibility = "private"
			expect = "true"
		}
		t.Run(label, func(t *testing.T) {
			for _, root := range []string{"ScopeOwner", "RenamedPrivateScope"} {
				t.Run(root, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							fixture := strings.NewReplacer("ScopeOwner", root, "PRIVATE", visibility, "EXPECTPRIVATE", expect).Replace(source)
							files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, root, debug, true)
							original := t.TempDir()
							for name, raw := range files {
								if e := os.WriteFile(filepath.Join(original, name), raw, 0600); e != nil {
									t.Fatal(e)
								}
							}
							originalOutput := t04RunJava(t, java, original, "PrivateScopeDriver")
							if originalOutput != want {
								t.Fatal("original oracle", originalOutput)
							}
							z := nativeArchive(t, files)
							defer z.Close()
							entry := root + "$Capsule.class"
							raw, err := z.ReadFile(entry)
							if err != nil {
								t.Fatal(err)
							}
							text := string(raw)
							if strings.Contains(text, DecompileStubMarker) || !strings.Contains(text, "class Worker") {
								t.Fatalf("independent component was not recovered\n%s", text)
							}
							out := t.TempDir()
							owned := map[string][]byte{}
							for name, data := range files {
								if name == entry || strings.HasPrefix(name, root+"$Capsule$") {
									owned[name] = data
									continue
								}
								if e := os.WriteFile(filepath.Join(out, name), data, 0600); e != nil {
									t.Fatal(e)
								}
							}
							if len(owned) != 2 {
								t.Fatal("original component inventory", len(owned))
							}
							src := filepath.Join(out, root+"$Capsule.java")
							if e := os.WriteFile(src, raw, 0600); e != nil {
								t.Fatal(e)
							}
							if log, e := exec.Command(modern, "-proc:none", "--release", "8", "-parameters", "-g:"+debug, "-cp", out, "-d", out, src).CombinedOutput(); e != nil {
								t.Fatalf("compile without original component: %v\n%s\n%s", e, log, text)
							}
							if got := t04RunJava(t, java, out, "PrivateScopeDriver"); got != want {
								t.Fatalf("unchanged original caller %q wanted%q\n%s", got, want, text)
							}
							shape := nativeStaticIndependentRootRepresentationShape(root)
							for name, before := range owned {
								after, e := os.ReadFile(filepath.Join(out, name))
								if e != nil {
									t.Fatal(e)
								}
								shape(t, name, before, after)
							}
						})
					}
				})
			}
		})
	}
}
