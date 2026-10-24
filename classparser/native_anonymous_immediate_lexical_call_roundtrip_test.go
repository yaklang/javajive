package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeAnonymousImmediateCallBody = `return new ForestResult<T>(){public T get(){return new ForestResult<T>(){public T get(){return other==null?Bag.this.token:other;}public long stamp(){return Bag.this.base^stamp;}}.get();}public long stamp(){return new ForestResult<T>(){public T get(){return other;}public long stamp(){return Bag.this.base^stamp;}}.stamp();}};`

func nativeAnonymousImmediateCallFixture(shape, rename string) (string, string) {
	preamble, body, driver := nativeIndependentForestPreamble, nativeAnonymousImmediateCallBody, nativeIndependentForestDriver
	prefix := ""
	switch shape {
	case "method shadow":
		preamble = strings.Replace(preamble, "Bag<T>", "Bag<T extends CharSequence>", 1)
		prefix = "<T>"
		body = strings.ReplaceAll(body, "return other==null?Bag.this.token:other;", "return (T)(other==null?Bag.this.token:other);")
		driver = strings.ReplaceAll(driver, "for(Object token:new Object[]{null,same,\"same\",new String(\"same\")})", "for(CharSequence token:new CharSequence[]{null,new StringBuilder(\"same\"),\"same\",new String(\"same\")})")
		driver = strings.ReplaceAll(driver, "ForestScope.Bag<Object> bag=new ForestScope.Bag<Object>", "ForestScope.Bag<CharSequence> bag=new ForestScope.Bag<CharSequence>")
	case "dependent method":
		prefix = "<S,T extends S>"
		body = strings.ReplaceAll(body, "return other==null?Bag.this.token:other;", "return (T)(other==null?Bag.this.token:other);")
	case "forward method dependency":
		prefix = "<T extends S,S>"
		body = strings.ReplaceAll(body, "return other==null?Bag.this.token:other;", "return (T)(other==null?Bag.this.token:other);")
	case "dependent class":
		preamble = strings.Replace(preamble, "Bag<T>", "Bag<S,T extends S>", 1)
		driver = strings.ReplaceAll(driver, "Bag<Object>", "Bag<Object,Object>")
	case "static method dependency":
		prefix = "static <S,T extends S>"
		body = strings.ReplaceAll(body, "Bag.this.token", "other")
		body = strings.ReplaceAll(body, "Bag.this.base", "(~stamp)")
		driver = strings.ReplaceAll(driver, "(other==null?token:other)", "other")
	case "static cut":
		prefix = "static <T>"
		body = strings.ReplaceAll(body, "Bag.this.token", "other")
		body = strings.ReplaceAll(body, "Bag.this.base", "(~stamp)")
		driver = strings.ReplaceAll(driver, "(other==null?token:other)", "other")
	}
	fixture := preamble + prefix + "ForestResult<T> make(final T other,final long stamp){" + body + "}}}" + driver
	outer := "ForestScope"
	if rename == "renamed" {
		outer = "ChangedScope"
		fixture = strings.NewReplacer("ForestScope", outer, "ForestResult", "ChangedResult").Replace(fixture)
	}
	return fixture, outer
}
func TestNativeAnonymousImmediateCallKeepsOriginalLexicalReturn(t *testing.T) {
	for _, shape := range []string{"class binder", "method shadow", "dependent method", "forward method dependency", "dependent class", "static method dependency", "static cut"} {
		t.Run(shape, func(t *testing.T) {
			for _, rename := range []string{"original", "renamed"} {
				t.Run(rename, func(t *testing.T) {
					fixture, outer := nativeAnonymousImmediateCallFixture(shape, rename)
					testNativeIndependentMutatedFamilyFixture(t, fixture, []string{outer + "$Bag"}, "ForestDriver", "80:independent:anonymous:forest\n", func(t *testing.T, files map[string][]byte) { nativeIndependentForestInput(t, files, outer) }, nativeLexicalExactSignatures)
				})
			}
		})
	}
}
func TestNativeAnonymousImmediateLexicalCallNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for independently compiled lexical binders")
	}
	version, e := exec.Command(javac, "-version").CombinedOutput()
	if e != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original compiler", e, string(version))
	}
	for _, shape := range []string{"class binder", "method shadow", "dependent method", "forward method dependency", "dependent class", "static method dependency", "static cut"} {
		t.Run(shape, func(t *testing.T) {
			fixture, outer := nativeAnonymousImmediateCallFixture(shape, "original")
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				dir := t.TempDir()
				path := filepath.Join(dir, outer+".java")
				if e := os.WriteFile(path, []byte(fixture), 0600); e != nil {
					t.Fatal(e)
				}
				if out, e := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", dir, path).CombinedOutput(); e != nil {
					t.Fatal("original compile", e, string(out))
				}
				entries, e := os.ReadDir(dir)
				if e != nil {
					t.Fatal(e)
				}
				files := map[string][]byte{}
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".class") {
						raw, e := os.ReadFile(filepath.Join(dir, entry.Name()))
						if e != nil {
							t.Fatal(e)
						}
						files[entry.Name()] = raw
					}
				}
				return files
			}, NativeJavac8, javac, []string{outer + "$Bag"}, "ForestDriver", "80:independent:anonymous:forest\n", func(t *testing.T, files map[string][]byte) { nativeIndependentForestInput(t, files, outer) }, nativeLexicalExactSignatures)
		})
	}
}
