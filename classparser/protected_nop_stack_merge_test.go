package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Instrumentation may put a NOP before the protected region or at the handler
// entry, and leave normal/caught results on the stack until a shared store.
// javac's source-level try/catch stores on each arm instead, so construct this
// valid version-49 class directly and use the JVM itself as the first oracle.
func protectedNopStackFixture(handlerNop, reference bool) []byte {
	code := []byte{0x1a, 0x10, 7} // iload_0; bipush 7
	labels := map[string]int{}
	type branch struct {
		pc    int
		label string
	}
	var branches []branch
	jump := func(op byte, label string) {
		branches = append(branches, branch{len(code), label})
		code = append(code, op, 0, 0)
	}
	jump(0xa0, "work") // if_icmpne
	if reference {
		code = append(code, 0x01) // cached path leaves null
	} else {
		code = append(code, 0x10, 99)
	}
	jump(0xa7, "merge")
	labels["work"] = len(code)
	code = append(code, 0x00) // NOP owns the following try's structural anchor
	start := len(code)
	if reference {
		code = append(code, 0x1a, 0xb8, 0, 15) // Hooks.evaluate(argument)
	} else {
		code = append(code, 0x10, 40, 0x1a, 0x6c) // 40 / argument
	}
	end := len(code)
	jump(0xa7, "merge")
	handler := len(code)
	if handlerNop {
		code = append(code, 0x00)
	}
	code = append(code, 0x4c) // caught exception
	if reference {
		code = append(code, 0x1a, 0xb8, 0, 18) // Hooks.fallback(argument)
	} else {
		code = append(code, 0x10, 0xf6) // bipush -10
	}
	jump(0xa7, "merge")
	labels["merge"] = len(code)
	if reference {
		code = append(code, 0x4c, 0x2b, 0xb0) // astore; aload; areturn
	} else {
		code = append(code, 0x3c, 0x1b, 0x06, 0x60, 0xac) // store; load; +3; ireturn
	}
	for _, b := range branches {
		binary.BigEndian.PutUint16(code[b.pc+1:], uint16(int16(labels[b.label]-b.pc)))
	}

	var out bytes.Buffer
	u2 := func(v int) { _ = binary.Write(&out, binary.BigEndian, uint16(v)) }
	u4 := func(v int) { _ = binary.Write(&out, binary.BigEndian, uint32(v)) }
	utf := func(s string) { out.WriteByte(1); u2(len(s)); out.WriteString(s) }
	class := func(index int) { out.WriteByte(7); u2(index) }
	u4(0xcafebabe)
	u2(0)
	u2(49)
	if reference {
		u2(19)
	} else {
		u2(10)
	}
	utf("NopProtected")
	class(1)
	utf("java/lang/Object")
	class(3)
	utf("java/lang/ArithmeticException")
	class(5)
	utf("compute")
	if reference {
		utf("(I)Ljava/lang/Integer;")
	} else {
		utf("(I)I")
	}
	utf("Code")
	if reference {
		utf("Hooks")
		class(10)
		utf("evaluate")
		utf("(I)Ljava/lang/Integer;")
		out.WriteByte(12)
		u2(12)
		u2(13)
		out.WriteByte(10)
		u2(11)
		u2(14)
		utf("fallback")
		out.WriteByte(12)
		u2(16)
		u2(13)
		out.WriteByte(10)
		u2(11)
		u2(17)
	}
	u2(0x21)
	u2(2)
	u2(4)
	u2(0)
	u2(0)
	u2(1)
	u2(0x09)
	u2(7)
	u2(8)
	u2(1)
	u2(9)
	u4(12 + len(code) + 8)
	u2(2)
	u2(2)
	u4(len(code))
	out.Write(code)
	u2(1)
	u2(start)
	u2(end)
	u2(handler)
	u2(6)
	u2(0)
	u2(0)
	return out.Bytes()
}

func TestAdversarialProtectedNopStackMergeRoundTrip(t *testing.T) {
	t.Parallel()
	javac, java := t04Tools(t)
	for _, reference := range []bool{false, true} {
		for _, handlerNop := range []bool{false, true} {
			name := "direct-handler"
			if handlerNop {
				name = "nop-handler"
			}
			if reference {
				name += "-reference"
			}
			t.Run(name, func(t *testing.T) {
				raw := protectedNopStackFixture(handlerNop, reference)
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "NopProtected.class"), raw, 0644); err != nil {
					t.Fatal(err)
				}
				probe := filepath.Join(dir, "Probe.java")
				probeSource := `public class Probe {
  public static void main(String[] args) {
    for(int value:new int[]{-2,-1,0,1,2,7,8}) System.out.print(NopProtected.compute(value)+";");
  }
}`
				want := "-17;-37;-7;43;23;102;8;"
				if reference {
					probeSource = `class Hooks {
  static String trace="";
  static Integer evaluate(int value) {
    trace+="N";
    if(value==-1)throw new IllegalArgumentException();
    return Integer.valueOf(40/value);
  }
  static Integer fallback(int value) { trace+="C";return Integer.valueOf(-10); }
}
public class Probe {
  public static void main(String[] args) {
    for(int value:new int[]{-2,-1,0,1,2,7,8}) {
      Hooks.trace="";
      try { System.out.print(NopProtected.compute(value)+":"); }
      catch(RuntimeException e) { System.out.print(e.getClass().getSimpleName()+":"); }
      System.out.print(Hooks.trace+";");
    }
  }
}`
					want = "-20:N;IllegalArgumentException:N;-10:NC;40:N;20:N;null:;5:N;"
				}
				if err := os.WriteFile(probe, []byte(probeSource), 0644); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", dir, probe).CombinedOutput(); err != nil {
					t.Fatalf("compile probe: %v\n%s", err, out)
				}
				if got := t04RunJava(t, java, dir, "Probe"); got != want {
					t.Fatalf("original JVM=%q, arithmetic oracle=%q", got, want)
				}
				compiled := map[string]string{}
				for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
					var source string
					var err error
					if mode == "legacy" {
						source, err = Decompile(raw)
					} else {
						result, e := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
						source, err = result.Source, e
					}
					if err != nil {
						t.Fatal(err)
					}
					rebuilt, ok := compiled[source]
					if !ok {
						rebuilt = t.TempDir()
						path := filepath.Join(rebuilt, "NopProtected.java")
						if err := os.WriteFile(path, []byte(source), 0644); err != nil {
							t.Fatal(err)
						}
						if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, path).CombinedOutput(); err != nil {
							t.Fatalf("rebuild %s: %v\n%s\n%s", mode, err, out, source)
						}
						compiled[source] = rebuilt
					}
					if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "Probe"); got != want {
						t.Fatalf("%s returned %q want %q\n%s", mode, got, want, source)
					}
				}
			})
		}
	}
}
