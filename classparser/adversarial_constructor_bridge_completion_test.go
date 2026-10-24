package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Native javac8 needs an access constructor with an unused marker argument.
// A later compiler can use nest access instead, but binary callers and
// reflection still observe the original constructor universe. The same driver
// invokes both the reconstructed factory and the ORIGINAL bridge descriptor.
func TestAdversarialConstructorAccessBridgeSurvivesCompilerProtocolChange(t *testing.T) {
	for _, owner := range []string{"AccessOwner", "RenamedAccessOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, target := range []int{8, 11, 16} {
				t.Run(fmt.Sprintf("%s/%s/target%d", owner, debug, target), func(t *testing.T) {
					fixture := `class AccessEffects {static String trace="";static Object published;static boolean fail;static final java.io.IOException failure=new java.io.IOException("same");static long argument(long n){trace+="A";return n;}}
class ` + owner + ` {static class Child {final Object value;final long number;private Child(Object value,long number)throws java.io.IOException{AccessEffects.trace+="P";AccessEffects.published=this;if(AccessEffects.fail)throw AccessEffects.failure;this.value=value;this.number=number;AccessEffects.trace+="C";}}static Child make(Object v,long n)throws java.io.IOException{return new Child(v,AccessEffects.argument(n));}}
class AccessDriver {public static void main(String[]args)throws Exception{Class<?>marker=Class.forName("` + owner + `$1");java.lang.reflect.Constructor<?>bridge=` + owner + `.Child.class.getDeclaredConstructor(Object.class,long.class,marker);bridge.setAccessible(true);if(!bridge.isSynthetic()||bridge.getModifiers()!=0x1000||` + owner + `.Child.class.getDeclaredConstructors().length!=2||bridge.getExceptionTypes().length!=1||bridge.getExceptionTypes()[0]!=java.io.IOException.class)throw new AssertionError("bridge contract");Object token=new Object();int rows=0;for(boolean reflect:new boolean[]{false,true})for(boolean fail:new boolean[]{false,true})for(Object value:new Object[]{null,token,"text"})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){AccessEffects.trace="";AccessEffects.published=null;AccessEffects.fail=fail;try{` + owner + `.Child c=reflect?(` + owner + `.Child)bridge.newInstance(value,n,null):` + owner + `.make(value,n);if(fail||c!=AccessEffects.published||c.value!=value||c.number!=n||!AccessEffects.trace.equals(reflect?"PC":"APC"))throw new AssertionError("value/effect order");}catch(Throwable e){if(e instanceof java.lang.reflect.InvocationTargetException)e=e.getCause();` + owner + `.Child c=(` + owner + `.Child)AccessEffects.published;if(!fail||e!=AccessEffects.failure||c==null||c.value!=null||c.number!=0||!AccessEffects.trace.equals(reflect?"P":"AP"))throw new AssertionError("exception/publication",e);}rows++;}System.out.println(rows+":constructor:bridge:descriptor:identity:effects");}}`
					roundTripConstructorBridgeArtifact(t, fixture, owner, "AccessDriver", "36:constructor:bridge:descriptor:identity:effects\n", debug, target)
				})
			}
		}
	}
}

// Every fixture first runs its authored original, then compiles an owning
// source family with no original family classes on its classpath.
func roundTripConstructorBridgeArtifact(t *testing.T, fixture, owner, driver, oracle, debug string, target int) {
	t.Helper()
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, false)
	run := func(dir string) {
		t.Helper()
		out, err := exec.Command(java, "-Xverify:all", "-cp", dir, driver).CombinedOutput()
		if err != nil || string(out) != oracle {
			t.Fatal("original-first constructor bridge oracle", err, string(out))
		}
	}
	run(sourceArtifactWriteFiles(t, files))
	owned := map[string][]byte{}
	support := map[string][]byte{}
	for name, raw := range files {
		if name == owner+".class" || strings.HasPrefix(name, owner+"$") {
			owned[name] = raw
		} else {
			support[name] = raw
		}
	}
	path := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(path, t23Zip(t, owned), 0600); err != nil {
		t.Fatal(err)
	}
	resolve := func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
	artifact, err := NewJarReconstructionArtifactFromLocal(path, target, ModernJavac, resolve)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	names := make([]string, 0, len(owned))
	for name := range owned {
		names = append(names, name)
	}
	sort.Strings(names)
	out := sourceArtifactWriteFiles(t, support)
	var sources []string
	for _, name := range names {
		source, err := artifact.ReadFile(name)
		if err != nil || legacySourceHasFailureMarker(string(source)) {
			t.Fatal("complete owning source", name, err, string(source))
		}
		file := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
		if err := os.WriteFile(file, source, 0600); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, file)
	}
	args := []string{"-proc:none", "--release", fmt.Sprint(target), "-g:" + debug, "-cp", out, "-sourcepath", out, "-d", out}
	args = append(args, sources...)
	if log, err := exec.Command(modern, args...).CombinedOutput(); err != nil {
		t.Fatal("fresh source family", err, string(log))
	}
	// Complete every owned output, including the retained empty marker. The
	// original family is deliberately absent from the rebuild/runtime classpath.
	for _, name := range names {
		fresh, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal("missing original binary", name, err)
		}
		before := bytes.Clone(fresh)
		result, err := artifact.FinishClass(name, fresh)
		if err != nil {
			t.Fatal("exact constructor universe", name, err)
		}
		if !bytes.Equal(fresh, before) {
			t.Fatal("fresh input mutated")
		}
		if err := os.WriteFile(filepath.Join(out, name), result, 0600); err != nil {
			t.Fatal(err)
		}
	}
	run(out)
}

func TestAdversarialConstructorBridgeModernTargetsKeepGenericAndEnclosingBinding(t *testing.T) {
	for _, kind := range []string{"generic member", "nonstatic member", "root wide overload", "root generic varargs"} {
		fixture, owner, driver, oracle := nativeMemberPrivateBridgeFixture, "BridgeOwner", "BridgeDriver", "18:APC\n"
		if kind == "nonstatic member" {
			fixture = strings.Replace(fixture, "static class Child<U>", "class Child<U>", 1)
			fixture = strings.ReplaceAll(fixture, "BridgeOwner.Child<Object>", "BridgeOwner<Object>.Child<Object>")
			fixture = strings.ReplaceAll(fixture, "BridgeOwner.Child<?>", "BridgeOwner<?>.Child<?>")
			fixture = strings.Replace(fixture, "getDeclaredConstructor(Object.class,long.class)", "getDeclaredConstructor(BridgeOwner.class,Object.class,long.class)", 1)
		}
		if kind == "root wide overload" {
			fixture, owner, driver, oracle = nativeRootWideConstructorFixture, "WideBridgePacket", "WideBridgeDriver", "9:wide:overload:order:owner\n"
		}
		if kind == "root generic varargs" {
			fixture, owner, driver, oracle = nativeRootGenericConstructorFixture, "GenericBridgePacket", "GenericBridgeDriver", "3:generic:varargs:checked:owner\n"
		}
		for _, target := range []int{11, 16} {
			t.Run(fmt.Sprintf("%s/target%d", kind, target), func(t *testing.T) {
				roundTripConstructorBridgeArtifact(t, fixture, owner, driver, oracle, "none", target)
			})
		}
	}
}
