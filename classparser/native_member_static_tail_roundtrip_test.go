package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMemberStaticTailFixture = `class StaticTailEffects{static String trace="";static Object token=new Object();static Object read(){trace+="R";return token;}}
class StaticTailBase{final int number;final Object observed;StaticTailBase(int n){number=n;observed=owner();}Object owner(){return null;}}
class StaticTailOwner{interface Value<T>{T get();}static final Value<Object> VALUE=new Value<Object>(){public Object get(){return StaticTailEffects.read();}};class Child extends StaticTailBase{Child(int n){super(n);}Object owner(){return StaticTailOwner.this;}}}
class StaticTailDriver{public static void main(String[]args){StaticTailOwner owner=new StaticTailOwner();StaticTailOwner.Child child=owner.new Child(Integer.MIN_VALUE);if(child.owner()!=owner||child.observed!=owner||child.number!=Integer.MIN_VALUE||!StaticTailEffects.trace.equals(""))throw new AssertionError("construction or eager effects");Object first=StaticTailOwner.VALUE.get();Object second=StaticTailOwner.VALUE.get();if(first!=StaticTailEffects.token||second!=first||!StaticTailEffects.trace.equals("RR")||StaticTailOwner.VALUE.getClass().getDeclaredMethods().length!=1)throw new AssertionError("delayed identity or ABI");System.out.println("static:tail:"+child.number+":"+StaticTailEffects.trace);}}`

// A real javac8/7 initializer supplies EnclosingMethod.method_index=0 and a
// final anonymous header. Rebuilding the named family with a modern compiler
// must retain this independent flat declaration rather than losing the named
// child's original enclosing/super constructor operands.
func TestNativeMemberStaticInitializerTailKeepsNamedConstructorBinding(t *testing.T) {
	modern, _ := t04Tools(t)
	legacy := os.Getenv("JAVA8_JAVAC")
	if legacy == "" {
		t.Skip("JAVA8_JAVAC is required for the independent original compiler")
	}
	version, err := exec.Command(legacy, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatalf("independent javac8 identity: %v %s", err, version)
	}
	for _, root := range []string{"StaticTailOwner", "RenamedStaticTailOwner"} {
		t.Run(root, func(t *testing.T) {
			fixture := strings.ReplaceAll(nativeMemberStaticTailFixture, "StaticTailOwner", root)
			for _, release := range []string{"7", "8"} {
				t.Run(release, func(t *testing.T) {
					compile := func(debug string) map[string][]byte {
						dir := t.TempDir()
						file := filepath.Join(dir, root+".java")
						if err := os.WriteFile(file, []byte(fixture), 0600); err != nil {
							t.Fatal(err)
						}
						if out, err := exec.Command(legacy, "-proc:none", "-source", release, "-target", release, "-g:"+debug, "-d", dir, file).CombinedOutput(); err != nil {
							t.Fatalf("authored original: %v %s", err, out)
						}
						files := map[string][]byte{}
						entries, err := os.ReadDir(dir)
						if err != nil {
							t.Fatal(err)
						}
						for _, entry := range entries {
							if strings.HasSuffix(entry.Name(), ".class") {
								raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
								if err != nil {
									t.Fatal(err)
								}
								files[entry.Name()] = raw
							}
						}
						anonymous, err := Parse(files[root+"$1.class"])
						if err != nil {
							t.Fatal(err)
						}
						owner, method, known := originalAnonymousOwner(anonymous)
						if !known || owner != root || method != "" || anonymous.AccessFlags != 0x30 {
							t.Fatalf("real static-initializer ownership: %s %s %v %#x", owner, method, known, anonymous.AccessFlags)
						}
						return files
					}
					testNativeIndependentCompilerFamilyFixture(t, compile, ModernJavac, modern, []string{root}, "StaticTailDriver", "static:tail:-2147483648:RR\n", nil, nativeLexicalExactSignatures)
				})
			}
		})
	}
}
