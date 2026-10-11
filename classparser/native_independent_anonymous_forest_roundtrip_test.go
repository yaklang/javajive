package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeIndependentForestNestedBody = `return new ForestResult<T>(){public T get(){ForestResult<T> nested=new ForestResult<T>(){public T get(){return other==null?Bag.this.token:other;}public long stamp(){return Bag.this.base^stamp;}};return nested.get();}public long stamp(){ForestResult<T> nested=new ForestResult<T>(){public T get(){return other;}public long stamp(){return Bag.this.base^stamp;}};return nested.stamp();}};`
const nativeIndependentForestPreamble = `interface ForestResult<T>{T get();long stamp();}class ForestScope{static class Bag<T>{final T token;final long base;Bag(T token,long base){this.token=token;this.base=base;}`
const nativeIndependentForestDriver = `class ForestDriver{public static void main(String[]args){int rows=0;Object same=new Object();for(Object token:new Object[]{null,same,"same",new String("same")})for(Object other:new Object[]{null,same,"same",new String("same")})for(long word:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){ForestScope.Bag<Object> bag=new ForestScope.Bag<Object>(token,~word);ForestResult<Object> result=bag.make(other,word);if(result.get()!=(other==null?token:other)||result.get()!=(other==null?token:other))throw new AssertionError("nested capture identity");if(result.stamp()!=-1)throw new AssertionError("wide captured word");if(result.getClass().getEnclosingClass()!=ForestScope.Bag.class||result.getClass().getEnclosingMethod()==null||!result.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("anonymous declaration metadata");rows++;}System.out.println(rows+":independent:anonymous:forest");}}`

func nativeIndependentForestFixture(shape, rename string) (string, string) {
	body := nativeIndependentForestNestedBody
	if shape == "direct" {
		body = `return new ForestResult<T>(){public T get(){return other==null?Bag.this.token:other;}public long stamp(){return Bag.this.base^stamp;}};`
	}
	method := "ForestResult<T> make(final T other,final long stamp){" + body + "}"
	driver := nativeIndependentForestDriver
	if shape == "named" {
		method = "class Holder{ForestResult<T> create(final T other,final long stamp){" + body + "}}ForestResult<T> make(final T other,final long stamp){return new Holder().create(other,stamp);}"
		driver = strings.ReplaceAll(driver, "ForestScope.Bag.class", "ForestScope.Bag.Holder.class")
		driver = strings.ReplaceAll(driver, "equals(\"make\")", "equals(\"create\")")
	}
	fixture := nativeIndependentForestPreamble + method + "}}" + driver
	outer := "ForestScope"
	if rename == "renamed" {
		outer = "ChangedScope"
		fixture = strings.NewReplacer("ForestScope", outer, "ForestResult", "ChangedResult").Replace(fixture)
	}
	return fixture, outer
}
func nativeIndependentForestInput(t *testing.T, files map[string][]byte, outer string) {
	t.Helper()
	o, e := Parse(files[outer+".class"])
	if e != nil {
		t.Fatal(e)
	}
	o.MajorVersion = 55
	files[outer+".class"] = o.Bytes()
}
func TestNativeIndependentAnonymousForestRetainsNestedCaptures(t *testing.T) {
	for _, shape := range []string{"direct", "nested", "named"} {
		t.Run(shape, func(t *testing.T) {
			for _, rename := range []string{"original", "renamed"} {
				t.Run(rename, func(t *testing.T) {
					fixture, outer := nativeIndependentForestFixture(shape, rename)
					testNativeIndependentMutatedFamilyFixture(t, fixture, []string{outer + "$Bag"}, "ForestDriver", "80:independent:anonymous:forest\n", func(t *testing.T, files map[string][]byte) { nativeIndependentForestInput(t, files, outer) }, nativeLexicalExactSignatures)
				})
			}
		})
	}
}
func TestNativeIndependentAnonymousForestNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for independent legacy source closure")
	}
	v, e := exec.Command(javac, "-version").CombinedOutput()
	if e != nil || !strings.Contains(string(v), "javac 1.8.") {
		t.Fatal("original compiler", e, string(v))
	}
	for _, shape := range []string{"direct", "nested", "named"} {
		t.Run(shape, func(t *testing.T) {
			fixture, outer := nativeIndependentForestFixture(shape, "original")
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
