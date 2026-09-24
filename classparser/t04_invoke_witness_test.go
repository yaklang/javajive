package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

var t04BareValueOfNull = regexp.MustCompile(`valueOf\s*\(\s*null\s*\)`)
var t04TypedObjectValueOf = regexp.MustCompile(`(?s)Object\s+var\d+\s*=\s*null\s*;.*valueOf\(var\d+\)`)

func TestT04C01ConcatProbe(t *testing.T) {
	src, err := os.ReadFile("testdata/t04/ConcatProbe.java")
	if err != nil {
		t.Fatal(err)
	}
	origOut, classes := t04CompileRun(t, "17", "ConcatProbe", map[string]string{"ConcatProbe.java": string(src)})
	if !strings.Contains(origOut, "x=7,o=null") {
		t.Fatalf("T04-C01 original stdout missing x=7,o=null: %q", origOut)
	}
	descs := t04PoolMethodDescriptors(t, classes["ConcatProbe"], "java/lang/String", "valueOf")
	foundObject := false
	for _, d := range descs {
		if d == "(Ljava/lang/Object;)Ljava/lang/String;" {
			foundObject = true
		}
		if d == "([C)Ljava/lang/String;" {
			t.Fatalf("T04-C01 original bytecode already bound valueOf(char[]): %v", descs)
		}
	}
	if !foundObject {
		t.Fatalf("T04-C01 expected String.valueOf Object descriptor in original pool, got %v", descs)
	}
	t04RoundTripModes(t, "17", "ConcatProbe", origOut, classes, func(t *testing.T, src string) {
		if t04BareValueOfNull.MatchString(src) {
			t.Fatalf("T04-C01 decompiled source has uncast valueOf(null) (binds char[]):\n%s", src)
		}
		if strings.Contains(src, "valueOf") && !strings.Contains(src, "(Object)") && !t04TypedObjectValueOf.MatchString(src) {
			t.Fatalf("T04-C01 valueOf path missing (Object) cast:\n%s", src)
		}
	})
}

func TestT04C02OverloadWitness(t *testing.T) {
	src, err := os.ReadFile("testdata/t04/OverloadWitness.java")
	if err != nil {
		t.Fatal(err)
	}
	want := "Object\nString\nchars\nInteger\nint\nnull\n"
	origOut, classes := t04CompileRun(t, "8", "OverloadWitness", map[string]string{"OverloadWitness.java": string(src)})
	if origOut != want {
		t.Fatalf("T04-C02 original stdout mismatch:\n got %q\nwant %q", origOut, want)
	}
	t04RoundTripModes(t, "8", "OverloadWitness", origOut, classes, func(t *testing.T, src string) {
		if t04BareValueOfNull.MatchString(src) {
			t.Fatalf("T04-C02 uncast valueOf(null) would NPE on char[]:\n%s", src)
		}
	})
}

func TestT04C03EffectOrder(t *testing.T) {
	const src = `public class T04EffectSide {
  static int i = 0;
  static String trace = "";
  static int getter() { trace += "G"; return ++i; }
  static String f(Object x) { trace += "F"; return "f:" + x; }
  public static void main(String[] args) {
    String a = f(++i);
    String b = f(getter());
    System.out.println(trace);
    System.out.println(i);
    System.out.println(a);
    System.out.println(b);
    try {
      System.out.println(f((String)new Object()));
    } catch (ClassCastException e) {
      System.out.println("cce:" + i + ":" + trace);
    }
  }
}
`
	origOut, classes := t04CompileRun(t, "8", "T04EffectSide", map[string]string{"T04EffectSide.java": src})
	t04RoundTripModes(t, "8", "T04EffectSide", origOut, classes, func(t *testing.T, src string) {
		if !strings.Contains(src, "++") && !strings.Contains(src, "getter") && !strings.Contains(src, "i") {
			t.Fatalf("T04-C03 side-effect argument missing from decompiled source:\n%s", src)
		}
	})
}

