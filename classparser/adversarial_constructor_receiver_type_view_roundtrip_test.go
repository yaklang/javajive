package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Only the child's original capture store is moved ahead of delegation. The
// parent and driver remain physical originals, checking both normal and abrupt
// paths, capture identity, shared throwables and the original effect trace.
func TestAdversarialConstructorReceiverTypeViewsKeepOriginalCaptureAndFailure(t *testing.T) {
	for _, shape := range []string{"instanceof-true", "instanceof-false", "checked-alias"} {
		t.Run(shape, func(t *testing.T) {
			fixture := constructorUnknownAbruptCaptureFixture + "\ninterface AbruptMarker {}\ninterface AbruptOtherMarker {}\n"
			// The immutable driver checks both rebuilt-target and retained-input
			// origins, so a missing candidate cannot fall back to its original.
			fixture = strings.Replace(fixture, "Object x=new Object();", `String[] cp=System.getProperty("java.class.path").split(java.io.File.pathSeparator);
             java.io.File targetRoot=new java.io.File(cp[0]).getCanonicalFile();
             java.io.File originalRoot=new java.io.File(cp[cp.length-1]).getCanonicalFile();
             if(!new java.io.File(AbruptChild.class.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(targetRoot))throw new AssertionError("target fallback");
             for(Class<?> type:new Class<?>[]{AbruptParent.class,AbruptDriver.class,AbruptMarker.class,AbruptOtherMarker.class})if(!new java.io.File(type.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(originalRoot))throw new AssertionError("retained producer/driver origin");
             Object x=new Object();`, 1)
			marker := shape != "instanceof-false"
			if marker {
				fixture = strings.Replace(fixture, "extends AbruptParent {", "extends AbruptParent implements AbruptMarker {", 1)
			}
			switch shape {
			case "instanceof-true", "instanceof-false":
				fixture = strings.Replace(fixture, "this.n=n;trace=trace*31+2;", "this.n=n+(this instanceof AbruptMarker?1:2);trace=trace*31+2;", 1)
				add := "2"
				if marker {
					add = "1"
				}
				fixture = strings.Replace(fixture, "c.n!=n||", "c.n!=n+"+add+"||", 1)
			case "checked-alias":
				fixture = strings.Replace(fixture, "this.n=n;trace=trace*31+2;", "Object alias=(AbruptMarker)this;if(alias!=this)throw marker;if(n==70000){Object bad=(AbruptOtherMarker)this;if(bad!=null)throw marker;}this.n=n;trace=trace*31+2;", 1)
				fixture = strings.Replace(fixture, "}catch(NullPointerException e){", "}catch(ClassCastException e){if(n!=70000||AbruptParent.trace!=1)throw new AssertionError(\"cast failure/order\");System.out.println(n+\":cast:\"+(value==x));}catch(NullPointerException e){", 1)
			}
			want := "-3:null:false\n-3:null:true\n-2:fresh:false\n-2:fresh:true\n-1:shared:false\n-1:shared:true\n0:ok:false\n0:ok:true\n7:ok:false\n7:ok:true\n70000:ok:false\n70000:ok:true\n"
			if shape == "checked-alias" {
				want = strings.ReplaceAll(want, "70000:ok:", "70000:cast:")
			}
			for _, name := range []string{"Abrupt", "Cinder"} {
				t.Run(name, func(t *testing.T) {
					testConstructorReceiverTypeViewFixture(t, strings.ReplaceAll(fixture, "Abrupt", name), name, want)
				})
			}
		})
	}
}

func testConstructorReceiverTypeViewFixture(t *testing.T, fixture, prefix string, expectedOutput ...string) {
	t.Helper()
	javac, java := t04Tools(t)
	child, driver := prefix+"Child", prefix+"Driver"
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			obj, err := Parse(files[child+".class"])
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range obj.Fields {
				name, _ := sourceBridgeUTF8(obj, f.NameIndex)
				if name == "value" {
					f.AccessFlags |= 0x1000
				}
			}
			changed := false
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name != "<init>" {
					continue
				}
				var originalCode *CodeAttribute
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						if originalCode != nil {
							t.Fatal("duplicate original code")
						}
						originalCode = c
					}
				}
				if originalCode == nil {
					t.Fatal("missing original code")
				}
				decoder := core.NewDecompiler(originalCode.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				ops := constructorMotionOps(decoder)
				store := -1
				for i, op := range ops {
					member := constructorMotionMember(obj, op, core.OP_PUTFIELD)
					if member != nil && member.Name == child && member.Member == "value" && member.Description == "Ljava/lang/Object;" {
						if i < 2 || ops[i-2].Instr.OpCode != core.OP_ALOAD_0 || ops[i-1].Instr.OpCode != core.OP_ALOAD_1 {
							t.Fatal("capture packet")
						}
						store = int(ops[i-2].CurrentOffset)
					}
				}
				if store < 0 {
					t.Fatal("missing original capture")
				}
				for _, a := range m.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						if store+5 >= len(code.Code) || len(code.ExceptionTable) != 0 {
							t.Fatal("capture boundary")
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
			files[child+".class"] = obj.Bytes()
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, driver)
			expected := "-3:null:false\n-3:null:true\n-2:fresh:false\n-2:fresh:true\n-1:shared:false\n-1:shared:true\n0:ok:false\n0:ok:true\n7:ok:false\n7:ok:true\n70000:ok:false\n70000:ok:true\n"
			if len(expectedOutput) == 1 {
				expected = expectedOutput[0]
			} else if len(expectedOutput) > 1 {
				t.Fatal("ambiguous original oracle")
			}
			if oracle != expected {
				t.Fatalf("original oracle=%q", oracle)
			}
			resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy", "archive"} {
				t.Run(string(mode), func(t *testing.T) {
					var source string
					if mode == "legacy" {
						source, err = DecompileWithResolver(files[child+".class"], resolve)
					} else if mode == "archive" {
						archive := nativeArchive(t, files)
						defer archive.Close()
						archive.targetSourceVersion = 8
						raw, readErr := archive.ReadFile(child + ".class")
						source, err = string(raw), readErr
					} else {
						var r DecompileResult
						r, err = DecompileWithOptions(files[child+".class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
						source = r.Source
					}
					if err != nil || strings.Contains(source, DecompileStubMarker) {
						t.Fatalf("%s:%v\n%s", mode, err, source)
					}
					out := t.TempDir()
					path := filepath.Join(out, child+".java")
					if err := os.WriteFile(path, []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
					if raw, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", out, path).CombinedOutput(); err != nil {
						t.Fatalf("rebuild:%v\n%s\n%s", err, raw, source)
					}
					if _, e := os.Stat(filepath.Join(out, child+".class")); e != nil {
						t.Fatalf("requested rebuilt class missing: %v", e)
					}
					if got := t04RunJava(t, java, out+string(os.PathListSeparator)+original, driver); got != oracle {
						t.Fatalf("got=%q want=%q", got, oracle)
					}
				})
			}
		})
	}
}
