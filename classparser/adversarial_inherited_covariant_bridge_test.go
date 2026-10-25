package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const inheritedCovariantBridgeFixture = `class BridgeEffects {static int calls;static boolean fail;static final java.io.IOException error=new java.io.IOException("identity");}
class BridgeBase {public Object read(long n,double real,Object token)throws java.io.IOException {throw new AssertionError("wrong owner");}}
public class BridgeOuter extends BridgeBase {
 static class Leaf extends BridgeOuter {public Leaf(){}}
 public String read(long n,double real,Object token)throws java.io.IOException {BridgeEffects.calls++;if(BridgeEffects.fail)throw BridgeEffects.error;return String.valueOf(n)+":"+Long.toHexString(Double.doubleToRawLongBits(real))+":"+String.valueOf(token);}
 static Object leaf(){return new Leaf();}
}
class BridgeDerived extends BridgeOuter.Leaf {
 public String read(long n,double d,Object v){throw new AssertionError("a proved super bridge became virtual");}
 public Object special(long n,double d,Object v)throws Throwable {
 java.lang.invoke.MethodHandle h=java.lang.invoke.MethodHandles.lookup().findSpecial(BridgeOuter.Leaf.class,"read",java.lang.invoke.MethodType.methodType(Object.class,long.class,double.class,Object.class),BridgeDerived.class);
 return (Object)h.bindTo(this).invokeExact(n,d,v);}}
class BridgeDriver {public static void main(String[]args)throws Exception {
 Object leaf=BridgeOuter.leaf();java.lang.reflect.Method bridge=null;
 for(java.lang.reflect.Method m:Class.forName("BridgeOuter$Leaf").getDeclaredMethods()){if(bridge!=null||!m.isSynthetic()||!m.isBridge()||m.getReturnType()!=Object.class)throw new AssertionError("exact original bridge universe");bridge=m;}
 if(bridge==null)throw new AssertionError("lost inherited bridge");
 if(args.length>0&&args[0].equals("special")){leaf=new BridgeDerived();bridge=BridgeDerived.class.getDeclaredMethod("special",long.class,double.class,Object.class);}
 bridge.setAccessible(true);int rows=0;
 for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(double d:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY})for(Object v:new Object[]{null,"token"})for(boolean fail:new boolean[]{false,true}){
 String expected=String.valueOf(n)+":"+Long.toHexString(Double.doubleToRawLongBits(d))+":"+String.valueOf(v);BridgeEffects.fail=fail;BridgeEffects.calls=0;boolean caught=false;
 try{Object result=bridge.invoke(leaf,n,d,v);if(!expected.equals(result))throw new AssertionError("result");}catch(java.lang.reflect.InvocationTargetException e){if(e.getCause()!=BridgeEffects.error)throw e;caught=true;}
 if(caught!=fail||BridgeEffects.calls!=1)throw new AssertionError("invocation target/effects/exception identity");rows++;
 }System.out.println(rows+":inherited:covariant:bridge:dispatch:wide:exception");}}
`

func TestAdversarialIndependentNestedSubclassKeepsInheritedCovariantBridge(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, root := range []string{"BridgeOuter", "AlternateOuter"} {
			for _, array := range []bool{false, true} {
				for _, parameters := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/array=%t/parameters=%t", debug, root, array, parameters), func(t *testing.T) {
						fixture := strings.ReplaceAll(inheritedCovariantBridgeFixture, "BridgeOuter", root)
						if array {
							fixture = strings.ReplaceAll(fixture, "public String read(", "public String[] read(")
							fixture = strings.Replace(fixture, `return String.valueOf(n)+":"+Long.toHexString(Double.doubleToRawLongBits(real))+":"+String.valueOf(token);`, `return new String[]{String.valueOf(n)+":"+Long.toHexString(Double.doubleToRawLongBits(real))+":"+String.valueOf(token),"tail"};`, 1)
							fixture = strings.Replace(fixture, `if(!expected.equals(result))`, `if(!(result instanceof String[])||((String[])result).length!=2||!expected.equals(((String[])result)[0])||!"tail".equals(((String[])result)[1]))`, 1)
						}
						testIndependentInheritedBridge(t, modern, java, native, fixture, root, debug, parameters, "36:inherited:covariant:bridge:dispatch:wide:exception\n")
					})
				}
			}
		}
	}
}

func testIndependentInheritedBridge(t *testing.T, modern, java, native, fixture, root, debug string, parameters bool, expected string) {
	t.Helper()
	testIndependentInheritedBridgeWithOriginal(t, modern, java, native, fixture, root, debug, parameters, expected, nil)
}

func testIndependentInheritedBridgeWithOriginal(t *testing.T, modern, java, native, fixture, root, debug string, parameters bool, expected string, mutate func(map[string][]byte), verify ...func(*testing.T, string, []byte, []byte, *JarReconstructionArtifact)) {
	testIndependentInheritedBridgeOwner(t, modern, java, native, fixture, root, root+"$Leaf", debug, parameters, expected, mutate, verify...)
}