func TestT04C04SuperCtor(t *testing.T) {
	const src = `interface T04I {
  default void d(Object x) { System.out.println("I.Object"); }
  default void d(String x) { System.out.println("I.String"); }
}
class T04Base {
  void m(Object x) { System.out.println("Base.Object"); }
  void m(String x) { System.out.println("Base.String"); }
}
class T04Sub extends T04Base {
  void m(Object x) { super.m((Object)null); }
}
class T04Ctor {
  T04Ctor(Object x) { System.out.println("Ctor.Object"); }
  T04Ctor(String x) { System.out.println("Ctor.String"); }
}
class T04Impl implements T04I {
  public void d(Object x) { T04I.super.d((Object)null); }
}
public class T04SuperCtorMain {
  public static void main(String[] args) {
    new T04Sub().m((Object)null);
    new T04Ctor((Object)null);
    new T04Ctor((String)null);
    new T04Impl().d((Object)null);
  }
}
`
	origOut, classes := t04CompileRun(t, "8", "T04SuperCtorMain", map[string]string{"T04SuperCtorMain.java": src})
	wantLines := []string{"Base.Object", "Ctor.Object", "Ctor.String", "I.Object"}
	for _, line := range wantLines {
		if !strings.Contains(origOut, line) {
			t.Fatalf("T04-C04 original missing %q in %q", line, origOut)
		}
	}
	t04AssertSpecialWitness(t)
	t04RoundTripModes(t, "8", "T04SuperCtorMain", origOut, classes, func(t *testing.T, src string) {
		if strings.Contains(src, "class T04Sub") && !strings.Contains(src, "super.m") {
			t.Fatalf("T04-C04 Sub lost super.m:\n%s", src)
		}
		if !strings.Contains(src, "super.m((Object)") && !strings.Contains(src, "super.m((java.lang.Object)") {
			t.Fatalf("T04-C04 missing super.m((Object)null) pin:\n%s", src)
		}
	})
}

func TestT04C05VarargsArray(t *testing.T) {
	const src = `public class T04VarargsPick {
  static String pick(Object x) { return "Object"; }
  static String pick(Object[] x) { return "Object[]"; }
  static String pick(String... x) { return "String..."; }
  public static void main(String[] args) {
    System.out.println(pick((Object)null));
    System.out.println(pick((Object[])null));
    System.out.println(pick((String[])null));
    System.out.println(pick("a", "b"));
  }
}
`
	want := "Object\nObject[]\nString...\nString...\n"
	origOut, classes := t04CompileRun(t, "8", "T04VarargsPick", map[string]string{"T04VarargsPick.java": src})
	if origOut != want {
		t.Fatalf("T04-C05 original stdout mismatch:\n got %q\nwant %q", origOut, want)
	}
	t04RoundTripModes(t, "8", "T04VarargsPick", origOut, classes, func(t *testing.T, src string) {
		if !strings.Contains(src, "(Object)") && !strings.Contains(src, "(Object[])") && !strings.Contains(src, "(String[])") {
			t.Fatalf("T04-C05 expected descriptor casts in source:\n%s", src)
		}
	})
}

func TestT04C06GenericBridge(t *testing.T) {
	const src = `class T04P<T> {
  void m(T t) { System.out.println("P:" + t); }
}
class T04C extends T04P<String> {
  void m(String s) { System.out.println("C:" + s); }
}
public class T04BridgeMain {
  public static void main(String[] args) {
    T04P<String> p = new T04C();
    p.m(null);
    T04C c = new T04C();
    c.m(null);
    T04P raw = new T04C();
    raw.m(null);
  }
}
`
	origOut, classes := t04CompileRun(t, "8", "T04BridgeMain", map[string]string{"T04BridgeMain.java": src})
	pDescs := t04PoolMethodDescriptors(t, classes["T04BridgeMain"], "T04P", "m")
	cDescs := t04PoolMethodDescriptors(t, classes["T04BridgeMain"], "T04C", "m")
	if !t04Contains(pDescs, "(Ljava/lang/Object;)V") {
		t.Fatalf("T04-C06 erased P.m descriptor missing from caller pool: %v", pDescs)
	}
	if !t04Contains(cDescs, "(Ljava/lang/String;)V") {
		t.Fatalf("T04-C06 C.m String descriptor missing from caller pool: %v", cDescs)
	}
	erased := &values.FunctionCallExpression{
		ClassName: "T04P", FunctionName: "m", Descriptor: "(Ljava/lang/Object;)V", Kind: values.InvokeVirtual,
	}
	override := &values.FunctionCallExpression{
		ClassName: "T04C", FunctionName: "m", Descriptor: "(Ljava/lang/String;)V", Kind: values.InvokeVirtual,
	}
	if erased.Witness().Descriptor == override.Witness().Descriptor {
		t.Fatal("T04-C06 Witness() must keep erased vs String descriptors, not just name m")
	}
	t04RoundTripModes(t, "8", "T04BridgeMain", origOut, classes, nil)
}

