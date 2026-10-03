package javaclassparser

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A fixed generic ancestor changes the Java source formal while retaining the
// erased JVM descriptor. The original helper records which overload executed.
const inheritedBindingFormalFixture = `
package bindingp;
class BindingEntry {final int id;BindingEntry(int id){this.id=id;}}
class BindingItem extends BindingEntry {BindingItem(int id){super(id);}}
class BindingPool<E extends BindingEntry> {
 String trace="";Object seen;
 public void release(E e,boolean reusable){seen=e;trace+="base:"+(e==null?-1:e.id)+":"+reusable+";";}
}
class BindingMid<T extends BindingEntry> extends BindingPool<T> {}
class BindingFixed extends BindingMid<BindingItem> {}
class BindingCompeting extends BindingMid<BindingItem> {public void release(String e,boolean reusable){trace+="wrong;";}}
class BindingProgram {
 static void direct(BindingFixed pool,BindingItem e,boolean reusable){pool.release(e,reusable);}
 static void competing(BindingCompeting pool,BindingItem e,boolean reusable){pool.release(e,reusable);}
 static void erased(BindingCompeting pool,BindingEntry e){((BindingPool)pool).release(e,true);}
}
class BindingFormalOracle {
 static void run(){BindingItem token=new BindingItem(17);
  for(BindingItem e:new BindingItem[]{null,token})for(boolean reusable:new boolean[]{false,true}){
   BindingFixed p=new BindingFixed();BindingProgram.direct(p,e,reusable);if(p.seen!=e)throw new AssertionError("identity");System.out.println(p.trace);
   BindingCompeting q=new BindingCompeting();BindingProgram.competing(q,e,reusable);if(q.seen!=e||q.trace.contains("wrong"))throw new AssertionError("binding");System.out.println(q.trace);
  }
  BindingCompeting p=new BindingCompeting();BindingEntry other=new BindingEntry(23);BindingProgram.erased(p,other);if(p.seen!=other)throw new AssertionError("no added check");System.out.println(p.trace);
 }
}
public class BindingFormalDriver {public static void main(String[]args){BindingFormalOracle.run();}}
`

func TestAdversarialInheritedConcreteFormalKeepsErasedMemberBinding(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "bindingp.BindingFormalDriver", inheritedBindingFormalFixture, nil, []string{"bindingp.BindingProgram"}, true, Precision, Compatibility, "legacy")
}

// The external declarations must inform type substitution without becoming
// archive-owned flattened output classes. This is the actual historical JAR
// API path, distinct from the all-siblings control above.
func TestAdversarialDependencySignatureBindsInheritedConcreteFormal(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "BindingFormalDriver.java")
			if err := os.WriteFile(file, []byte(inheritedBindingFormalFixture), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, file).CombinedOutput(); err != nil {
				t.Fatalf("original: %v\n%s", err, out)
			}
			want := t04RunJava(t, java, dir, "bindingp.BindingFormalDriver")
			classes := classMapFromDir(t, dir)
			const owner = "bindingp/BindingProgram"
			var jar bytes.Buffer
			zw := zip.NewWriter(&jar)
			entry, err := zw.Create(owner + ".class")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = entry.Write(classes[owner]); err != nil {
				t.Fatal(err)
			}
			if err = zw.Close(); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "target.jar")
			if err = os.WriteFile(target, jar.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			archive, err := NewJarFSFromLocalWithResolver(target, func(name string) ([]byte, bool) {
				if name == owner {
					t.Fatal("target delegated to dependency")
				}
				return resolverFromClasses(classes)(name)
			})
			if err != nil {
				t.Fatal(err)
			}
			defer archive.Close()
			source, err := archive.ReadFile(owner + ".class")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(source), DecompileStubMarker) {
				t.Fatalf("complete declaration stubbed:\n%s", source)
			}
			// The oracle/driver and all helper implementations remain original, but the
			// selected target is absent from both candidate classpaths: no fallback.
			if err = os.Remove(filepath.Join(dir, "bindingp", "BindingProgram.class")); err != nil {
				t.Fatal(err)
			}
			rebuilt := t.TempDir()
			file = filepath.Join(rebuilt, "BindingProgram.java")
			if err = os.WriteFile(file, source, 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, file).CombinedOutput(); err != nil {
				t.Fatalf("rebuild: %v\n%s\n%s", err, out, source)
			}
			if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "bindingp.BindingFormalDriver"); got != want {
				t.Fatalf("binding/identity: got %q want %q\n%s", got, want, source)
			}
		})
	}
}

func TestAdversarialDeclarationSignatureProviderIdentity(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "BindingFormalDriver.java")
	if err := os.WriteFile(file, []byte(inheritedBindingFormalFixture), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("original: %v\n%s", err, out)
	}
	classes := classMapFromDir(t, dir)
	obj, err := Parse(classes["bindingp/BindingProgram"])
	if err != nil {
		t.Fatal(err)
	}
	ctx := &class_context.ClassContext{}
	dumper := &ClassObjectDumper{obj: obj, FuncCtx: ctx, declarationResolver: resolverFromClasses(classes)}
	provider := dumper.buildSiblingClassSig()
	// A declaration-only resolver is enough; no archive/sibling ownership is
	// manufactured merely to make the generic constraint visible.
	formal := types.ResolveInstantiatedParamType(ctx, provider, "bindingp.BindingFixed", nil, "release", "(Lbindingp/BindingEntry;Z)V", 2, 0)
	if n, ok := types.RawClassFQN(formal); !ok || n != "bindingp.BindingItem" {
		t.Fatalf("source formal: %v", formal)
	}
	dumper.declarationResolver = func(string) ([]byte, bool) { return classes["bindingp/BindingItem"], true }
	if _, _, ok := dumper.buildSiblingClassSig()("bindingp/BindingPool"); ok {
		t.Fatal("wrong original class identity accepted")
	}
	// A supplied target has priority, including invalid bytes. Never silently
	// replace an invalid own declaration with a convenient dependency witness.
	dumper.foldSiblingResolver = func(string) ([]byte, bool) { return []byte{0, 1}, true }
	dumper.declarationResolver = resolverFromClasses(classes)
	if _, _, ok := dumper.buildSiblingClassSig()("bindingp/BindingPool"); ok {
		t.Fatal("invalid target hidden by dependency")
	}
}