func testIndependentInheritedBridgeOwner(t *testing.T, modern, java, native, fixture, root, name, debug string, parameters bool, expected string, mutate func(map[string][]byte), verify ...func(*testing.T, string, []byte, []byte, *JarReconstructionArtifact)) {
	t.Helper()
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, root, debug, parameters)
	if mutate != nil {
		mutate(files)
	}
	run := func(dir string) {
		t.Helper()
		for _, mode := range []string{"-ea", "-da"} {
			for _, dispatch := range []string{"virtual", "special"} {
				out, err := enumFreshOracleRun(t, java, mode, dir, "BridgeDriver", dispatch)
				if err != nil || string(out) != expected {
					t.Fatal("unchanged original bridge caller", err, string(out))
				}
			}
		}
	}
	run(sourceArtifactWriteFiles(t, files))
	owned := map[string][]byte{name + ".class": files[name+".class"]}
	if _, _, anonymous := originalAnonymousOwner(mustParseLowering(t, files[name+".class"])); anonymous {
		// The independent flat source reader needs the original lexical owner
		// in its owned archive. Only the requested class is recompiled below;
		// its original owner and unchanged driver remain binary dependencies.
		for n, b := range files {
			if n == root+".class" || strings.HasPrefix(n, root+"$") {
				owned[n] = b
			}
		}
	}
	jar := filepath.Join(t.TempDir(), "input.jar")
	if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(n string) ([]byte, bool) { b, k := files[n+".class"]; return b, k })
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	source, err := a.ReadFile(name + ".class")
	if err != nil || legacySourceHasFailureMarker(string(source)) {
		t.Fatal("independent source", err, string(source))
	}
	external := map[string][]byte{}
	for n, b := range files {
		if n != name+".class" {
			external[n] = b
		}
	}
	out := sourceArtifactWriteFiles(t, external)
	path := filepath.Join(out, name+".java")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
		t.Fatal("fresh independent source", err, string(log), string(source))
	}
	path = filepath.Join(out, name+".class")
	fresh, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("missing independently compiled target", err, string(source))
	}
	saved := bytes.Clone(fresh)
	for _, check := range verify {
		check(t, name, owned[name+".class"], fresh, a)
	}
	finished, err := a.FinishClass(name+".class", fresh)
	if err != nil {
		t.Fatal("original inherited bridge method universe", err, string(source), "publication", a.source.jar.sourceConstructorLowering.bridgeActive)
	}
	if !bytes.Equal(saved, fresh) {
		t.Fatal("input mutation")
	}
	if again, err := a.FinishClass(name+".class", finished); err != nil || !bytes.Equal(again, finished) {
		t.Fatal("idempotence", err)
	}
	if err := os.WriteFile(path, finished, 0600); err != nil {
		t.Fatal(err)
	}
	run(out)
}

func TestAdversarialIndependentInheritedBridgeGroupKeepsEverySuperBinding(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	fixture := strings.Split(inheritedCovariantBridgeFixture, "class BridgeDriver")[0]
	fixture = strings.Replace(fixture, "public class BridgeOuter extends BridgeBase", "interface BridgeText {CharSequence read(long n,double d,Object v)throws java.io.IOException;}\npublic class BridgeOuter extends BridgeBase implements BridgeText", 1)
	fixture = strings.Replace(fixture, "invokeExact(n,d,v);}}", `invokeExact(n,d,v);}
 public Object specialText(long n,double d,Object v)throws Throwable {
 java.lang.invoke.MethodHandle h=java.lang.invoke.MethodHandles.lookup().findSpecial(BridgeOuter.Leaf.class,"read",java.lang.invoke.MethodType.methodType(CharSequence.class,long.class,double.class,Object.class),BridgeDerived.class);
 return (CharSequence)h.bindTo(this).invokeExact(n,d,v);}}`, 1)
	fixture += `class BridgeDriver {public static void main(String[]args)throws Exception {
 java.lang.reflect.Method[] methods=Class.forName("BridgeOuter$Leaf").getDeclaredMethods();
 if(methods.length!=2)throw new AssertionError("exact bridge group universe");
 int rows=0;boolean object=false,text=false;
 for(java.lang.reflect.Method method:methods){
 if(!method.isSynthetic()||!method.isBridge())throw new AssertionError("ordinary declaration leaked");
 if(method.getReturnType()==Object.class){if(object)throw new AssertionError("duplicate");object=true;}
 else if(method.getReturnType()==CharSequence.class){if(text)throw new AssertionError("duplicate");text=true;}
 else throw new AssertionError("unexpected return");
 Object leaf=BridgeOuter.leaf();java.lang.reflect.Method bridge=method;
 if(args.length>0&&args[0].equals("special")){leaf=new BridgeDerived();bridge=BridgeDerived.class.getDeclaredMethod(method.getReturnType()==Object.class?"special":"specialText",long.class,double.class,Object.class);}
 bridge.setAccessible(true);
 for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(double d:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY})for(Object v:new Object[]{null,"token"})for(boolean fail:new boolean[]{false,true}){
 String expected=String.valueOf(n)+":"+Long.toHexString(Double.doubleToRawLongBits(d))+":"+String.valueOf(v);BridgeEffects.fail=fail;BridgeEffects.calls=0;boolean caught=false;
 try{Object result=bridge.invoke(leaf,n,d,v);if(!expected.equals(result))throw new AssertionError("result");}catch(java.lang.reflect.InvocationTargetException e){if(e.getCause()!=BridgeEffects.error)throw e;caught=true;}
 if(caught!=fail||BridgeEffects.calls!=1)throw new AssertionError("group target/effects/exception identity");rows++;
 }}if(!object||!text)throw new AssertionError("missing bridge");System.out.println(rows+":inherited:bridge:group:super:identity");}}
`
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, root := range []string{"BridgeOuter", "AlternateOuter"} {
			t.Run(debug+"/"+root, func(t *testing.T) {
				testIndependentInheritedBridge(t, modern, java, native, strings.ReplaceAll(fixture, "BridgeOuter", root), root, debug, true, "72:inherited:bridge:group:super:identity\n")
			})
		}
	}
}