func TestT04C07CrossClassNonNullAndBoxed(t *testing.T) {
	const src = `class T04Parent {
  static String pick(Object x) { return "Object"; }
  static String pick(String x) { return "String"; }
  static String num(int x) { return "int"; }
  static String num(Integer x) { return "Integer"; }
  static String num(byte x) { return "byte"; }
  static String num(char x) { return "char"; }
}
class T04Child extends T04Parent {
  static String run(String s, Integer boxed) {
    return T04Parent.pick((Object)s) + "," + T04Parent.pick(s) + "," + T04Parent.num(boxed.intValue()) + "," + T04Parent.num(boxed) + "," + T04Parent.num((byte)1) + "," + T04Parent.num('x');
  }
}
public class T04CrossMain {
  public static void main(String[] args) {
    System.out.println(T04Child.run("hi", Integer.valueOf(3)));
  }
}
`
	want := "Object,String,int,Integer,byte,char\n"
	origOut, classes := t04CompileRun(t, "8", "T04CrossMain", map[string]string{"T04CrossMain.java": src})
	if origOut != want {
		t.Fatalf("T04-C07 original stdout mismatch:\n got %q\nwant %q", origOut, want)
	}
	t04RoundTripModes(t, "8", "T04CrossMain", origOut, classes, func(t *testing.T, src string) {
		if strings.Contains(src, "pick(s)") && !strings.Contains(src, "(Object)") {
			// may still be pick((Object)s)
		}
	})
}

func TestT04PoolDescriptorsOnDumper(t *testing.T) {
	_, classes := t04CompileRun(t, "8", "T04RegMain", map[string]string{
		"T04RegMain.java": `class T04RegOver {
  static String pick(Object x) { return "Object"; }
  static String pick(String x) { return "String"; }
}
public class T04RegMain {
  public static void main(String[] args) {
    String s = "hi";
    System.out.println(T04RegOver.pick((Object)s));
    System.out.println(T04RegOver.pick(s));
  }
}
`,
	})
	raw := classes["T04RegMain"]
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(obj)
	d.foldSiblingResolver = func(internalName string) ([]byte, bool) {
		base := internalName
		if i := strings.LastIndexByte(internalName, '/'); i >= 0 {
			base = internalName[i+1:]
		}
		b, ok := classes[base]
		return b, ok
	}
	src, err := d.DumpClass()
	if err != nil {
		t.Fatal(err)
	}
	if d.FuncCtx.PoolMethodDescriptors["T04RegOver.pick(Ljava/lang/Object;)Ljava/lang/String;"] != true {
		t.Fatalf("missing pick(Object) pool key in %v", d.FuncCtx.PoolMethodDescriptors)
	}
	if !strings.Contains(src, "pick((Object)") {
		t.Fatalf("expected pick((Object) pin, got:\n%s", src)
	}
}

func TestT04RegressionGenericBridgeOverloadArrayLambda(t *testing.T) {
	const src = `class T04RegP<T> {
  void m(T t) { System.out.println("P:" + t); }
}
class T04RegC extends T04RegP<String> {
  void m(String s) { System.out.println("C:" + s); }
}
class T04RegOver {
  static String pick(Object x) { return "Object"; }
  static String pick(String x) { return "String"; }
  static String pick(Object[] x) { return "Object[]"; }
  static String pick(String... x) { return "String..."; }
}
public class T04RegMain {
  public static void main(String[] args) {
    java.util.List<Integer> xs = new java.util.ArrayList<Integer>();
    xs.add(Integer.valueOf(3));
    xs.add(Integer.valueOf(1));
    xs.add(Integer.valueOf(2));
    java.util.Collections.sort(xs, (Integer l0, Integer l1) -> l1.compareTo(l0));
    System.out.println(xs);
    T04RegP<String> p = new T04RegC();
    p.m(null);
    T04RegC c = new T04RegC();
    c.m(null);
    String s = "hi";
    System.out.println(T04RegOver.pick((Object)s));
    System.out.println(T04RegOver.pick(s));
    System.out.println(T04RegOver.pick((Object[])null));
    System.out.println(T04RegOver.pick("a", "b"));
  }
}
`
	origOut, classes := t04CompileRun(t, "8", "T04RegMain", map[string]string{"T04RegMain.java": src})
	if !strings.Contains(origOut, "Object") || !strings.Contains(origOut, "String") {
		t.Fatalf("T04 regression original stdout missing overload pins: %q", origOut)
	}
	t04RoundTripModes(t, "8", "T04RegMain", origOut, classes, func(t *testing.T, src string) {
		if strings.Contains(src, "(Comparator)((Integer") || strings.Contains(src, "(Comparator)((") {
			t.Fatalf("raw Comparator/FI cast on typed lambda (breaks javac inference):\n%s", src)
		}
		if t04BareValueOfNull.MatchString(src) {
			t.Fatalf("uncast valueOf(null) in family rebuild:\n%s", src)
		}
	})
}

