package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const independentSourceCycleFixture = `import java.util.function.IntUnaryOperator;abstract class CycleParent{final int seen;CycleParent(){seen=read();}abstract int read();}
class CyclicA{static class Capsule{final int value;Capsule(int value){this.value=value;}int get(){return value;}final class Worker extends CycleParent{final CyclicB.Capsule peer;Worker(CyclicB.Capsule peer){this.peer=peer;}int read(){return Capsule.this.value;}int sum(){return Capsule.this.value+peer.get();}}Worker make(CyclicB.Capsule peer){return new Worker(peer);}}static IntUnaryOperator unrelated(final String text){return value->value+text.length();}}
class CyclicB{static class Capsule{final int value;Capsule(int value){this.value=value;}int get(){return value;}final class Worker extends CycleParent{final CyclicA.Capsule peer;Worker(CyclicA.Capsule peer){this.peer=peer;}int read(){return Capsule.this.value;}int sum(){return Capsule.this.value+peer.get();}}Worker make(CyclicA.Capsule peer){return new Worker(peer);}}static IntUnaryOperator unrelated(final String text){return value->value+text.length();}}
class CycleDriver{public static void main(String[]args){int rows=0;for(int left:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int right:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){CyclicA.Capsule a=new CyclicA.Capsule(left);CyclicB.Capsule b=new CyclicB.Capsule(right);CyclicA.Capsule.Worker x=a.make(b);CyclicB.Capsule.Worker y=b.make(a);int expected=java.math.BigInteger.valueOf(left).add(java.math.BigInteger.valueOf(right)).intValue();if(x.seen!=left||y.seen!=right||x.sum()!=expected||y.sum()!=expected||x.peer!=b||y.peer!=a||x.getClass().getDeclaringClass()!=a.getClass()||y.getClass().getDeclaringClass()!=b.getClass())throw new AssertionError("mutual declarations or early capture");if(CyclicA.unrelated("abc").applyAsInt(4)!=7||CyclicB.unrelated("xy").applyAsInt(4)!=6)throw new AssertionError("original outer callers");rows++;}System.out.println("25:independent:mutual:scopes");}}
`

func TestAdversarialIndependentStaticScopesKeepMutualDeclarationBinding(t *testing.T) {
	const source = independentSourceCycleFixture
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("original native8 compiler")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, source, "CyclicA", debug, true)
			original := t.TempDir()
			for name, data := range files {
				if e := os.WriteFile(filepath.Join(original, name), data, 0600); e != nil {
					t.Fatal(e)
				}
			}
			want := t04RunJava(t, java, original, "CycleDriver")
			if want != "25:independent:mutual:scopes\n" {
				t.Fatal("original oracle", want)
			}
			for _, reverse := range []bool{false, true} {
				label := "forward"
				if reverse {
					label = "reverse"
				}
				t.Run(label, func(t *testing.T) {
					z := nativeArchive(t, files)
					defer z.Close()
					roots := []string{"CyclicA", "CyclicB"}
					if reverse {
						roots[0], roots[1] = roots[1], roots[0]
					}
					sources := map[string][]byte{}
					for _, root := range roots {
						raw, e := z.ReadFile(root + "$Capsule.class")
						if e != nil {
							t.Fatal(e)
						}
						if strings.Contains(string(raw), DecompileStubMarker) || !strings.Contains(string(raw), "class Worker") {
							t.Fatalf("scope not recovered %s\n%s", root, raw)
						}
						sources[root+"$Capsule.java"] = raw
					}
					out := t.TempDir()
					owned := map[string][]byte{}
					for name, data := range files {
						if strings.HasPrefix(name, "CyclicA$Capsule") || strings.HasPrefix(name, "CyclicB$Capsule") {
							owned[name] = data
							continue
						}
						if e := os.WriteFile(filepath.Join(out, name), data, 0600); e != nil {
							t.Fatal(e)
						}
					}
					if len(owned) != 4 {
						t.Fatal("original component set", len(owned))
					}
					args := []string{"-proc:none", "--release", "8", "-parameters", "-g:" + debug, "-cp", out, "-d", out}
					for _, root := range []string{"CyclicA", "CyclicB"} {
						name := root + "$Capsule.java"
						p := filepath.Join(out, name)
						if e := os.WriteFile(p, sources[name], 0600); e != nil {
							t.Fatal(e)
						}
						args = append(args, p)
					}
					if log, e := exec.Command(modern, args...).CombinedOutput(); e != nil {
						t.Fatalf("compile four rebuilt components without original fallback: %v\n%s\n%s\n%s", e, log, sources["CyclicA$Capsule.java"], sources["CyclicB$Capsule.java"])
					}
					if got := t04RunJava(t, java, out, "CycleDriver"); got != want {
						t.Fatalf("same binary original caller %q wanted%q", got, want)
					}
					for name, before := range owned {
						after, e := os.ReadFile(filepath.Join(out, name))
						if e != nil {
							t.Fatal(e)
						}
						root := "CyclicA"
						if strings.HasPrefix(name, "CyclicB") {
							root = "CyclicB"
						}
						nativeStaticIndependentRootRepresentationShape(root)(t, name, before, after)
					}
				})
			}
		})
	}
}
