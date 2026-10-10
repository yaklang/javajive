package cross

import (
	"crypto/sha256"
	"fmt"
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
	auditSourceSetWithLedgerExpectations(t, sources, driver, mode, nil)
}

type memberLedgerExpectation struct {
	owner, name, descriptor, state, evidence string
}

func auditSourceSetWithLedgerExpectations(t *testing.T, sources map[string]string, driver string, mode javajive.DecompileMode, expected []memberLedgerExpectation) {
	t.Helper()
	original, rebuilt := t.TempDir(), t.TempDir()
	record := newAuditObservation(t, mode, "source-set-g:none")
	writeSources(t, original, sources)
	writeSources(t, original, map[string]string{"Driver.java": driver, "AuditVerifier.java": auditVerifierSource})
	names := []string{"Driver.java", "AuditVerifier.java"}
	for n := range sources {
		names = append(names, n)
	}
	sort.Strings(names)
	args := append([]string{"--release", "8", "-g:none", "-d", original}, names...)
	javac, java := auditTool(t, "javac"), auditTool(t, "java")
	auditCommand(t, original, javac, args...)
	record.Compiler = strings.TrimSpace(auditCommand(t, original, javac, "-version"))
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
		if rel == "Driver.class" || rel == "AuditVerifier.class" {
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
	names = []string{"Driver.java", "AuditVerifier.java"}
	keys := []string{}
	for key := range raws {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	classNames := []string{}
	hash := sha256.New()
	for _, key := range keys {
		classNames = append(classNames, strings.ReplaceAll(key, "/", "."))
		fmt.Fprintf(hash, "%s:%d:", key, len(raws[key]))
		hash.Write(raws[key])
	}
	record.InputHash = fmt.Sprintf("%x", hash.Sum(nil))
	verifyArgs := append([]string{"-Xverify:all", "-cp", original, "AuditVerifier"}, classNames...)
	auditCommand(t, original, java, verifyArgs...)
	record.OriginalVerified = true
	want := auditCommand(t, original, java, "-Xverify:all", "-cp", original, "Driver")
	record.Original = want
	for _, key := range keys {
		result, err := javajive.DecompileWithOptions(raws[key], javajive.DecompileOptions{Mode: mode, Resolve: resolver})
		for _, want := range expected {
			if want.owner != key {
				continue
			}
			found := false
			for _, member := range result.Members {
				if member.Owner == want.owner && member.Name == want.name && member.Descriptor == want.descriptor {
					found = member.State == want.state && strings.Contains(member.Evidence, want.evidence)
					if !found {
						t.Fatalf("ledger %s.%s%s = %+v, want state=%q evidence containing %q", member.Owner, member.Name, member.Descriptor, member, want.state, want.evidence)
					}
					break
				}
			}
			if !found {
				t.Fatalf("member ledger did not contain expected row %+v: %+v", want, result.Members)
			}
		}
		if result.Status != "complete" || len(result.StubMethods) > 0 {
			record.Stub = true
		}
		if err != nil || record.Stub {
			t.Fatalf("%s: status=%s stub_methods=%v diagnostics=%+v members=%+v err=%v\nsource:\n%s",
				key, result.Status, result.StubMethods, result.Diagnostics, result.Members, err, result.Source)
		}
		for _, rule := range result.RulesApplied {
			record.Rules = append(record.Rules, rule.Rule)
		}
		t.Logf("%s hash=%s rules=%+v", key, result.InputHash, result.RulesApplied)
		name := key + ".java"
		writeSources(t, rebuilt, map[string]string{name: result.Source})
		names = append(names, name)
	}
	record.Decompiled = true
	writeSources(t, rebuilt, map[string]string{"Driver.java": driver, "AuditVerifier.java": auditVerifierSource})
	args = append([]string{"--release", "8", "-cp", rebuilt, "-d", rebuilt}, names...)
	auditCommand(t, rebuilt, javac, args...)
	record.Recompiled = true
	verifyArgs = append([]string{"-Xverify:all", "-cp", rebuilt, "AuditVerifier"}, classNames...)
	auditCommand(t, rebuilt, java, verifyArgs...)
	record.RebuiltVerified = true
	got := auditCommand(t, rebuilt, java, "-Xverify:all", "-cp", rebuilt, "Driver")
	record.Rebuilt = got
	record.Equal = got == want
	if !record.Equal {
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
