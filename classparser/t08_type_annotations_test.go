package javaclassparser

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

const typePathOracleSrc = `import java.lang.annotation.Annotation;
import java.lang.reflect.*;
import java.util.*;
public class TypePathOracle {
  static String names(Annotation[] a) {
    ArrayList<String> n = new ArrayList<String>();
    for (Annotation x : a) n.add(x.annotationType().getSimpleName());
    return String.join(",", n);
  }
  static String walk(AnnotatedType t) {
    if (t instanceof AnnotatedArrayType) {
      AnnotatedArrayType a = (AnnotatedArrayType) t;
      return "ARR{" + names(a.getDeclaredAnnotations()) + "}(" + walk(a.getAnnotatedGenericComponentType()) + ")";
    }
    if (t instanceof AnnotatedParameterizedType) {
      AnnotatedParameterizedType p = (AnnotatedParameterizedType) t;
      StringBuilder sb = new StringBuilder("P{" + names(p.getDeclaredAnnotations()) + "}<");
      AnnotatedType[] args = p.getAnnotatedActualTypeArguments();
      for (int i = 0; i < args.length; i++) {
        if (i > 0) sb.append(",");
        sb.append(walk(args[i]));
      }
      sb.append(">");
      return sb.toString();
    }
    return "T{" + names(t.getDeclaredAnnotations()) + "}" + t.getType().getTypeName();
  }
  public static void main(String[] args) throws Exception {
    String mode = args[0];
    if (mode.equals("array")) {
      System.out.println(walk(Class.forName("ArrAnno").getDeclaredMethod("f").getAnnotatedReturnType()));
      Method g = null;
      for (Method mm : Class.forName("ArrAnno").getDeclaredMethods()) if (mm.getName().equals("g")) g = mm;
      System.out.println(walk(g.getAnnotatedParameterTypes()[0]));
    } else if (mode.equals("generic")) {
      System.out.println(walk(Class.forName("GenAnno").getDeclaredMethod("f").getAnnotatedReturnType()));
    } else if (mode.equals("recv")) {
      Method m = Class.forName("Recv").getDeclaredMethod("m");
      System.out.println("RECV{" + names(m.getAnnotatedReceiverType().getDeclaredAnnotations()) + "}");
      Method b = Class.forName("Recv").getDeclaredMethod("b", Number.class);
      TypeVariable<?> tv = b.getTypeParameters()[0];
      System.out.println("BOUND{" + names(tv.getAnnotatedBounds()[0].getDeclaredAnnotations()) + "}");
      Method th = Class.forName("Recv").getDeclaredMethod("t");
      System.out.println("THROWS{" + names(th.getAnnotatedExceptionTypes()[0].getDeclaredAnnotations()) + "}");
    }
  }
}
`

func t08WriteCompile(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		t07Write(t, dir, name, src)
	}
	compileFamily(t, dir, "8")
}

