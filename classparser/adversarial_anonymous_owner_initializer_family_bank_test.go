package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Structural bank: ordered pre/post stores, original super arguments and a
// second anonymous initializer. Original callbacks and abrupt completion are
// observed by an unchanged driver; all target classfiles are rebuilt.
func TestAdversarialAnonymousOwnerInitializerFamilyBank(t *testing.T) {
	testAnonymousOwnerInitializerFamilyBank(t, "8")
}
func TestAdversarialAnonymousOwnerInitializerFamilyBankJava21(t *testing.T) {
	testAnonymousOwnerInitializerFamilyBank(t, "21")
}
func testAnonymousOwnerInitializerFamilyBank(t *testing.T, release string) {
	javac, java := t04Tools(t)
	sources := map[string]string{}
	var calls strings.Builder
	families := 0
	for before := 0; before < 5; before++ {
		for after := 0; after < 5; after++ {
			for _, parameter := range []bool{false, true} {
				for _, second := range []bool{false, true} {
					owner := fmt.Sprintf("InitBank%d", families)
					families++
					var s strings.Builder
					fmt.Fprintf(&s, "public class %s extends InitBase{", owner)
					preTrace := ""
					for i := 0; i < before; i++ {
						fmt.Fprintf(&s, "final long pre%d=InitEffects.touch(this.seed,%d,\"B%d\");", i, i, i)
						preTrace += fmt.Sprintf("B%d", i)
					}
					s.WriteString("final InitParent value=new InitParent(InitEffects.arg(this.seed)){long get(){return word^0xCAFEBABEL;}};")
					secondTrace := ""
					if second {
						s.WriteString("final InitParent other=new InitParent(InitEffects.arg(this.seed+1)){long get(){return word^0xFADEBABEL;}};")
						secondTrace = "AP"
					}
					postTrace := ""
					for i := 0; i < after; i++ {
						fmt.Fprintf(&s, "final long post%d=InitEffects.touch(this.seed,%d,\"T%d\");", i, i, i)
						postTrace += fmt.Sprintf("T%d", i)
					}
					constructor, factory := "()", "new "+owner+"()"
					super := "InitEffects.input"
					if parameter {
						constructor = "(long n)"
						factory = "new " + owner + "(InitEffects.argument(InitEffects.input))"
						super = "n"
					}
					fmt.Fprintf(&s, "%s%s{super(%s);InitEffects.trace+=\"C\";}", owner, constructor, super)
					s.WriteString("}")
					var check strings.Builder
					fmt.Fprintf(&check, "class InitCheck%d{", families-1)
					begin := "S"
					if parameter {
						begin = "VS"
					}
					fmt.Fprintf(&check, `static void verify(){int rows=0;for(long n:new long[]{Long.MIN_VALUE,-1,0,1,42,Long.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){InitEffects.input=n;InitEffects.fail=fail;InitEffects.trace="";InitEffects.published=null;InitEffects.partial=null;try{%s root=%s;if(fail)throw new AssertionError("missing failure");if(root.seed!=n||root.value.word!=n||root.value.get()!=(n^0xCAFEBABEL)||!InitEffects.trace.equals("%s%sAP%s%sC"))throw new AssertionError("original effect/value order");scope(root.value.getClass());`, owner, factory, begin, preTrace, secondTrace, postTrace)
					for i := 0; i < before; i++ {
						fmt.Fprintf(&check, "if(root.pre%d!=n+%d)throw new AssertionError(\"pre store\");", i, i)
					}
					for i := 0; i < after; i++ {
						fmt.Fprintf(&check, "if(root.post%d!=n+%d)throw new AssertionError(\"post store\");", i, i)
					}
					if second {
						check.WriteString("if(root.other.word!=n+1||root.other.get()!=((n+1)^0xFADEBABEL))throw new AssertionError(\"second initializer\");scope(root.other.getClass());")
					}
					fmt.Fprintf(&check, `}catch(RuntimeException e){if(!fail||e!=InitEffects.failure||InitEffects.partial==null||InitEffects.partial.word!=n||!InitEffects.trace.equals("%s%sAP"))throw new AssertionError("partial construction identity/order",e);%s root=(%s)InitEffects.published;if(root==null||root.value!=null)throw new AssertionError("store occurred after throwing parent");scope(InitEffects.partial.getClass());}rows++;}if(rows!=12)throw new AssertionError("rows");}static void scope(Class<?> c){if(!c.isAnonymousClass()||c.getEnclosingClass()!=%s.class||c.getEnclosingMethod()!=null||c.getEnclosingConstructor()!=null)throw new AssertionError("initializer lexical ownership");}}`, begin, preTrace, owner, owner, owner)
					sources[owner+".java"] = s.String()
					sources[fmt.Sprintf("InitCheck%d.java", families-1)] = check.String()
					fmt.Fprintf(&calls, "InitCheck%d.verify();", families-1)
				}
			}
		}
	}
	if families != 100 {
		t.Fatal(families)
	}
	sources["InitEffects.java"] = `class InitEffects{static long input;static boolean fail;static String trace="";static InitBase published;static InitParent partial;static final RuntimeException failure=new RuntimeException("same");static long touch(long n,int i,String mark){trace+=mark;return n+i;}static long argument(long n){trace+="V";return n;}static long arg(long n){trace+="A";return n;}}class InitBase{final long seed;InitBase(long seed){InitEffects.trace+="S";this.seed=seed;InitEffects.published=this;}}class InitParent{final long word;InitParent(long word){InitEffects.trace+="P";this.word=word;InitEffects.partial=this;if(get()!=(word^0xCAFEBABEL)&&get()!=(word^0xFADEBABEL))throw new AssertionError("virtual callback");if(InitEffects.fail)throw InitEffects.failure;}long get(){return word;}}`
	sources["InitOracle.java"] = "class InitOracle{public static void main(String[]args){" + calls.String() + `System.out.println("families:100:rows:1200");}}`
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileSourceReleaseClasses(t, sources, debug, release)
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, "InitOracle")
			if oracle != "families:100:rows:1200\n" {
				t.Fatalf("original observations %q", oracle)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					sourceVersion := 8
					if release == "21" {
						sourceVersion = 21
					}
					archive, err := NewJarFSFromLocalWithCompilerProfile(func() string {
						p := filepath.Join(t.TempDir(), "authored.jar")
						if e := os.WriteFile(p, t23Zip(t, files), 0600); e != nil {
							t.Fatal(e)
						}
						return p
					}(), sourceVersion, ModernJavac, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer archive.Close()
					out := t.TempDir()
					var paths []string
					for name, raw := range files {
						if !strings.HasPrefix(name, "InitBank") {
							if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
							continue
						}
						text, err := archive.ReadFile(name)
						if err != nil || strings.Contains(string(text), DecompileStubMarker) {
							t.Fatalf("candidate %s %v\n%s", name, err, text)
						}
						path := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
						if err := os.WriteFile(path, text, 0600); err != nil {
							t.Fatal(err)
						}
						paths = append(paths, path)
					}
					sort.Strings(paths)
					if text, err := exec.Command(javac, append([]string{"-proc:none", "--release", release, "-cp", out, "-d", out}, paths...)...).CombinedOutput(); err != nil {
						t.Fatalf("candidate compile %v\n%s", err, text)
					}
					if got := t04RunJava(t, java, out, "InitOracle"); got != oracle {
						t.Fatalf("compiled observations differ %q", got)
					}
					for name, raw := range files {
						if !strings.HasPrefix(name, "InitBank") {
							continue
						}
						candidate, err := os.ReadFile(filepath.Join(out, name))
						if err != nil {
							t.Fatal(err)
						}
						if nativeBinaryShape(t, raw) != nativeBinaryShape(t, candidate) || nativeAnonymousAccessorShape(t, raw) != nativeAnonymousAccessorShape(t, candidate) {
							t.Fatalf("original binary/accessor declarations changed %s", name)
						}
					}
				})
			}
		})
	}
}
