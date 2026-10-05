package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testNativeIndependentFamilyFixture(t *testing.T, fixture string, owners []string, driver, want string, verify ...func(*testing.T, string, []byte, []byte)) {
	t.Helper()
	javac, java := t04Tools(t)
	owned := func(name string) bool {
		for _, owner := range owners {
			if name == owner+".class" || strings.HasPrefix(name, owner+"$") {
				return true
			}
		}
		return false
	}
	write := func(root, name string, raw []byte) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			original := t.TempDir()
			for n, raw := range files {
				write(original, n, raw)
			}
			oracle := t04RunJava(t, java, original, driver)
			if oracle != want {
				t.Fatalf("original oracle=%q", oracle)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					defer z.Close()
					out := t.TempDir()
					paths := []string{}
					for n, raw := range files {
						if !owned(n) {
							write(out, n, raw)
							continue
						}
						source, err := z.ReadFile(n)
						if err != nil || strings.Contains(string(source), DecompileStubMarker) {
							t.Fatalf("source %s:%v\n%s", n, err, source)
						}
						name := strings.TrimSuffix(n, ".class") + ".java"
						write(out, name, source)
						paths = append(paths, filepath.Join(out, filepath.FromSlash(name)))
					}
					if raw, err := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, paths...)...).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt:%v\n%s", err, raw)
					}
					if got := t04RunJava(t, java, out, driver); got != oracle {
						t.Fatalf("rebuilt JVM=%q want=%q", got, oracle)
					}
					for n, raw := range files {
						if !owned(n) {
							continue
						}
						got, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(n)))
						if err != nil {
							t.Fatal(err)
						}
						if a, b := nativeBinaryShape(t, raw), nativeBinaryShape(t, got); a != b {
							t.Fatalf("ABI %s\n%s\n%s", n, a, b)
						}
						for _, check := range verify {
							check(t, n, raw, got)
						}
					}
				})
			}
		})
	}
}

func TestNativeStaticDependencyBothFamiliesRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeStaticDependencyFixture, []string{"DependencyOwner", "OtherScope"}, "DependencyDriver", "static:dependency:identity:callback\n")
}
func TestNativeStaticDependencyOnlySignatureGenericArrayRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeStaticDependencyFixture, `static class Value{final Object token;Value(Object token){this.token=token;}Object value(){return token;}}`, `static class Value<T>{final T token;Value(T token){this.token=token;}T value(){return token;}}`)
	f = strings.ReplaceAll(f, `OtherScope.Value pass(OtherScope.Value value){return value;}`, `<U> OtherScope.Value<U>[] pass(OtherScope.Value<U>[] value){return value;}`)
	f = strings.ReplaceAll(f, `OtherScope.Value value=new OtherScope.Value(token);`, `OtherScope.Value<Object> value=new OtherScope.Value<Object>(token);OtherScope.Value<Object>[] array=(OtherScope.Value<Object>[])new OtherScope.Value<?>[]{value,null};`)
	f = strings.ReplaceAll(f, `child.pass(value)!=value`, `child.pass(array)!=array`)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyOwner", "OtherScope"}, "DependencyDriver", "static:dependency:identity:callback\n")
}
func TestNativeStaticDependencyKeepsLocalMemberNameRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeStaticDependencyFixture, `class DependencyOwner{`, `class DependencyOwner{static class Value{final int n;Value(int n){this.n=n;}}`)
	f = strings.ReplaceAll(f, `OtherScope.Value pass(OtherScope.Value value){return value;}`, `OtherScope.Value pass(OtherScope.Value value){Value local=new Value(31);if(local.n!=31)throw new AssertionError("local member binding");return value;}`)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyOwner", "OtherScope"}, "DependencyDriver", "static:dependency:identity:callback\n")
}
func TestNativeStaticDependencyTransitiveFamiliesRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeStaticDependencyFixture, `class OtherScope{`, `class LastScope{static class Item{final Object value;Item(Object value){this.value=value;}}}class OtherScope{`)
	f = strings.ReplaceAll(f, `static class Value{final Object token;Value(Object token){this.token=token;}Object value(){return token;}}`, `static class Value{final LastScope.Item token;Value(Object token){this.token=new LastScope.Item(token);}Object value(){return token.value;}}`)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyOwner", "OtherScope", "LastScope"}, "DependencyDriver", "static:dependency:identity:callback\n")
}
