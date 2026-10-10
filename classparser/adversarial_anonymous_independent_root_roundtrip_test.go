package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Original javac8's final static anonymous root remains an independent final
// declaration. Its nonfinal child must be regenerated inside that declaration
// so a superclass callback observes the original pre-SUPER capture stores.
func TestAdversarialAnonymousIndependentRootKeepsEarlyCapturedReceiver(t *testing.T) {
	fixtures := []struct{ name, source, want string }{
		{"identity", `abstract class ShellParent{final Object seen;ShellParent(Object value){seen=read();if(seen!=value)throw new AssertionError("captured receiver before SUPER");}abstract Object read();}interface ShellFactory{ShellParent make();}class ShellOwner{static ShellFactory factory(final Object token){return new ShellFactory(){Object marker(){return token;}public ShellParent make(){return new ShellParent(token){Object read(){return marker();}};}};}}
class ShellDriver{public static void main(String[]args){int rows=0;Object marker=new Object();for(Object token:new Object[]{null,marker,"same",new String("same")}){ShellFactory f=ShellOwner.factory(token);ShellParent p;try{p=f.make();}catch(NullPointerException e){throw new AssertionError("early captured receiver timing",e);}if(!p.getClass().isAnonymousClass()||p.getClass().getEnclosingClass()!=f.getClass()||!p.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("final root and native child ownership");if(p.seen!=token||p.read()!=token)throw new AssertionError("root identity and observation");rows++;}System.out.println(rows+":standalone:anonymous:root");}}`, "4:standalone:anonymous:root\n"},
		{"generic covariant arrays", `abstract class ShellParent<T>{final T seen;ShellParent(T value){seen=read();if(seen!=value)throw new AssertionError("generic array captured receiver before SUPER");}abstract T read();}interface ShellFactory<T>{Object make();}class ShellOwner{static ShellFactory<String[]> factory(final String[] token){return new ShellFactory<String[]>(){String[] marker(){return token;}public ShellParent<String[]> make(){return new ShellParent<String[]>(token){String[] read(){return marker();}};}};}}
class ShellDriver{public static void main(String[]args){int rows=0;for(String[]token:new String[][]{null,new String[0],new String[]{null},new String[]{"a","\u0000","中文"}}){ShellFactory<String[]>f=ShellOwner.factory(token);ShellParent<?>p;try{p=(ShellParent<?>)f.make();}catch(NullPointerException e){throw new AssertionError("early captured receiver timing",e);}if(p.seen!=token||p.read()!=token)throw new AssertionError("generic array identity and covariant bridge");rows++;}System.out.println(rows+":standalone:generic:array");}}`, "4:standalone:generic:array\n"},
		{"wide words", `abstract class ShellParent{final long seen;ShellParent(long value){seen=read();if(seen!=value)throw new AssertionError("wide captured receiver before SUPER");}abstract long read();}interface ShellFactory{ShellParent make();}class ShellOwner{static ShellFactory factory(final long left,final double right){return new ShellFactory(){long marker(){return left^Double.doubleToRawLongBits(right);}public ShellParent make(){return new ShellParent(marker()){long read(){return marker();}};}};}}
class ShellDriver{public static void main(String[]args){int rows=0;for(long left:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long bits:new long[]{0L,Long.MIN_VALUE,0x7ff0000000000000L,0x7ff8000000000042L,0xfff0000000000000L}){double right=Double.longBitsToDouble(bits);ShellFactory f=ShellOwner.factory(left,right);ShellParent p;try{p=f.make();}catch(NullPointerException e){throw new AssertionError("early captured receiver timing",e);}if(p.seen!=(left^bits)||p.read()!=(left^bits))throw new AssertionError("wide word identity");rows++;}System.out.println(rows+":standalone:wide:root");}}`, "25:standalone:wide:root\n"},
		{"two children", `abstract class ShellParent{final Object seen;ShellParent(Object value){seen=read();if(seen!=value)throw new AssertionError("two-child captured receiver before SUPER");}abstract Object read();}interface ShellFactory{ShellParent make();ShellParent other();}class ShellOwner{static ShellFactory factory(final Object left,final Object right){return new ShellFactory(){Object first(){return left;}Object second(){return right;}public ShellParent make(){return new ShellParent(left){Object read(){return first();}};}public ShellParent other(){return new ShellParent(right){Object read(){return second();}};}};}}
class ShellDriver{public static void main(String[]args){int rows=0;Object same=new Object();for(Object left:new Object[]{null,same,"same",new String("same")})for(Object right:new Object[]{null,same,"same",new String("same")}){ShellFactory f=ShellOwner.factory(left,right);ShellParent a,b;try{a=f.make();b=f.other();}catch(NullPointerException e){throw new AssertionError("early captured receiver timing",e);}if(a.seen!=left||a.read()!=left||b.seen!=right||b.read()!=right||a.getClass()==b.getClass()||!a.getClass().getEnclosingMethod().getName().equals("make")||!b.getClass().getEnclosingMethod().getName().equals("other"))throw new AssertionError("equal-typed distinct captures and original registration");rows++;}System.out.println(rows+":standalone:two:root");}}`, "16:standalone:two:root\n"},
	}
	for _, root := range []string{"ShellOwner", "RenamedIndependentOwner"} {
		for _, level := range []string{"7", "8"} {
			for _, fixture := range fixtures {
				t.Run(root+"/source"+level+"/"+fixture.name, func(t *testing.T) {
					source := strings.ReplaceAll(fixture.source, "ShellOwner", root)
					testNativePrivateSetterCompiledFixtureWithShape(t, root, "ShellDriver", fixture.want, func(t *testing.T, debug string) map[string][]byte {
						return nativeCompileIndependentRootFixture(t, root, source, debug, level)
					}, nativeIndependentRootRepresentationShape)
				})
			}
		}
	}
}

func nativeCompileIndependentRootFixture(t *testing.T, root, fixture, debug, level string) map[string][]byte {
	t.Helper()
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for independently compiled legacy anonymous lowering")
	}
	version, err := exec.Command(compiler, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original javac8 required", err, string(version))
	}
	dir := t.TempDir()
	source := filepath.Join(dir, root+".java")
	if err := os.WriteFile(source, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(compiler, "-proc:none", "-source", level, "-target", level, "-g:"+debug, "-d", dir, source).CombinedOutput(); err != nil {
		t.Fatal("original compile", err, string(log))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".class") {
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			files[entry.Name()] = raw
		}
	}
	obj, err := Parse(files[root+"$1.class"])
	major := uint16(51)
	if level == "8" {
		major = 52
	}
	if err != nil || obj.AccessFlags != 0x30 || obj.MajorVersion != major {
		t.Fatal("genuine original static final anonymous lowering", err)
	}
	return files
}

// Only the independently materialized final root has the established flat
// representation differences. Every regenerated child retains exact binary
// capture ABI; accepting a flat root is not a waiver for a native child.
func nativeIndependentRootRepresentationShape(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	old, err := Parse(original)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, anonymous := originalAnonymousOwner(old); anonymous && old.AccessFlags == 0x0030 {
		nativeAnonymousPrefixRepresentationShape(t, name, original, rebuilt)
		return
	}
	if a, b := nativeBinaryShape(t, original), nativeBinaryShape(t, rebuilt); a != b {
		t.Fatalf("exact child ABI %s\n%s\n%s", name, a, b)
	}
	if a, b := nativeAnonymousAccessorShape(t, original), nativeAnonymousAccessorShape(t, rebuilt); a != b {
		t.Fatalf("exact child ownership %s\n%s\n%s", name, a, b)
	}
}