func TestTaskT08C01ArrayDimensions(t *testing.T) {
	t.Run("T08-C01", func(t *testing.T) {
		orig := t.TempDir()
		t08WriteCompile(t, orig, map[string]string{
			"A.java":       "import java.lang.annotation.*;\n@Retention(RetentionPolicy.RUNTIME)\n@Target(ElementType.TYPE_USE)\npublic @interface A {}\n",
			"B.java":       "import java.lang.annotation.*;\n@Retention(RetentionPolicy.RUNTIME)\n@Target(ElementType.TYPE_USE)\npublic @interface B {}\n",
			"C.java":       "import java.lang.annotation.*;\n@Retention(RetentionPolicy.RUNTIME)\n@Target(ElementType.TYPE_USE)\npublic @interface C {}\n",
			"ArrAnno.java": "public class ArrAnno { public static @A String @B [] @C [] f() { return null; } public static void g(@A String @B [] @C [] p) {} }\n",
		})
		obj := parseClassFile(t, orig, "ArrAnno.class")
		var tas []*TypeAnnotation
		for _, m := range obj.Methods {
			name, _ := obj.getUtf8(m.NameIndex)
			if name == "f" {
				tas = filterTypeAnnos(typeAnnosFrom(m.Attributes), 0x14, 0, false)
			}
		}
		if len(tas) != 3 {
			t.Fatalf("want 3 return type annotations, got %d", len(tas))
		}
		seen := map[string]string{}
		order := []string{}
		for _, ta := range tas {
			key := ta.Annotation.TypeName
			order = append(order, key)
			var dims []string
			for _, p := range ta.TypePath {
				dims = append(dims, fmtPath(p))
			}
			seen[key] = strings.Join(dims, "/")
		}
		// @A on String (two array steps), @B one array, @C empty (outermost array).
		if !strings.Contains(seen["LA;"], "arr") && seen["LA;"] != "0/0" && seen["LA;"] != "arr/arr" {
			// accept either encoding of two ARRAY steps
			if len(splitPath(seen["LA;"])) != 2 {
				t.Fatalf("A path want 2 array steps, got %q order=%v seen=%v", seen["LA;"], order, seen)
			}
		}
		t07Write(t, orig, "TypePathOracle.java", typePathOracleSrc)
		compileFamily(t, orig, "8")
		origRefl := runJavaCP(t, orig, "TypePathOracle", "array")
		if !strings.Contains(origRefl, "T{A}java.lang.String") || !strings.Contains(origRefl, "ARR{") {
			t.Fatalf("original reflection path unexpected:\n%s", origRefl)
		}
		if strings.Count(origRefl, "ARR{") < 2 {
			t.Fatalf("want two array dimensions in oracle:\n%s", origRefl)
		}
		rebuild := t.TempDir()
		decompileFamily(t, orig, rebuild)
		t07Write(t, rebuild, "TypePathOracle.java", typePathOracleSrc)
		compileFamily(t, rebuild, "8")
		rebRefl := runJavaCP(t, rebuild, "TypePathOracle", "array")
		if origRefl != rebRefl {
			t.Fatalf("array dimension round-trip mismatch\norig %s\nreb  %s", origRefl, rebRefl)
		}
		rebObj := parseClassFile(t, rebuild, "ArrAnno.class")
		var reb []*TypeAnnotation
		for _, m := range rebObj.Methods {
			name, _ := rebObj.getUtf8(m.NameIndex)
			if name == "f" {
				reb = filterTypeAnnos(typeAnnosFrom(m.Attributes), 0x14, 0, false)
			}
		}
		if len(reb) != 3 {
			t.Fatalf("rebuilt type_path count %d", len(reb))
		}
		for i := range tas {
			if !pathEqual(tas[i].TypePath, reb[i].TypePath) {
				t.Fatalf("path order changed at %d orig=%v reb=%v", i, tas[i].TypePath, reb[i].TypePath)
			}
		}
	})
}

