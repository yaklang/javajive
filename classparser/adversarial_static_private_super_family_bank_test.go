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

// 100 structurally distinct Java programs: five primitive constructor widths,
// five reference arities, checked/unchecked SUPER, and static/instance callers.
// Names, debug variants and repeated input rows do not increase that count.
func TestAdversarialStaticPrivateSuperConstructorFamilies(t *testing.T) {
	javac, java := t04Tools(t)
	sources := map[string]string{}
	var driver strings.Builder
	driver.WriteString("class StaticFamilyDriver{public static void main(String[]args)throws Exception{int families=0,rows=0;")
	number := 0
	for _, pair := range [][2]string{{"int", "long"}, {"long", "float"}, {"float", "double"}, {"double", "Object"}, {"char", "int"}} {
		for arity := 0; arity <= 4; arity++ {
			for _, checked := range []bool{false, true} {
				for _, static := range []bool{false, true} {
					owner := fmt.Sprintf("StaticFamily%d", number)
					number++
					var fields, formal, actual, stores, observe strings.Builder
					for i := 0; i < arity; i++ {
						fmt.Fprintf(&fields, "final Object tag%d;", i)
						fmt.Fprintf(&formal, ",final Object tag%d", i)
						fmt.Fprintf(&actual, ",tag%d", i)
						fmt.Fprintf(&stores, "this.tag%d=tag%d;", i, i)
						fmt.Fprintf(&observe, "if(p.tag%d!=tag%d)throw new AssertionError(\"reference argument %d\");", i, i, i)
					}
					throws, fail := "", ""
					if checked {
						throws, fail = "throws java.io.IOException", "if(word<0)throw failure;"
					}
					modifier := ""
					if static {
						modifier = "static "
					}
					body := fmt.Sprintf(`public class %s{
static String trace="";static int arguments,parents;static final java.io.IOException failure=new java.io.IOException("same");
public static class Parent{final %s word;%s
private Parent(%s word%s)%s{trace+="P";parents++;this.word=word;%s%s}
private Parent(%s word%s)%s{throw new AssertionError("wrong overload");}
public double value(){throw new AssertionError("wrong body");}}
private static %s argument(%s word){trace+="A";arguments++;return word;}
public %sParent make(final %s word%s)%s{return new Parent(argument(word)%s){public double value(){trace+="G";return (double)word;}};}
}`, owner, pair[0], fields.String(), pair[0], formal.String(), throws, stores.String(), fail, pair[1], formal.String(), throws, pair[0], pair[0], modifier, pair[0], formal.String(), throws, actual.String())
					sources[owner+".java"] = body
					words := "-1,0,1"
					if pair[0] == "char" {
						words = "0,1,65535"
					}
					fmt.Fprintf(&driver, "{for(%s word:new %s[]{%s}){", pair[0], pair[0], words)
					for i := 0; i < arity; i++ {
						fmt.Fprintf(&driver, "Object tag%d=%s;", i, []string{"null", "new Object()"}[i%2])
					}
					call := owner + ".make"
					if !static {
						call = "new " + owner + "().make"
					}
					// An extra catchable operation makes the same independent driver
					// legal for unchecked constructor scenarios, without editing it
					// according to the generated candidate.
					fmt.Fprintf(&driver, `%s.trace="";%s.arguments=%s.parents=0;
try{barrier();%s.Parent p=%s(word%s);%s
if(p.word!=word||Double.doubleToRawLongBits(p.value())!=Double.doubleToRawLongBits((double)word)||!%s.trace.equals("APG"))throw new AssertionError("values or effects");
Class<?> c=p.getClass();if(!c.isAnonymousClass()||c.getSuperclass()!=%s.Parent.class||c.getEnclosingClass()!=%s.class||!c.getEnclosingMethod().getName().equals("make"))throw new AssertionError("ownership");
if(%t&&word<0)throw new AssertionError("missing throw");
}catch(java.io.IOException e){if(!%t||word>=0||e!=%s.failure||!%s.trace.equals("AP"))throw new AssertionError("exception identity",e);}
if(%s.arguments!=1||%s.parents!=1)throw new AssertionError("evaluation count");rows++;}families++;}`, owner, owner, owner, owner, call, actual.String(), observe.String(), owner, owner, owner, checked, checked, owner, owner, owner, owner)
				}
			}
		}
	}
	if number != 100 {
		t.Fatalf("family inventory %d", number)
	}
	driver.WriteString(`System.out.println(families+":"+rows+":width:arity:checked:caller:private-super");}static void barrier()throws java.io.IOException{}}`)
	sources["StaticFamilyDriver.java"] = driver.String()
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, "StaticFamilyDriver")
			if oracle != "100:300:width:arity:checked:caller:private-super\n" {
				t.Fatal(oracle)
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
						if !strings.HasPrefix(name, "StaticFamily") || name == "StaticFamilyDriver.class" {
							if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
							continue
						}
						source, err := archive.ReadFile(name)
						if err != nil || strings.Contains(string(source), DecompileStubMarker) {
							t.Fatalf("generated %s: %v\n%s", name, err, source)
						}
						path := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
						if err := os.WriteFile(path, source, 0600); err != nil {
							t.Fatal(err)
						}
						paths = append(paths, path)
					}
					sort.Strings(paths)
					if text, err := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, paths...)...).CombinedOutput(); err != nil {
						t.Fatalf("candidate compile: %v\n%s", err, text)
					}
					if got := t04RunJava(t, java, out, "StaticFamilyDriver"); got != oracle {
						t.Fatalf("generated family observations %q want %q", got, oracle)
					}
					for name, raw := range files {
						if !strings.HasPrefix(name, "StaticFamily") || name == "StaticFamilyDriver.class" {
							continue
						}
						candidate, err := os.ReadFile(filepath.Join(out, name))
						if err != nil {
							t.Fatal(err)
						}
						if nativeBinaryShape(t, raw) != nativeBinaryShape(t, candidate) || nativeAnonymousAccessorShape(t, raw) != nativeAnonymousAccessorShape(t, candidate) {
							t.Fatalf("original ABI/accessor shape changed: %s", name)
						}
					}
				})
			}
		})
	}
}