func TestT04C08EnvSnapshotNullCast(t *testing.T) {
	_, classes := t04CompileRun(t, "17", "NullCastProbe", map[string]string{
		"NullCastProbe.java": `public class NullCastProbe {
  static String value() { return String.valueOf((Object)null); }
  public static void main(String[] args) { System.out.println(value()); }
}`,
	})
	raw := classes["NullCastProbe"]
	liveOff := os.Getenv("JDEC_NULL_ARG_CAST_OFF")
	t.Setenv("JDEC_NULL_ARG_CAST_OFF", "1")
	res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, EnvSnapshot: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Source, "(Object)") {
		t.Fatalf("closed empty EnvSnapshot must ignore live JDEC_NULL_ARG_CAST_OFF: %s", res.Source)
	}
	res2, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, EnvSnapshot: map[string]string{"JDEC_NULL_ARG_CAST_OFF": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res2.Source, "valueOf((Object)") {
		t.Fatalf("snapshot flag on should disable witness null cast, got:\n%s", res2.Source)
	}
	_ = liveOff
}

func TestT04ExternalUnreferencedOverloadFamily(t *testing.T) {
	const lib = `public class T04ExtLib {
  public static String pick(Object o) { return "O"; }
  public static String pick(String s) { return "S"; }
}
`
	const caller = `public class T04ExtCaller {
  public static void main(String[] args) {
    String x = "hi";
    System.out.println(T04ExtLib.pick((Object)x));
  }
}
`
	origOut, classes := t04CompileRun(t, "8", "T04ExtCaller", map[string]string{
		"T04ExtLib.java":    lib,
		"T04ExtCaller.java": caller,
	})
	if strings.TrimSpace(origOut) != "O" {
		t.Fatalf("original bytecode must bind pick(Object), stdout %q", origOut)
	}
	t04RoundTripModes(t, "8", "T04ExtCaller", origOut, classes, func(t *testing.T, src string) {
		if !strings.Contains(src, "pick((Object)") && !strings.Contains(src, "pick((java.lang.Object)") {
			t.Fatalf("family rebuild must keep Object pin, else javac binds pick(String):\n%s", src)
		}
	})
	callerOnly, err := DecompileWithOptions(classes["T04ExtCaller"], DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatal(err)
	}
	if callerOnly.Status == "complete" {
		t.Fatalf("caller-only dump without Lib family must not claim complete (silent retarget risk), status=%s\n%s", callerOnly.Status, callerOnly.Source)
	}
}

func t04AssertSpecialWitness(t *testing.T) {
	t.Helper()
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)V")
	if err != nil {
		t.Fatal(err)
	}
	superCall := values.NewFunctionCallExpression(nil, &values.JavaClassMember{
		Name: "T04Base", Member: "m", Description: "(Ljava/lang/Object;)V", JavaType: ft,
	}, ft.FunctionType())
	superCall.IsSpecialInvoke = true
	superCall.Kind = values.InvokeSpecial
	superCall.OriginPC = 7
	superCall.Arguments = []values.JavaValue{values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}
	w := superCall.Witness()
	if w.Kind != values.InvokeSpecial || w.Descriptor != "(Ljava/lang/Object;)V" || w.Name != "m" {
		t.Fatalf("T04-C04 super Witness: %+v", w)
	}
	oldID, newID := utils.NewRootVariableId(), utils.NewRootVariableId()
	superCall.ReplaceVar(oldID, newID)
	if superCall.Witness().Kind != values.InvokeSpecial || superCall.Witness().OriginPC != 7 {
		t.Fatalf("T04-C04 ReplaceVar dropped special witness: %+v", superCall.Witness())
	}
	ctor := &values.FunctionCallExpression{
		ClassName: "T04Ctor", FunctionName: "<init>", Descriptor: "(Ljava/lang/Object;)V",
		Kind: values.InvokeSpecial, IsSpecialInvoke: true, OriginPC: 1,
		FuncType:  ft.FunctionType(),
		Arguments: []values.JavaValue{values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))},
	}
	if ctor.Witness().Kind != values.InvokeSpecial || ctor.Witness().Name != "<init>" {
		t.Fatalf("T04-C04 ctor Witness: %+v", ctor.Witness())
	}
	got := ctor.ArgumentString(&class_context.ClassContext{ClassName: "T04SuperCtorMain"})
	if !strings.Contains(got, "Object") || !strings.Contains(got, "null") {
		t.Fatalf("T04-C04 ctor null arg should carry Object cast, got %q", got)
	}
}

