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

// Vary independent source structures, not only names: interface inheritance,
// local copy-chain length, static/instance factory and enum-role composition.
// The driver is compiled and run first against the original classfiles, then
// retained unchanged while every target classfile is replaced by rebuilt Java.
func TestAdversarialAnonymousThisAliasFamilyBank(t *testing.T) {
	javac, java := t04Tools(t)
	sources := map[string]string{}
	var calls strings.Builder
	families := 0
	for depth := 1; depth <= 5; depth++ {
		for copies := 1; copies <= 5; copies++ {
			for _, static := range []bool{false, true} {
				for _, enumeration := range []bool{false, true} {
					owner := fmt.Sprintf("AliasBank%d", families)
					families++
					var source strings.Builder
					fmt.Fprintf(&source, "public class %s{public interface Api{Object token();Api next();long score();}", owner)
					parent := "Api"
					for i := 1; i <= depth; i++ {
						fmt.Fprintf(&source, "public interface Api%d extends %s{}", i, parent)
						parent = fmt.Sprintf("Api%d", i)
					}
					if enumeration {
						source.WriteString("public enum Kind{A{public int score(){return 17;}},B{public int score(){return 31;}};public abstract int score();}")
					}
					fmt.Fprintf(&source, "static long pick(Object v,long n){return n+91;}static long pick(Api v,long n){return n+17;}static long pick(%s v,long n){return n+31;}", parent)
					modifier, factory := "", "new "+owner+"().make(token,n)"
					if static {
						modifier, factory = "static ", owner+".make(token,n)"
					}
					fmt.Fprintf(&source, "public %s%s make(final Object token,final long n){return new %s(){public Object token(){return token;}public long score(){return n;}public Api next(){", modifier, parent, parent)
					previous := "this"
					for i := 0; i < copies; i++ {
						name := fmt.Sprintf("alias%d", i)
						fmt.Fprintf(&source, "final %s %s=%s;", parent, name, previous)
						previous = name
					}
					fmt.Fprintf(&source, "return new %s(){public Object token(){return %s.token();}public Api next(){return %s;}public long score(){return pick(%s,n);}};}};}", parent, previous, previous, previous)
					fmt.Fprintf(&source, `static void verify(){Object sentinel=new Object();int rows=0;for(Object token:new Object[]{null,sentinel,"text"})for(long n:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){Api first=%s,second=first.next();if(first.token()!=token||second.token()!=token||second.next()!=first||first.score()!=n||second.score()!=n+31)throw new AssertionError("identity/value/overload");if(!first.getClass().isAnonymousClass()||!second.getClass().isAnonymousClass()||first.getClass().getEnclosingClass()!=%s.class||second.getClass().getEnclosingClass()!=first.getClass()||!second.getClass().getEnclosingMethod().getName().equals("next"))throw new AssertionError("lexical ownership");rows++;}`, factory, owner)
					if enumeration {
						source.WriteString(`if(Kind.A.score()!=17||Kind.B.score()!=31||!Kind.A.getClass().isAnonymousClass()||Kind.A.getClass().getSuperclass()!=Kind.class)throw new AssertionError("enum role");`)
					}
					source.WriteString(`if(rows!=12)throw new AssertionError("rows");}}`)
					sources[owner+".java"] = source.String()
					fmt.Fprintf(&calls, "%s.verify();", owner)
				}
			}
		}
	}
	if families != 100 {
		t.Fatal(families)
	}
	sources["AliasOracle.java"] = "class AliasOracle{public static void main(String[]args){" + calls.String() + `System.out.println("families:100:rows:1200");}}`
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, "AliasOracle")
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
					archive := nativeArchive(t, files)
					defer archive.Close()
					out := t.TempDir()
					var paths []string
					for name, raw := range files {
						if !strings.HasPrefix(name, "AliasBank") {
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
					if got := t04RunJava(t, java, out, "AliasOracle"); got != oracle {
						t.Fatalf("compiled observations differ %q", got)
					}
					for name, raw := range files {
						if !strings.HasPrefix(name, "AliasBank") {
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
