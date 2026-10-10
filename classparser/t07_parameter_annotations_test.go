package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func t07RequireJavac(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("javac"); err != nil {
		t.Fatalf("infrastructure: javac missing: %v", err)
	}
}

func t07Compile(t *testing.T, dir string, release string, extra []string, srcs ...string) {
	t.Helper()
	t07RequireJavac(t)
	args := append([]string{"--release", release, "-d", dir}, extra...)
	args = append(args, srcs...)
	cmd := exec.Command("javac", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("infrastructure javac: %v\n%s", err, out)
	}
}

func t07Write(t *testing.T, dir, name, src string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

func t07RunJava(t *testing.T, dir, class string) string {
	t.Helper()
	if _, err := exec.LookPath("java"); err != nil {
		t.Fatalf("infrastructure: java missing: %v", err)
	}
	cmd := exec.Command("java", "-cp", dir, class)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("java %s: %v\n%s", class, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestTaskT07C01MinParamAnnotation(t *testing.T) {
	t.Run("T07-C01", func(t *testing.T) {
		dir := t.TempDir()
		src := t07Write(t, dir, "ParamAnnotation.java",
			"public class ParamAnnotation { public static void f(@Deprecated String s) {} public static void main(String[] a) throws Exception { System.out.println(ParamAnnotation.class.getDeclaredMethod(\"f\", String.class).getParameterAnnotations()[0].length); } }\n")
		t07Compile(t, dir, "8", nil, src)
		raw, err := os.ReadFile(filepath.Join(dir, "ParamAnnotation.class"))
		if err != nil {
			t.Fatal(err)
		}
		obj, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		found := false
		for _, m := range obj.Methods {
			name, _ := obj.getUtf8(m.NameIndex)
			if name != "f" {
				continue
			}
			pa := paramAnnosFromMethod(m, false)
			if pa == nil || len(pa.ParameterAnnotations) < 1 || len(pa.ParameterAnnotations[0]) != 1 {
				t.Fatalf("parsed visible param annos missing: %+v", pa)
			}
			found = true
		}
		if !found {
			t.Fatal("method f not found")
		}
		dumped, err := Decompile(raw)
		if err != nil {
			t.Fatalf("Decompile: %v", err)
		}
		if !strings.Contains(dumped, "@Deprecated") {
			t.Fatalf("decompiled source dropped parameter annotation:\n%s", dumped)
		}
		origOut := t07RunJava(t, dir, "ParamAnnotation")
		if origOut != "1" {
			t.Fatalf("original oracle want 1 got %q", origOut)
		}
		rebuild := t.TempDir()
		rsrc := t07Write(t, rebuild, "ParamAnnotation.java", dumped)
		t07Compile(t, rebuild, "8", nil, rsrc)
		rebOut := t07RunJava(t, rebuild, "ParamAnnotation")
		if rebOut != "1" {
			t.Fatalf("rebuilt reflection count want 1 got %q\nsource:\n%s", rebOut, dumped)
		}
	})
}

func TestTaskT07C02ValueTypes(t *testing.T) {
	t.Run("T07-C02", func(t *testing.T) {
		orig := t.TempDir()
		t07Write(t, orig, "Color.java", "public enum Color { RED, BLUE }\n")
		t07Write(t, orig, "NestedA.java", "public @interface NestedA { int v() default 2; }\n")
		t07Write(t, orig, "Rich.java", `import java.lang.annotation.*;
@Retention(RetentionPolicy.RUNTIME)
public @interface Rich {
  int n() default 1;
  Color c() default Color.RED;
  Class<?> k() default String.class;
  String[] arr() default {"a"};
  NestedA nest() default @NestedA(v=2);
}
`)
		t07Write(t, orig, "UseRich.java", `public class UseRich {
  public static void f(@Rich(n=3, c=Color.BLUE, k=Integer.class, arr={"x","y"}, nest=@NestedA(v=4)) String s) {}
}
`)
		compileFamily(t, orig, "8")
		origRich := parseClassFile(t, orig, "Rich.class")
		origUse := parseClassFile(t, orig, "UseRich.class")
		origNested := parseClassFile(t, orig, "NestedA.class")
		defN := annotationDefaultOf(origRich, "n")
		defC := annotationDefaultOf(origRich, "c")
		defK := annotationDefaultOf(origRich, "k")
		defArr := annotationDefaultOf(origRich, "arr")
		defNest := annotationDefaultOf(origRich, "nest")
		if defN == "" || defC == "" || defK == "" || defArr == "" || defNest == "" {
			t.Fatalf("AnnotationDefault missing on original Rich: n=%q c=%q k=%q arr=%q nest=%q", defN, defC, defK, defArr, defNest)
		}
		if annotationDefaultOf(origNested, "v") == "" {
			t.Fatal("NestedA default missing from metadata")
		}
		f := methodByName(origUse, "f")
		pa := paramAnnosFromMethod(f, false)
		if pa == nil || len(pa.ParameterAnnotations) == 0 || len(pa.ParameterAnnotations[0]) == 0 {
			t.Fatal("use-site Rich annotation missing")
		}
		origUseNorm := normalizeAnnotation(pa.ParameterAnnotations[0][0])

		rebuild := t.TempDir()
		decompileFamily(t, orig, rebuild)
		oracle := `import java.lang.annotation.Annotation;
import java.lang.reflect.Method;
import java.util.Arrays;
public class RichOracle {
  public static void main(String[] a) throws Exception {
    Annotation[] an = UseRich.class.getDeclaredMethod("f", String.class).getParameterAnnotations()[0];
    Rich r = null;
    for (Annotation x : an) if (x instanceof Rich) r = (Rich) x;
    if (r == null) { System.out.println("missing"); return; }
    System.out.println("n="+r.n());
    System.out.println("c="+r.c().name());
    System.out.println("k="+r.k().getName());
    System.out.println("arr="+Arrays.toString(r.arr()));
    System.out.println("nest="+r.nest().v());
    System.out.println("def_n="+Rich.class.getMethod("n").getDefaultValue());
    System.out.println("def_c="+((Color)Rich.class.getMethod("c").getDefaultValue()).name());
    System.out.println("def_k="+((Class)Rich.class.getMethod("k").getDefaultValue()).getName());
    System.out.println("def_arr="+Arrays.toString((String[])Rich.class.getMethod("arr").getDefaultValue()));
    System.out.println("def_nest="+((NestedA)Rich.class.getMethod("nest").getDefaultValue()).v());
  }
}
`
		t07Write(t, rebuild, "RichOracle.java", oracle)
		compileFamily(t, rebuild, "8")
		reb := runJavaCP(t, rebuild, "RichOracle")
		want := []string{
			"n=3", "c=BLUE", "k=java.lang.Integer", "arr=[x, y]", "nest=4",
			"def_n=1", "def_c=RED", "def_k=java.lang.String", "def_arr=[a]", "def_nest=2",
		}
		for _, w := range want {
			if !strings.Contains(reb, w) {
				t.Errorf("rebuilt reflection missing %q in\n%s", w, reb)
			}
		}
		rebRich := parseClassFile(t, rebuild, "Rich.class")
		rebUse := parseClassFile(t, rebuild, "UseRich.class")
		if annotationDefaultOf(rebRich, "n") != defN {
			t.Fatalf("default n metadata orig=%q rebuilt=%q", defN, annotationDefaultOf(rebRich, "n"))
		}
		if annotationDefaultOf(rebRich, "c") != defC || annotationDefaultOf(rebRich, "k") != defK {
			t.Fatalf("enum/class defaults drifted orig c=%q k=%q reb c=%q k=%q", defC, defK, annotationDefaultOf(rebRich, "c"), annotationDefaultOf(rebRich, "k"))
		}
		rebF := methodByName(rebUse, "f")
		rebPA := paramAnnosFromMethod(rebF, false)
		if rebPA == nil || len(rebPA.ParameterAnnotations[0]) == 0 {
			t.Fatal("rebuilt use-site annotation missing")
		}
		if normalizeAnnotation(rebPA.ParameterAnnotations[0][0]) != origUseNorm {
			t.Fatalf("use-site metadata mismatch\norig %s\nreb  %s", origUseNorm, normalizeAnnotation(rebPA.ParameterAnnotations[0][0]))
		}
	})
}

func TestTaskT07C03SyntheticMapping(t *testing.T) {
	t.Run("T07-C03", func(t *testing.T) {
		orig := t.TempDir()
		t07Write(t, orig, "Outer.java", `public class Outer {
  public class Inner { Inner(String keep, @Deprecated int x) {} }
  public static void inst(@Deprecated String s, int y) {}
}
`)
		t07Write(t, orig, "E.java", `public enum E { A(1,2); E(int keep, @Deprecated int x) {} }
`)
		t07Write(t, orig, "Plain.java", `public class Plain { public static void f(@Deprecated String s, int y) {} }
`)
		compileFamily(t, orig, "8")
		oracle := `import java.lang.annotation.Annotation;
import java.lang.reflect.*;
public class SynthOracle {
  static String dump(Annotation[][] a) {
    StringBuilder sb = new StringBuilder();
    for (int i = 0; i < a.length; i++) {
      if (i > 0) sb.append("|");
      sb.append(a[i].length);
      for (Annotation x : a[i]) sb.append(":").append(x.annotationType().getSimpleName());
    }
    return sb.toString();
  }
  public static void main(String[] args) throws Exception {
    for (Constructor<?> c : Class.forName("Outer$Inner").getDeclaredConstructors()) {
      System.out.println("inner="+dump(c.getParameterAnnotations()));
    }
    for (Constructor<?> c : Class.forName("E").getDeclaredConstructors()) {
      Class<?>[] pt = c.getParameterTypes();
      if (pt.length >= 2) System.out.println("enum="+dump(c.getParameterAnnotations()));
    }
    System.out.println("plain="+dump(Class.forName("Plain").getDeclaredMethod("f", String.class, int.class).getParameterAnnotations()));
    System.out.println("inst="+dump(Class.forName("Outer").getDeclaredMethod("inst", String.class, int.class).getParameterAnnotations()));
  }
}
`
		t07Write(t, orig, "SynthOracle.java", oracle)
		compileFamily(t, orig, "8")
		origDump := runJavaCP(t, orig, "SynthOracle")
		rebuild := t.TempDir()
		decompileFamily(t, orig, rebuild)
		t07Write(t, rebuild, "SynthOracle.java", oracle)
		compileFamily(t, rebuild, "8")
		rebDump := runJavaCP(t, rebuild, "SynthOracle")
		if origDump != rebDump {
			t.Fatalf("synthetic mapping reflection mismatch\norig %s\nreb  %s", origDump, rebDump)
		}
		for _, line := range strings.Split(rebDump, "\n") {
			if strings.HasPrefix(line, "inner=") && strings.Contains(line, "Deprecated") && strings.HasPrefix(line, "inner=1:Deprecated") {
				t.Fatalf("inner annotation collapsed onto first param: %s", line)
			}
		}
		innerSrc, err := Decompile(mustRead(t, filepath.Join(orig, "Outer$Inner.class")))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(innerSrc, "@Deprecated Outer") || !strings.Contains(innerSrc, "@Deprecated") {
			t.Fatalf("inner source mapping:\n%s", innerSrc)
		}
		enumSrc, err := Decompile(mustRead(t, filepath.Join(orig, "E.class")))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(enumSrc, "@Deprecated String") || !strings.Contains(enumSrc, "@Deprecated") {
			t.Fatalf("enum source mapping:\n%s", enumSrc)
		}
	})
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTaskT07C04Retention(t *testing.T) {
	t.Run("T07-C04", func(t *testing.T) {
		orig := t.TempDir()
		t07Write(t, orig, "RunA.java", "import java.lang.annotation.*;\n@Retention(RetentionPolicy.RUNTIME)\npublic @interface RunA {}\n")
		t07Write(t, orig, "ClassA.java", "import java.lang.annotation.*;\n@Retention(RetentionPolicy.CLASS)\npublic @interface ClassA {}\n")
		t07Write(t, orig, "Ret.java", `public class Ret {
  public static void f(@RunA @ClassA String s) {}
}
`)
		compileFamily(t, orig, "8")
		obj := parseClassFile(t, orig, "Ret.class")
		f := methodByName(obj, "f")
		vis := paramAnnosFromMethod(f, false)
		inv := paramAnnosFromMethod(f, true)
		if vis == nil || len(vis.ParameterAnnotations[0]) != 1 || vis.IsInvisible {
			t.Fatalf("visible: %+v", vis)
		}
		if inv == nil || len(inv.ParameterAnnotations[0]) != 1 || !inv.IsInvisible {
			t.Fatalf("invisible: %+v", inv)
		}
		rebuild := t.TempDir()
		decompileFamily(t, orig, rebuild)
		t07Write(t, rebuild, "RetOracle.java", `public class RetOracle {
  public static void main(String[] a) throws Exception {
    System.out.println(Ret.class.getDeclaredMethod("f", String.class).getParameterAnnotations()[0].length);
  }
}
`)
		compileFamily(t, rebuild, "8")
		if runJavaCP(t, rebuild, "RetOracle") != "1" {
			t.Fatal("rebuilt reflection mixed CLASS into RUNTIME")
		}
		reb := parseClassFile(t, rebuild, "Ret.class")
		rf := methodByName(reb, "f")
		rvis := paramAnnosFromMethod(rf, false)
		rinv := paramAnnosFromMethod(rf, true)
		if rvis == nil || rinv == nil || rvis.IsInvisible || !rinv.IsInvisible {
			t.Fatalf("rebuilt flags vis=%+v inv=%+v", rvis, rinv)
		}
		if normalizeAnnotation(rvis.ParameterAnnotations[0][0]) != normalizeAnnotation(vis.ParameterAnnotations[0][0]) {
			t.Fatal("visible metadata drifted")
		}
		if normalizeAnnotation(rinv.ParameterAnnotations[0][0]) != normalizeAnnotation(inv.ParameterAnnotations[0][0]) {
			t.Fatal("invisible metadata drifted")
		}
	})
}

func TestTaskT07C05Malformed(t *testing.T) {
	t.Run("T07-C05", func(t *testing.T) {
		r := NewClassReader([]byte{'X', 0, 0})
		r.SetStage("annotation", "tag")
		cp := &ClassParser{reader: r, classObj: NewClassObject()}
		_ = ParseAnnotationElementValue(cp)
		if r.Err() == nil {
			t.Fatal("illegal element tag accepted")
		}
		if classParseCode(r.Err()) != ParseCodeInvalidInput {
			t.Fatalf("want invalid_input code, got %v", r.Err())
		}

		r = NewClassReader([]byte{'@'})
		r.SetStage("annotation", "trunc")
		cp = &ClassParser{reader: r, classObj: NewClassObject()}
		_ = ParseAnnotationElementValue(cp)
		if r.Err() == nil {
			t.Fatal("truncated nested annotation accepted")
		}

		r = NewClassReader([]byte{'s', 0, 99})
		r.SetStage("annotation", "index")
		cp = &ClassParser{reader: r, classObj: NewClassObject()}
		cp.classObj.ConstantPool = []ConstantInfo{&ConstantUtf8Info{Value: "x"}}
		_ = ParseAnnotationElementValue(cp)
		if r.Err() == nil {
			t.Fatal("bad CP index accepted")
		}

		// Nesting limit: 65 nested '@' without completing annotation bodies.
		var nest []byte
		for i := 0; i < maxAnnotationNesting+2; i++ {
			nest = append(nest, '@', 0, 1, 0, 0) // type_index=1, 0 pairs would not recurse; use 1 pair then value
		}
		// Simpler: call ParseAnnotation repeatedly by constructing '@' values that recurse.
		deep := []byte{}
		for i := 0; i < maxAnnotationNesting+1; i++ {
			deep = append(deep, '@', 0, 1, 0, 1, 0, 1) // type 1, 1 pair, name index 1, then next value tag
		}
		deep = append(deep, 's', 0, 1)
		r = NewClassReader(deep)
		r.SetStage("annotation", "depth")
		cp = &ClassParser{reader: r, classObj: NewClassObject()}
		cp.classObj.ConstantPool = []ConstantInfo{&ConstantUtf8Info{Value: "Lx;"}}
		_ = ParseAnnotationElementValue(cp)
		if r.Err() == nil {
			t.Fatal("depth overflow accepted")
		}
		if classParseCode(r.Err()) != ParseCodeResourceLimit && classParseCode(r.Err()) != ParseCodeTruncated && classParseCode(r.Err()) != ParseCodeInvalidInput && classParseCode(r.Err()) != ParseCodeCPIndex {
			t.Fatalf("depth error %v", r.Err())
		}
		res, err := DecompileWithOptions(deep, DecompileOptions{Mode: Precision})
		if err == nil || res.Status == "complete" {
			t.Fatalf("malformed must not be complete: %+v %v", res, err)
		}
	})
}

func TestTaskT07C06DebugAgnostic(t *testing.T) {
	t.Run("T07-C06", func(t *testing.T) {
		oracle := `import java.lang.annotation.Annotation;
public class DbgOracle {
  public static void main(String[] a) throws Exception {
    Annotation[][] p = Dbg.class.getDeclaredMethod("f", String.class, int.class).getParameterAnnotations();
    System.out.println(p[0].length+":"+(p[0].length==0?"":p[0][0].annotationType().getSimpleName())+"|"+p[1].length);
  }
}
`
		compileOne := func(src string, extra []string) string {
			dir := t.TempDir()
			t07Write(t, dir, "Dbg.java", src)
			t07Write(t, dir, "DbgOracle.java", oracle)
			args := append([]string{"--release", "8", "-d", dir}, extra...)
			args = append(args, filepath.Join(dir, "Dbg.java"), filepath.Join(dir, "DbgOracle.java"))
			cmd := exec.Command("javac", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("javac: %v\n%s", err, out)
			}
			return dir
		}
		srcA := "public class Dbg { public static void f(@Deprecated String s, int y) {} }\n"
		srcB := "public class Dbg { public static void f(@Deprecated String renamed, int zz) {} }\n"
		g := compileOne(srcA, []string{"-g"})
		n := compileOne(srcA, []string{"-g:none"})
		r := compileOne(srcB, []string{"-g"})
		og := runJavaCP(t, g, "DbgOracle")
		on := runJavaCP(t, n, "DbgOracle")
		or := runJavaCP(t, r, "DbgOracle")
		if og != "1:Deprecated|0" || on != og || or != og {
			t.Fatalf("original semantic placement drifted g=%q none=%q rename=%q", og, on, or)
		}
		rebuildCheck := func(srcDir string) {
			dst := t.TempDir()
			decompileFamily(t, srcDir, dst)
			t07Write(t, dst, "DbgOracle.java", oracle)
			compileFamily(t, dst, "8")
			got := runJavaCP(t, dst, "DbgOracle")
			if got != og {
				src, _ := os.ReadFile(filepath.Join(dst, "Dbg.java"))
				t.Fatalf("rebuilt mapping %q want %q\n%s", got, og, src)
			}
		}
		rebuildCheck(g)
		rebuildCheck(n)
		rebuildCheck(r)
	})
}

func TestTaskT07AnnotationStringUnits(t *testing.T) {
	t.Run("T07-units", func(t *testing.T) {
		obj := NewClassObject()
		u := &ConstantUtf8Info{}
		u.SetUnits([]uint16{0xD800, 'A'})
		obj.ConstantPool = []ConstantInfo{u}
		r := NewClassReader([]byte{'s', 0, 1})
		cp := &ClassParser{reader: r, classObj: obj}
		el := ParseAnnotationElementValue(cp)
		if r.Err() != nil {
			t.Fatal(r.Err())
		}
		want := []uint16{0xD800, 'A'}
		units := annotationStringUnits(el.Value)
		if !utf16UnitsEqual(units, want) {
			t.Fatalf("string element lost units: %T %v", el.Value, el.Value)
		}
		if _, ok := el.Value.(Utf8String); !ok {
			t.Fatalf("annotation tag s must store Utf8String, got %T", el.Value)
		}
		if lit := javaAnnotationStringLiteral(el.Value); !strings.Contains(lit, "D800") && !strings.Contains(lit, "d800") {
			t.Fatalf("literal dropped unpaired surrogate: %s", lit)
		}
	})
}

func TestTaskT07NotSilentDrop(t *testing.T) {
	// Guard: a successful parse of ParamAnnotation-like bytecode must keep the attribute.
	dir := t.TempDir()
	src := t07Write(t, dir, "P.java", "public class P { public static void f(@Deprecated String s) {} }\n")
	t07Compile(t, dir, "8", nil, src)
	raw, _ := os.ReadFile(filepath.Join(dir, "P.class"))
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("RuntimeVisibleParameterAnnotations")) {
		keep := false
		for _, m := range obj.Methods {
			if paramAnnosFromMethod(m, false) != nil {
				keep = true
			}
		}
		if !keep {
			t.Fatal("parameter annotations silently dropped after parse")
		}
	}
}