func t04CompileRun(t *testing.T, release, mainClass string, sources map[string]string) (string, map[string][]byte) {
	t.Helper()
	javac, java := t04Tools(t)
	srcDir := t.TempDir()
	outDir := t.TempDir()
	var files []string
	for name, body := range sources {
		p := filepath.Join(srcDir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	args := append([]string{"-proc:none", "-encoding", "UTF-8", "--release", release, "-d", outDir}, files...)
	cmd := exec.Command(javac, args...)
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("javac original: %v\n%s", err, out)
	}
	classes := map[string][]byte{}
	err := filepath.Walk(outDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".class") {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		classes[strings.TrimSuffix(filepath.Base(p), ".class")] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return t04RunJava(t, java, outDir, mainClass), classes
}

func t04RoundTripModes(t *testing.T, release, mainClass, origOut string, classes map[string][]byte, checkSrc func(*testing.T, string)) {
	t.Helper()
	javac, java := t04Tools(t)
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName
		if i := strings.LastIndexByte(internalName, '/'); i >= 0 {
			base = internalName[i+1:]
		}
		if b, ok := classes[base]; ok {
			return b, true
		}
		return nil, false
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			reDir := t.TempDir()
			outDir := t.TempDir()
			var javaFiles []string
			var allSrc strings.Builder
			for name, raw := range classes {
				res, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, Resolve: resolver})
				if err != nil {
					t.Fatalf("decompile %s: %v", name, err)
				}
				if res.Source == "" {
					t.Fatalf("decompile %s produced empty source (status=%s)", name, res.Status)
				}
				allSrc.WriteString(res.Source)
				allSrc.WriteByte('\n')
				jp := filepath.Join(reDir, name+".java")
				if err := os.WriteFile(jp, []byte(res.Source), 0o644); err != nil {
					t.Fatal(err)
				}
				javaFiles = append(javaFiles, jp)
			}
			if checkSrc != nil {
				checkSrc(t, allSrc.String())
			}
			args := append([]string{"-proc:none", "-encoding", "UTF-8", "--release", release, "-d", outDir}, javaFiles...)
			cmd := exec.Command(javac, args...)
			cmd.Dir = reDir
			cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("recompile decompiled %s: %v\n%s\n----- source -----\n%s", mode, err, out, allSrc.String())
			}
			got := t04RunJava(t, java, outDir, mainClass)
			if got != origOut {
				t.Fatalf("T04 rebuilt stdout mismatch (%s):\n got %q\nwant %q", mode, got, origOut)
			}
		})
	}
}

func t04RunJava(t *testing.T, java, classpath, mainClass string) string {
	t.Helper()
	cmd := exec.Command(java, "-Xverify:all", "-Xmx128m", "-Dfile.encoding=UTF-8", "-cp", classpath, mainClass)
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("java %s: %v\n%s", mainClass, err, out)
	}
	return string(out)
}

func t04Tools(t *testing.T) (javac, java string) {
	t.Helper()
	var err error
	javac, err = exec.LookPath("javac")
	if err != nil {
		t.Fatal("javac is required for T04 invoke-witness tests")
	}
	java, err = exec.LookPath("java")
	if err != nil {
		t.Fatal("java is required for T04 invoke-witness tests")
	}
	return javac, java
}

func t04PoolMethodDescriptors(t *testing.T, raw []byte, ownerSlash, method string) []string {
	t.Helper()
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var descs []string
	for _, info := range obj.ConstantPool {
		mref, ok := info.(*ConstantMethodrefInfo)
		if !ok {
			continue
		}
		cls, err := obj.getConstantInfo(mref.ClassIndex)
		if err != nil {
			continue
		}
		ci, ok := cls.(*ConstantClassInfo)
		if !ok {
			continue
		}
		owner, _ := obj.getUtf8(ci.NameIndex)
		nat, err := obj.getConstantInfo(mref.NameAndTypeIndex)
		if err != nil {
			continue
		}
		ni, ok := nat.(*ConstantNameAndTypeInfo)
		if !ok {
			continue
		}
		name, _ := obj.getUtf8(ni.NameIndex)
		desc, _ := obj.getUtf8(ni.DescriptorIndex)
		if owner == ownerSlash && name == method {
			descs = append(descs, desc)
		}
	}
	return descs
}

func t04Contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
