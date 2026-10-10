package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Authored JVM bytecode may store a final capture on uninitialized THIS.
// Reorder only that original assignment, then execute the mutated class first
// as the oracle. The original Parent and driver stay unchanged in the candidate.
func TestAdversarialConstructorOriginalFixedCapturePathRoundTrip(t *testing.T) {
	const f = `class FixedCaptureParent{final int n;FixedCaptureParent(int n){if(n!=7)throw new IllegalArgumentException("input:"+n);this.n=n;}}class FixedCaptureChild extends FixedCaptureParent{final Object value;FixedCaptureChild(Object value){super(7);this.value=value;}}class FixedCaptureDriver{public static void main(String[]args){Object x=new Object();for(Object v:new Object[]{null,x}){FixedCaptureChild c=new FixedCaptureChild(v);if(c.n!=7||c.value!=v)throw new AssertionError("capture");System.out.println("7:"+(c.value==x));}for(int n:new int[]{-1,8})try{new FixedCaptureParent(n);throw new AssertionError("failure lost");}catch(IllegalArgumentException e){System.out.println(e.getMessage());}}}`
	testConstructorOriginalFixedCapturePathRoundTrip(t, f)
}

func testConstructorOriginalFixedCapturePathRoundTrip(t *testing.T, f string) {
	t.Helper()
	javac, java := t04Tools(t)
	for _, magic := range []int{0, 7, -129, 70000} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(strconv.Itoa(magic)+"/"+debug, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, strings.ReplaceAll(f, "7", strconv.Itoa(magic)), debug)
				obj, err := Parse(files["FixedCaptureChild.class"])
				if err != nil {
					t.Fatal(err)
				}
				for _, field := range obj.Fields {
					name, _ := sourceBridgeUTF8(obj, field.NameIndex)
					if name == "value" {
						field.AccessFlags |= 0x1000
					}
				}
				changed := false
				for _, m := range obj.Methods {
					name, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if name != "<init>" {
						continue
					}
					ops, ok := nativeEnumMethodOps(obj, m, nil)
					if !ok {
						t.Fatal("original instructions")
					}
					store := -1
					for i, op := range ops {
						if field := constructorMotionMember(obj, op, core.OP_PUTFIELD); field != nil && field.Name == "FixedCaptureChild" && field.Member == "value" && field.Description == "Ljava/lang/Object;" {
							if i < 2 || ops[i-2].Instr.OpCode != core.OP_ALOAD_0 || ops[i-1].Instr.OpCode != core.OP_ALOAD_1 {
								t.Fatal("unexpected capture")
							}
							store = int(ops[i-2].CurrentOffset)
						}
					}
					if store < 0 {
						t.Fatal("missing original store")
					}
					for _, a := range m.Attributes {
						if code, ok := a.(*CodeAttribute); ok {
							if store+5 >= len(code.Code) || len(code.ExceptionTable) != 0 {
								t.Fatal("capture range")
							}
							raw := append([]byte(nil), code.Code[store:store+5]...)
							raw = append(raw, code.Code[:store]...)
							raw = append(raw, code.Code[store+5:]...)
							code.Code, code.Attributes = raw, nil
							code.AttrLen = uint32(12 + len(raw))
							changed = true
						}
					}
				}
				if !changed {
					t.Fatal("no mutation")
				}
				files["FixedCaptureChild.class"] = obj.Bytes()
				original := t.TempDir()
				for n, b := range files {
					if err := os.WriteFile(filepath.Join(original, n), b, 0600); err != nil {
						t.Fatal(err)
					}
				}
				oracle := t04RunJava(t, java, original, "FixedCaptureDriver")
				if oracle != strconv.Itoa(magic)+":false\n"+strconv.Itoa(magic)+":true\ninput:-1\ninput:8\n" {
					t.Fatal(oracle)
				}
				resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
				for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
					var source string
					if mode == "legacy" {
						source, err = DecompileWithResolver(files["FixedCaptureChild.class"], resolve)
					} else {
						var r DecompileResult
						r, err = DecompileWithOptions(files["FixedCaptureChild.class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
						source = r.Source
					}
					if err != nil || strings.Contains(source, DecompileStubMarker) {
						t.Fatalf("%s:%v\n%s", mode, err, source)
					}
					out := t.TempDir()
					path := filepath.Join(out, "FixedCaptureChild.java")
					if err := os.WriteFile(path, []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
					if raw, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", out, path).CombinedOutput(); err != nil {
						t.Fatalf("rebuild:%v\n%s\n%s", err, raw, source)
					}
					if got := t04RunJava(t, java, out+string(os.PathListSeparator)+original, "FixedCaptureDriver"); got != oracle {
						t.Fatalf("got=%q want=%q", got, oracle)
					}
				}
			})
		}
	}
}