func fmtPath(p TypePathEntry) string {
	return strings.TrimSpace(strings.Join([]string{itoc(p.Kind), itoc(p.ArgumentIndex)}, ":"))
}
func itoc(u uint8) string {
	return string(rune('0' + u))
}
func splitPath(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func TestTaskT08C02GenericNested(t *testing.T) {
	t.Run("T08-C02", func(t *testing.T) {
		orig := t.TempDir()
		t08WriteCompile(t, orig, map[string]string{
			"A.java":       "import java.lang.annotation.*;\n@Retention(RetentionPolicy.RUNTIME)\n@Target(ElementType.TYPE_USE)\npublic @interface A {}\n",
			"B.java":       "import java.lang.annotation.*;\n@Retention(RetentionPolicy.RUNTIME)\n@Target(ElementType.TYPE_USE)\npublic @interface B {}\n",
			"C.java":       "import java.lang.annotation.*;\n@Retention(RetentionPolicy.RUNTIME)\n@Target(ElementType.TYPE_USE)\npublic @interface C {}\n",
			"GenAnno.java": "import java.util.*;\npublic class GenAnno { public static List<@A Map<@B String, @C Integer>> f() { return null; } }\n",
		})
		t07Write(t, orig, "TypePathOracle.java", typePathOracleSrc)
		compileFamily(t, orig, "8")
		origRefl := runJavaCP(t, orig, "TypePathOracle", "generic")
		if !strings.Contains(origRefl, "P{A}<") || !strings.Contains(origRefl, "T{B}java.lang.String") || !strings.Contains(origRefl, "T{C}java.lang.Integer") {
			t.Fatalf("generic path oracle:\n%s", origRefl)
		}
		rebuild := t.TempDir()
		decompileFamily(t, orig, rebuild)
		t07Write(t, rebuild, "TypePathOracle.java", typePathOracleSrc)
		compileFamily(t, rebuild, "8")
		rebRefl := runJavaCP(t, rebuild, "TypePathOracle", "generic")
		if origRefl != rebRefl {
			t.Fatalf("generic nested path mismatch\norig %s\nreb  %s", origRefl, rebRefl)
		}
		obj := parseClassFile(t, orig, "GenAnno.class")
		reb := parseClassFile(t, rebuild, "GenAnno.class")
		ot := filterTypeAnnos(typeAnnosFrom(methodByName(obj, "f").Attributes), 0x14, 0, false)
		rt := filterTypeAnnos(typeAnnosFrom(methodByName(reb, "f").Attributes), 0x14, 0, false)
		if len(ot) != len(rt) {
			t.Fatalf("count orig=%d reb=%d", len(ot), len(rt))
		}
		for i := range ot {
			if !pathEqual(ot[i].TypePath, rt[i].TypePath) {
				t.Fatalf("type_path[%d] orig=%v reb=%v", i, ot[i].TypePath, rt[i].TypePath)
			}
		}
	})
}

func TestTaskT08C03ReceiverBoundThrows(t *testing.T) {
	t.Run("T08-C03", func(t *testing.T) {
		orig := t.TempDir()
		t08WriteCompile(t, orig, map[string]string{
			"A.java": "import java.lang.annotation.*;\n@Retention(RetentionPolicy.RUNTIME)\n@Target({ElementType.TYPE_USE, ElementType.TYPE_PARAMETER})\npublic @interface A {}\n",
			"Recv.java": `public class Recv {
  public void m(@A Recv this) {}
  public <T extends @A Number> void b(T x) {}
  public void t() throws @A Exception {}
}
`,
		})
		t07Write(t, orig, "TypePathOracle.java", typePathOracleSrc)
		compileFamily(t, orig, "8")
		origRefl := runJavaCP(t, orig, "TypePathOracle", "recv")
		for _, want := range []string{"RECV{A}", "BOUND{A}", "THROWS{A}"} {
			if !strings.Contains(origRefl, want) {
				t.Fatalf("original missing %s in %s", want, origRefl)
			}
		}
		rebuild := t.TempDir()
		decompileFamily(t, orig, rebuild)
		t07Write(t, rebuild, "TypePathOracle.java", typePathOracleSrc)
		compileFamily(t, rebuild, "8")
		rebRefl := runJavaCP(t, rebuild, "TypePathOracle", "recv")
		if origRefl != rebRefl {
			t.Fatalf("receiver/bound/throws mismatch\norig %s\nreb  %s", origRefl, rebRefl)
		}
		obj := parseClassFile(t, orig, "Recv.class")
		seen := map[uint8]bool{}
		for _, m := range obj.Methods {
			for _, ta := range typeAnnosFrom(m.Attributes) {
				if ta != nil {
					seen[ta.TargetType] = true
				}
			}
		}
		for _, want := range []uint8{0x15, 0x12, 0x17} {
			if !seen[want] {
				t.Errorf("missing parsed target 0x%02x", want)
			}
		}
	})
}

func TestTaskT08C04CorruptAndUnsupported(t *testing.T) {
	t.Run("T08-C04", func(t *testing.T) {
		r := NewClassReader([]byte{0xFF})
		r.SetStage("type_annotation", "target")
		cp := &ClassParser{reader: r, classObj: NewClassObject()}
		_ = parseTypeAnnotation(cp)
		if r.Err() == nil || classParseCode(r.Err()) != ParseCodeInvalidInput {
			t.Fatalf("illegal target tag: %v", r.Err())
		}

		r = NewClassReader([]byte{0x13, 2, 0})
		r.SetStage("type_annotation", "path")
		cp = &ClassParser{reader: r, classObj: NewClassObject()}
		_ = parseTypeAnnotation(cp)
		if r.Err() == nil {
			t.Fatal("truncated path accepted")
		}

		st, err := types.ParseDescriptor("Ljava/lang/String;")
		if err != nil {
			t.Fatal(err)
		}
		if err := walkTypePath(st, []TypePathEntry{{Kind: typePathKindTypeArg, ArgumentIndex: 0}}); err == nil {
			t.Fatal("type argument on String must be rejected")
		}
		listT, err := types.ParseDescriptor("Ljava/util/List;")
		if err != nil {
			t.Fatal(err)
		}
		if pt := types.ParseSignature("Ljava/util/List<Ljava/lang/String;>;"); pt != nil {
			listT = pt
		}
		if err := walkTypePath(listT, []TypePathEntry{{Kind: typePathKindTypeArg, ArgumentIndex: 9}}); err == nil {
			t.Fatal("nonexistent List type argument 9 must be rejected")
		}

		body := []byte{
			0x00, 0x01,
			0x43,
			0x00, 0x00,
			0x00,
			0x00, 0x01,
			0x00, 0x00,
		}
		r = NewClassReader(body)
		r.SetStage("type_annotation", "code_offset")
		cp = &ClassParser{reader: r, classObj: NewClassObject()}
		cp.classObj.ConstantPool = []ConstantInfo{&ConstantUtf8Info{Value: "LA;", Units: []uint16{'L', 'A', ';'}}}
		attr := &TypeAnnotationsAttribute{AttrLen: uint32(len(body))}
		attr.readInfo(cp)
		if cp.reader.Err() != nil {
			t.Fatalf("legal 0x43 must parse: %v", cp.reader.Err())
		}
		if len(attr.Annotations) != 1 || !attr.Annotations[0].CodeOffsetTarget {
			t.Fatalf("0x43 not preserved: %+v", attr.Annotations)
		}

		orig := t.TempDir()
		t08WriteCompile(t, orig, map[string]string{
			"A.java": "import java.lang.annotation.*;\n@Retention(RetentionPolicy.RUNTIME)\n@Target(ElementType.TYPE_USE)\npublic @interface A {}\n",
			"S.java": "import java.util.*;\npublic class S { public static List<@A String> f() { return null; } }\n",
		})
		obj := parseClassFile(t, orig, "S.class")
		mf := methodByName(obj, "f")
		tas := filterTypeAnnos(typeAnnosFrom(mf.Attributes), 0x14, 0, false)
		if len(tas) == 0 {
			t.Fatal("no return type annotations on S.f")
		}
		tas[0].TypePath = []TypePathEntry{{Kind: typePathKindTypeArg, ArgumentIndex: 9}}
		if err := validateTypeAnnotationPaths(obj); err == nil {
			t.Fatal("validateTypeAnnotationPaths accepted type argument 9")
		} else if classParseCode(err) != ParseCodeInvalidInput && !strings.Contains(err.Error(), "type argument") {
			t.Fatalf("want invalid_input for bad type argument, got %v", err)
		}
		raw := mustRead(t, filepath.Join(orig, "S.class"))
		var info []byte
		for _, attr := range mf.Attributes {
			if ta, ok := attr.(*TypeAnnotationsAttribute); ok {
				info = ta.Info
				break
			}
		}
		if len(info) == 0 {
			t.Fatal("missing type annotation Info")
		}
		win := bytes.Index(raw, info)
		rel := bytes.Index(info, []byte{3, 0})
		if win < 0 || rel < 0 {
			t.Fatal("could not locate type_argument path in class bytes")
		}
		patched := bytes.Clone(raw)
		patched[win+rel+1] = 9
		_, perr := Parse(patched)
		if perr == nil {
			t.Fatal("Parse accepted nonexistent type argument index 9")
		}
		if classParseCode(perr) != ParseCodeInvalidInput && !strings.Contains(perr.Error(), "type argument") && !strings.Contains(perr.Error(), "invalid_input") {
			t.Fatalf("want invalid_input for bad type argument, got %v", perr)
		}
	})
}

func TestTaskT08C05LocalCapability(t *testing.T) {
	t.Run("T08-C05", func(t *testing.T) {
		dir := t.TempDir()
		t08WriteCompile(t, dir, map[string]string{
			"A.java":   "import java.lang.annotation.*;\n@Retention(RetentionPolicy.RUNTIME)\n@Target(ElementType.TYPE_USE)\npublic @interface A {}\n",
			"Loc.java": "public class Loc { public void loc() { @A String s = \"x\"; } }\n",
		})
		raw := mustRead(t, filepath.Join(dir, "Loc.class"))
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		kept := false
		var local *TypeAnnotation
		for _, m := range obj.Methods {
			name, _ := obj.getUtf8(m.NameIndex)
			if name != "loc" {
				continue
			}
			for _, ta := range methodAndCodeTypeAnnos(m) {
				if ta != nil && ta.CodeOffsetTarget {
					kept = true
					local = ta
				}
			}
		}
		if !kept {
			t.Fatal("local/code-offset type annotation dropped from model")
		}
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatalf("decompile: %v", err)
		}
		if res.Status == "complete" {
			t.Fatalf("local type annotation must not count as declaration round-trip complete: %+v local=%+v", res, local)
		}
		found := false
		for _, d := range res.Diagnostics {
			if d.Code == "unsupported" {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing unsupported diagnostic: %+v", res.Diagnostics)
		}
	})
}
