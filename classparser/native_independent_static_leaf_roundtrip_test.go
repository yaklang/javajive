package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeIndependentLeafFixture = `class LeafEffects{static String trace="";static int fail;static Object published;static final RuntimeException error=new IllegalStateException("original");static void mark(String event,int stage){trace+=event;if(fail==stage)throw error;}static <T>T input(T token){mark("A",1);return token;}}abstract class LeafBase<T>{final T observed;LeafBase(T token){LeafEffects.published=this;LeafEffects.mark("B",2);observed=read();if(observed!=token)throw new AssertionError("early receiver");}abstract T read();}
class LeafNamespace{static abstract class Leaf<T> extends LeafBase<T>{final long marker;Leaf(T token,long marker){super(token);this.marker=marker;LeafEffects.mark("C",3);}}}
class LeafCaller<T>{final T token;LeafCaller(T token){this.token=token;}class Child extends LeafNamespace.Leaf<T>{Child(long marker){super(LeafEffects.input(LeafCaller.this.token),marker);}T read(){return LeafCaller.this.token;}}LeafNamespace.Leaf<T> make(long marker){return new Child(marker);}}
class LeafDriver{public static void main(String[]args){int rows=0;Object same=new Object();for(Object token:new Object[]{null,same,"same",new String("same")})for(long word:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(int fail=0;fail<4;fail++){LeafCaller<Object> caller=new LeafCaller<Object>(token);LeafEffects.trace="";LeafEffects.fail=fail;LeafEffects.published=null;try{LeafNamespace.Leaf<Object> value=caller.make(word);if(fail!=0||value.observed!=token||value.read()!=token||value.marker!=word||value.getClass().getDeclaringClass()!=LeafCaller.class||!LeafEffects.trace.equals("ABC"))throw new AssertionError("leaf declaration/early enclosing receiver/word/order");}catch(RuntimeException error){if(fail==0||error!=LeafEffects.error||!LeafEffects.trace.equals(fail==1?"A":fail==2?"AB":"ABC"))throw new AssertionError("original failure/order",error);if(fail==1){if(LeafEffects.published!=null)throw new AssertionError("argument failure publication");}else{LeafNamespace.Leaf<?> value=(LeafNamespace.Leaf<?>)LeafEffects.published;if(value==null||value.read()!=token||value.observed!=(fail==2?null:token)||value.marker!=(fail==2?0:word))throw new AssertionError("failure capture/partial state");}}rows++;}System.out.println(rows+":independent:static:leaf:early:receiver");}}`

func nativeIndependentLeafInput(t *testing.T, files map[string][]byte) {
	t.Helper()
	o, e := Parse(files["LeafNamespace.class"])
	if e != nil {
		t.Fatal(e)
	}
	// A valid mixed-version archive makes the physical outer unavailable to
	// this source profile, while the original static leaf stays independently
	// representable. The untouched driver runs this original program first.
	o.MajorVersion = 55
	files["LeafNamespace.class"] = o.Bytes()
}
func TestNativeIndependentStaticLeafClosesOwnDeclaration(t *testing.T) {
	files := nativeCompileClasses(t, nativeIndependentLeafFixture)
	nativeIndependentLeafInput(t, files)
	z := nativeArchive(t, files)
	defer z.Close()
	obj, e := Parse(files["LeafNamespace$Leaf.class"])
	if e != nil {
		t.Fatal(e)
	}
	d := z.nativeMemberReader(obj)
	cert := d.originalNativeMemberIndependentRoot()
	if cert == nil {
		t.Fatal("original static boundary missing")
	}
	p := d.planNativeMemberFamilyFromRoot(cert)
	if p == nil || len(p.children) != 0 {
		t.Fatal("certified leaf must be an empty owned family")
	}
	entry := z.nativeMemberIndependentEntry(obj)
	if entry == nil || entry.family == nil || len(entry.source) == 0 {
		t.Fatal("certified leaf declaration not completed")
	}
}
func TestNativeIndependentStaticLeafRetainsConstructorCallback(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "public"} {
		t.Run(variant, func(t *testing.T) {
			fixture := nativeIndependentLeafFixture
			outer, caller := "LeafNamespace", "LeafCaller"
			if variant == "renamed" {
				outer, caller = "OtherStaticScope", "ChangedReceiverScope"
				fixture = strings.NewReplacer("LeafNamespace", outer, "LeafCaller", caller).Replace(fixture)
			}
			if variant == "public" {
				fixture = strings.Replace(fixture, "static abstract class Leaf", "public static abstract class Leaf", 1)
			}
			testNativeIndependentMutatedFamilyFixture(t, fixture, []string{outer + "$Leaf", caller}, "LeafDriver", "80:independent:static:leaf:early:receiver\n", func(t *testing.T, files map[string][]byte) {
				o, e := Parse(files[outer+".class"])
				if e != nil {
					t.Fatal(e)
				}
				o.MajorVersion = 55
				files[outer+".class"] = o.Bytes()
			}, nativeLexicalExactSignatures)
		})
	}
}

func TestNativeIndependentStaticLeafNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for independently compiled legacy declaration")
	}
	version, e := exec.Command(javac, "-version").CombinedOutput()
	if e != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original compiler", e, string(version))
	}
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		dir := t.TempDir()
		path := filepath.Join(dir, "LeafNamespace.java")
		if e := os.WriteFile(path, []byte(nativeIndependentLeafFixture), 0600); e != nil {
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
	}, NativeJavac8, javac, []string{"LeafNamespace$Leaf", "LeafCaller"}, "LeafDriver", "80:independent:static:leaf:early:receiver\n", nativeIndependentLeafInput, nativeLexicalExactSignatures)
}
