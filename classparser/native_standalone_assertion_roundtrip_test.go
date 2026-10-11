package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The unchanged caller independently controls the two lexical status owners.
// It observes disabled predicate/message suppression, failure type/order,
// successful values and the original synthetic field metadata. The nested
// child causes javac to reach an inherited assertion-support field collision
// if the base's compiler field is emitted as an ordinary source declaration.
const nativeStandaloneAssertionFixture = `class RootAssertionEffects{static String trace="";}
class OwnerBase{boolean condition(int x){RootAssertionEffects.trace+="B";return x>=0;}String message(){RootAssertionEffects.trace+="M";return "base";}int step(int x){assert condition(x):message();return x+1;}}
class OwnerFamily{static class Child extends OwnerBase{boolean childCondition(int x){RootAssertionEffects.trace+="C";return x!=7;}String childMessage(){RootAssertionEffects.trace+="Q";return "child";}int check(int x){assert childCondition(x):childMessage();return step(x);}}static Child make(){return new Child();}}
class RootAssertionDriver{public static void main(String[]a)throws Exception{boolean base=BASE_STATUS,child=CHILD_STATUS;ClassLoader l=ClassLoader.getSystemClassLoader();l.setClassAssertionStatus("OwnerBase",base);l.setClassAssertionStatus("OwnerFamily",child);OwnerFamily.Child c=OwnerFamily.make();RootAssertionEffects.trace="";if(c.check(1)!=2||!RootAssertionEffects.trace.equals((child?"C":"")+(base?"B":"")))throw new AssertionError("success effects");RootAssertionEffects.trace="";try{if(c.check(7)!=8||child)throw new AssertionError("missing child assertion");}catch(AssertionError e){if(!child||!"child".equals(e.getMessage()))throw e;}if(!RootAssertionEffects.trace.equals(child?"CQ":base?"B":""))throw new AssertionError("child message order");RootAssertionEffects.trace="";try{if(c.check(-1)!=0||base)throw new AssertionError("missing base assertion");}catch(AssertionError e){if(!base||!"base".equals(e.getMessage()))throw e;}if(!RootAssertionEffects.trace.equals((child?"C":"")+(base?"BM":"")))throw new AssertionError("base message order");java.lang.reflect.Field f=OwnerBase.class.getDeclaredField("$assertionsDisabled");if(!f.isSynthetic()||f.getType()!=boolean.class||f.getModifiers()!=0x1018)throw new AssertionError("original flag metadata");System.out.println("root-assertion:independent-status:effects:metadata");}}`

func TestNativeStandaloneAssertionPreservesInheritedCompilerProtocol(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		for _, base := range []bool{false, true} {
			for _, child := range []bool{false, true} {
				t.Run(fmt.Sprintf("renamed=%t/base=%t/child=%t", renamed, base, child), func(t *testing.T) {
					fixture := strings.NewReplacer("BASE_STATUS", fmt.Sprint(base), "CHILD_STATUS", fmt.Sprint(child)).Replace(nativeStandaloneAssertionFixture)
					owner := "Owner"
					if renamed {
						fixture = strings.NewReplacer("Owner", "RenamedScope", "step(", "advance(", "condition(", "predicate(").Replace(fixture)
						owner = "RenamedScope"
					}
					testNativePrivateSetterFixture(t, fixture, owner, "RootAssertionDriver", "root-assertion:independent-status:effects:metadata\n")
				})
			}
		}
	}
}

func TestNativeStandaloneAssertionNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for independent original compiler")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original compiler", err, string(version))
	}
	for _, base := range []bool{false, true} {
		for _, child := range []bool{false, true} {
			t.Run(fmt.Sprintf("base=%t/child=%t", base, child), func(t *testing.T) {
				fixture := strings.NewReplacer("BASE_STATUS", fmt.Sprint(base), "CHILD_STATUS", fmt.Sprint(child)).Replace(nativeStandaloneAssertionFixture)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					dir := t.TempDir()
					path := filepath.Join(dir, "OwnerFamily.java")
					if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", dir, path).CombinedOutput(); err != nil {
						t.Fatal("original compile", err, string(out))
					}
					entries, err := os.ReadDir(dir)
					if err != nil {
						t.Fatal(err)
					}
					files := map[string][]byte{}
					for _, entry := range entries {
						if strings.HasSuffix(entry.Name(), ".class") {
							raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
							if err != nil {
								t.Fatal(err)
							}
							files[entry.Name()] = raw
						}
					}
					return files
				}, NativeJavac8, javac, []string{"OwnerBase", "OwnerFamily"}, "RootAssertionDriver", "root-assertion:independent-status:effects:metadata\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}
