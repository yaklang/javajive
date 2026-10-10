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

// 100 valid source families, with different enum allocation arities/counts,
// primitive widths, root staticness and capture array/reference descriptors.
// Name/debug/rewrite repetitions do not increase the semantic family count.
func TestAdversarialEnumAnonymousRoleFamilyBank(t *testing.T) {
	javac, java := t04Tools(t)
	sources := map[string]string{}
	var calls strings.Builder
	families := 0
	for count := 1; count <= 5; count++ {
		for _, typ := range []string{"int", "long", "float", "double", "char"} {
			for _, static := range []bool{false, true} {
				for _, array := range []bool{false, true} {
					index := families
					families++
					owner := fmt.Sprintf("EnumRoleBank%d", index)
					driver := fmt.Sprintf("EnumRoleDriver%d", index)
					var enumeration, probe strings.Builder
					enumeration.WriteString("public enum Kind{")
					for constant := 0; constant < count; constant++ {
						if constant > 0 {
							enumeration.WriteString(",")
						}
						fmt.Fprintf(&enumeration, "C%d((%s)%d){public double code(){return (double)word;}}", constant, typ, constant+1)
						fmt.Fprintf(&probe, "if(Double.doubleToRawLongBits(%s.Kind.C%d.code())!=Double.doubleToRawLongBits(%d.0)||!%s.Kind.C%d.getClass().isAnonymousClass()||%s.Kind.C%d.getClass().getSuperclass()!=%s.Kind.class)throw new AssertionError(\"enum width/dispatch/owner\");", owner, constant, constant+1, owner, constant, owner, constant, owner)
					}
					fmt.Fprintf(&enumeration, ";public final %s word;private Kind(%s word){this.word=word;}public abstract double code();}", typ, typ)
					source := strings.Replace(nativeAnonymousNestedFixture, "class NestedOwner{", "class NestedOwner{"+enumeration.String(), 1)
					if !static {
						source = strings.Replace(source, "static NestedFactory make(", "NestedFactory make(", 1)
						source = strings.ReplaceAll(source, "NestedOwner.make(", "new NestedOwner().make(")
					}
					if array {
						source = strings.Replace(source, "final Object token,", "final Object[] token,", 1)
						source = strings.Replace(source, "Object identity=new Object();", "Object[] identity=new Object[]{new Object()};", 1)
						source = strings.Replace(source, `for(Object token:new Object[]{null,identity,"text"})`, `for(Object[] token:new Object[][]{null,identity,new Object[]{"text"}})`, 1)
					}
					for old, replacement := range map[string]string{"NestedOwner": owner, "NestedEffects": fmt.Sprintf("EnumRoleEffects%d", index), "NestedFactory": fmt.Sprintf("EnumRoleFactory%d", index), "NestedPhase": fmt.Sprintf("EnumRolePhase%d", index), "NestedDriver": driver} {
						source = strings.ReplaceAll(source, old, replacement)
					}
					output := fmt.Sprintf(`System.out.println(rows+":"+%s.trace);`, fmt.Sprintf("EnumRoleEffects%d", index))
					if !strings.Contains(source, output) {
						t.Fatal("driver insertion point")
					}
					source = strings.Replace(source, output, probe.String()+output, 1)
					sources[owner+".java"] = source
					fmt.Fprintf(&calls, "%s.main(args);", driver)
				}
			}
		}
	}
	if families != 100 {
		t.Fatal(families)
	}
	sources["EnumRoleBankDriver.java"] = "class EnumRoleBankDriver{public static void main(String[]args)throws Exception{" + calls.String() + `System.out.println("families:100:rows:5800");}}`
	want := strings.Repeat("58:P\n", 100) + "families:100:rows:5800\n"
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, "EnumRoleBankDriver")
			if oracle != want {
				t.Fatalf("original family observations %q", oracle)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					archive := nativeArchive(t, files)
					defer archive.Close()
					out := t.TempDir()
					var paths []string
					for name, raw := range files {
						if !strings.HasPrefix(name, "EnumRoleBank") || name == "EnumRoleBankDriver.class" {
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
					if text, err := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, paths...)...).CombinedOutput(); err != nil {
						t.Fatalf("candidate compile %v\n%s", err, text)
					}
					if got := t04RunJava(t, java, out, "EnumRoleBankDriver"); got != oracle {
						t.Fatalf("compiled observations differ %q", got)
					}
					for name, raw := range files {
						if !strings.HasPrefix(name, "EnumRoleBank") || name == "EnumRoleBankDriver.class" {
							continue
						}
						candidate, err := os.ReadFile(filepath.Join(out, name))
						if err != nil {
							t.Fatal(err)
						}
						if nativeBinaryShape(t, raw) != nativeBinaryShape(t, candidate) || nativeAnonymousAccessorShape(t, raw) != nativeAnonymousAccessorShape(t, candidate) {
							t.Fatalf("exact original binary/accessor declarations changed %s", name)
						}
					}
				})
			}
		})
	}
}
