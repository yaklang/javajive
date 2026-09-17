package cross

import (
	"github.com/yaklang/javajive"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Every application class is rebuilt. The original directory serves only as
// a resolver during decompilation, and never appears on javac/java rebuilt CPs.
func auditSourceSet(t *testing.T, sources map[string]string, driver string, mode javajive.DecompileMode) {
	t.Helper()
	original, rebuilt := t.TempDir(), t.TempDir()
	writeSources(t, original, sources)
	writeSources(t, original, map[string]string{"Driver.java": driver})
	names := []string{"Driver.java"}
	for n := range sources {
		names = append(names, n)
	}
	sort.Strings(names)
	args := append([]string{"--release", "8", "-g:none", "-d", original}, names...)
	javac, java := auditTool(t, "javac"), auditTool(t, "java")
	auditCommand(t, original, javac, args...)
	want := auditCommand(t, original, java, "-Xverify:all", "-cp", original, "Driver")
	raws := map[string][]byte{}
	err := filepath.WalkDir(original, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".class") {
			return nil
		}
		rel, err := filepath.Rel(original, path)
		if err != nil {
			return err
		}
		if rel == "Driver.class" {
			return nil
		}
		raw, err := os.ReadFile(path)
		raws[strings.TrimSuffix(filepath.ToSlash(rel), ".class")] = raw
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := func(name string) ([]byte, bool) { b, ok := raws[name]; return b, ok }
	names = []string{"Driver.java"}
	keys := []string{}
	for key := range raws {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result, err := javajive.DecompileWithOptions(raws[key], javajive.DecompileOptions{Mode: mode, Resolve: resolver})
		if err != nil || result.Status != "complete" {
			t.Fatalf("%s: status=%s diagnostics=%+v err=%v", key, result.Status, result.Diagnostics, err)
		}
		t.Logf("%s hash=%s rules=%+v", key, result.InputHash, result.RulesApplied)
		name := key + ".java"
		writeSources(t, rebuilt, map[string]string{name: result.Source})
		names = append(names, name)
	}
	writeSources(t, rebuilt, map[string]string{"Driver.java": driver})
	args = append([]string{"--release", "8", "-cp", rebuilt, "-d", rebuilt}, names...)
	auditCommand(t, rebuilt, javac, args...)
	got := auditCommand(t, rebuilt, java, "-Xverify:all", "-cp", rebuilt, "Driver")
	if got != want {
		t.Fatalf("original=%q rebuilt=%q", want, got)
	}
}
func TestAuditTypeIdentitySources(t *testing.T) {
	for _, mode := range []javajive.DecompileMode{javajive.Precision, javajive.Compatibility} {
		t.Run(string(mode), func(t *testing.T) {
			t.Run("A25_same_simple_name", func(t *testing.T) {
				auditSourceSet(t, map[string]string{
					"one/List.java": "package one;public class List {public int f(){return 1;}}",
					"two/List.java": "package two;public class List {public int f(){return 2;}}",
					"Fixture.java":  "public class Fixture {public static String f(boolean b){Object x;if(b)x=new one.List();else x=new two.List();return x.getClass().getName();}public static int g(one.List a,two.List b){return a.f()*10+b.f();}}",
				}, "public class Driver {public static void main(String[] args){System.out.println(Fixture.f(true));System.out.println(Fixture.f(false));System.out.println(Fixture.g(new one.List(),new two.List()));}}", mode)
			})
			t.Run("A26_nested_not_inherited", func(t *testing.T) {
				auditSourceSet(t, map[string]string{
					"Holder.java":  "public class Holder {private int value=7;public static class Nested {public int f(Holder h){return h.value;}}}",
					"Fixture.java": "public class Fixture {public static int f(){return new Holder.Nested().f(new Holder());}}",
				}, "public class Driver {public static void main(String[] args){System.out.println(Fixture.f());}}", mode)
			})
		})
	}
}

func TestAuditNumberSuffixContinuation(t *testing.T) {
	raw, err := os.ReadFile("../../classparser/testdata/regression/SwitchBreakMissingReturnSeed.java")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []javajive.DecompileMode{javajive.Precision, javajive.Compatibility} {
		t.Run(string(mode), func(t *testing.T) {
			auditSourceSet(t, map[string]string{"Fixture.java": strings.ReplaceAll(string(raw), "SwitchBreakMissingReturnSeed", "Fixture")}, `public class Driver {public static void main(String[] args){for(String x:new String[]{null,"1","1L","0F","1F","1e100F","1D","badF","badL","1Q"}){try{Number n=Fixture.parse(x);System.out.println(n==null?"null":n.getClass().getName()+":"+n);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+e.getMessage());}}}}`, mode)
		})
	}
}
