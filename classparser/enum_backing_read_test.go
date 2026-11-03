package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

const enumBackingReadFixture = `enum BackingRead {
  ALPHA, BETA;
  public static BackingRead byName(String text) {
    for (BackingRead mode : values()) {
      if (mode.name().equals(text)) return mode;
    }
    throw new IllegalArgumentException(text);
  }
}`

func TestEnumBackingArrayIterationIsExpressible(t *testing.T) {
	javac, java := t04Tools(t)
	files := nativeCompileClasses(t, enumBackingReadFixture+`
class BackingDriver {
 public static void main(String[] args) {
  System.out.print(BackingRead.byName("ALPHA").name()+":"+BackingRead.byName("BETA").ordinal());
  for (String s : new String[]{null,"missing"}) {
   try { BackingRead.byName(s); throw new AssertionError(); }
   catch (IllegalArgumentException e) { System.out.print(":"+e.getClass().getSimpleName()+":"+e.getMessage()); }
  }
 }
}`)
	files["BackingRead.class"] = patchEnumBackingMethod(t, files["BackingRead.class"], "byName")
	original := writeBackingClasses(t, files)
	want := t04RunJava(t, java, original, "BackingDriver")
	if want != "ALPHA:1:IllegalArgumentException:null:IllegalArgumentException:missing" {
		t.Fatalf("oracle=%q", want)
	}
	archive := nativeArchive(t, files)
	defer archive.Close()
	raw, err := archive.ReadFile("BackingRead.class")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if strings.Contains(source, "decompile dump failed") || strings.Contains(source, "$VALUES") || strings.Contains(source, "undecompilable method body") || !strings.Contains(source, "values()") {
		t.Fatalf("iteration not restored:\n%s", source)
	}
	rebuilt := t.TempDir()
	path := filepath.Join(rebuilt, "BackingRead.java")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", rebuilt, path).CombinedOutput(); err != nil {
		t.Fatalf("recompile: %v\n%s\n%s", err, out, source)
	}
	if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+original, "BackingDriver"); got != want {
		t.Fatalf("original=%q rebuilt=%q", want, got)
	}
}

func TestEnumBackingArrayReturnIsRefused(t *testing.T) {
	_, java := t04Tools(t)
	files := nativeCompileClasses(t, `enum BackingRead {ALPHA,BETA; public static Object[] shared() { return values(); }}
class BackingDriver {public static void main(String[] args) {System.out.print(BackingRead.shared()==BackingRead.shared());}}`)
	files["BackingRead.class"] = patchEnumBackingMethod(t, files["BackingRead.class"], "shared")
	original := writeBackingClasses(t, files)
	if got := t04RunJava(t, java, original, "BackingDriver"); got != "true" {
		t.Fatalf("valid bytecode identity oracle=%q", got)
	}
	archive := nativeArchive(t, files)
	defer archive.Close()
	raw, err := archive.ReadFile("BackingRead.class")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "source-erased backing field escapes its generated factory") {
		t.Fatalf("escaped shared identity accepted:\n%s", raw)
	}
}

// An enum's valueOf(String) cannot be declared explicitly. Rendering a legacy
// loop with that name is illegal Java; silently using the implicit factory
// changes null and missing-name behavior. Keep refusal, not a false success.
func TestEnumLegacyValueOfCannotBeSilentlyReplaced(t *testing.T) {
	_, java := t04Tools(t)
	files := nativeCompileClasses(t, enumBackingReadFixture+`
class BackingDriver {
 public static void main(String[] args) {
  System.out.print(BackingRead.valueOf("ALPHA").name());
  for(String s:new String[]{null,"missing"}) {
   try {BackingRead.valueOf(s);throw new AssertionError();}
   catch(IllegalArgumentException e) {System.out.print(":"+e.getClass().getSimpleName()+":"+e.getMessage());}
  }
 }
}`)
	raw := patchEnumBackingMethod(t, files["BackingRead.class"], "byName")
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var donor *CodeAttribute
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n == "byName" {
			for _, a := range m.Attributes {
				if body, ok := a.(*CodeAttribute); ok {
					donor = body
				}
			}
		}
	}
	if donor == nil {
		t.Fatal("missing donor")
	}
	moved := false
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n != "valueOf" {
			continue
		}
		for i, a := range m.Attributes {
			if _, ok := a.(*CodeAttribute); ok {
				m.Attributes[i] = donor
				moved = true
			}
		}
	}
	if !moved {
		t.Fatal("missing factory")
	}
	files["BackingRead.class"] = obj.Bytes()
	original := writeBackingClasses(t, files)
	if got := t04RunJava(t, java, original, "BackingDriver"); got != "ALPHA:IllegalArgumentException:null:IllegalArgumentException:missing" {
		t.Fatalf("legacy oracle=%q", got)
	}
	archive := nativeArchive(t, files)
	defer archive.Close()
	got, err := archive.ReadFile("BackingRead.class")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "does not implement the source compiler protocol") {
		t.Fatalf("illegal or semantics-changing factory emitted:\n%s", got)
	}
}

func writeBackingClasses(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for n, raw := range files {
		path := filepath.Join(dir, n)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Both invokestatic and getstatic use a two-byte constant-pool operand, so the
// patch preserves branch offsets and StackMapTable frames. Return types and
// local shapes are unchanged; each negative fixture is runnable valid bytecode.
func patchEnumBackingMethod(t *testing.T, raw []byte, methodName string) []byte {
	t.Helper()
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var backing uint16
	for i, e := range obj.ConstantPool {
		ref, ok := e.(*ConstantFieldrefInfo)
		if !ok || ref == nil {
			continue
		}
		owner, known := sourceBridgeClassName(obj, ref.ClassIndex)
		if !known || owner != obj.GetClassName() {
			continue
		}
		nt, ok := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			continue
		}
		n, _ := sourceBridgeUTF8(obj, nt.NameIndex)
		d, _ := sourceBridgeUTF8(obj, nt.DescriptorIndex)
		if n == "$VALUES" && d == "[L"+obj.GetClassName()+";" {
			backing = uint16(i + 1)
		}
	}
	if backing == 0 {
		t.Fatal("backing absent")
	}
	patched := 0
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n != methodName {
			continue
		}
		for _, a := range m.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok || code == nil {
				continue
			}
			d := core.NewDecompiler(code.Code, nil)
			if d.ParseOpcode() != nil {
				t.Fatal("decode")
			}
			for _, op := range constructorMotionOps(d) {
				member := constructorMotionMember(obj, op, core.OP_INVOKESTATIC)
				if member == nil || member.Member != "values" || member.Name != obj.GetClassName() {
					continue
				}
				pc := int(op.CurrentOffset)
				if len(op.Data) != 2 || pc+3 > len(code.Code) {
					t.Fatal("factory shape")
				}
				code.Code[pc], code.Code[pc+1], code.Code[pc+2] = core.OP_GETSTATIC, byte(backing>>8), byte(backing)
				patched++
			}
		}
	}
	if patched != 1 {
		t.Fatalf("patched calls=%d", patched)
	}
	return obj.Bytes()
}
