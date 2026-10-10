package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// PUTFIELD narrows an int-category word for B/C/S/Z storage, while DUP
// retains the original word. Java assignment expressions return the converted
// field value. Execute valid original bytecode before checking this boundary.
func TestNativeAnonymousNestedNarrowStoreKeepsDuplicatedWord(t *testing.T) {
	javac, java := t04Tools(t)
	for _, packet := range []struct {
		descriptor   string
		word, stored int
	}{{"B", 257, 1}, {"S", 65537, 1}, {"C", 65537, 1}, {"Z", 3, 1}, {"Z", 2, 0}, {"Z", -2, 0}, {"Z", -1, 1}, {"Z", 0, 0}, {"Z", 1, 1}, {"Z", -2147483648, 0}, {"Z", 2147483647, 1}} {
		t.Run(fmt.Sprintf("%s/%d", packet.descriptor, packet.word), func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					fixture := fmt.Sprintf(`abstract class NarrowStoreParent {abstract int stored();abstract int copied();}
class NarrowStoreOwner {NarrowStoreParent make(){return new NarrowStoreParent(){int narrow,wide;{wide=(narrow=%d);}int stored(){return narrow;}int copied(){return wide;}};}}
class NarrowStoreDriver {public static void main(String[]args){NarrowStoreParent p=new NarrowStoreOwner().make();if(p.stored()!=%d||p.copied()!=%d)throw new AssertionError("DUP retains original word independently of narrow field storage:"+p.stored()+":"+p.copied());System.out.println("narrow:original:word");}}`, packet.word, packet.stored, packet.word)
					files := nativeCompileDebugClasses(t, fixture, debug)
					obj, e := Parse(files["NarrowStoreOwner$1.class"])
					if e != nil {
						t.Fatal(e)
					}
					descriptor := uint16(obj.ConstantPoolManager.AddUtf8Info(packet.descriptor))
					fields, refs := 0, 0
					for _, f := range obj.Fields {
						name, _ := sourceBridgeUTF8(obj, f.NameIndex)
						if name == "narrow" {
							f.DescriptorIndex = descriptor
							fields++
						}
					}
					for _, c := range obj.ConstantPool {
						if nt, ok := c.(*ConstantNameAndTypeInfo); ok {
							name, _ := sourceBridgeUTF8(obj, nt.NameIndex)
							desc, _ := sourceBridgeUTF8(obj, nt.DescriptorIndex)
							if name == "narrow" && desc == "I" {
								nt.DescriptorIndex = descriptor
								refs++
							}
						}
					}
					if fields != 1 || refs != 1 {
						t.Fatalf("unique declaration/ref=%d/%d", fields, refs)
					}
					files["NarrowStoreOwner$1.class"] = obj.Bytes()
					original := t.TempDir()
					for n, raw := range files {
						if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
					oracle := t04RunJava(t, java, original, "NarrowStoreDriver")
					if oracle != "narrow:original:word\n" {
						t.Fatalf("valid original=%q", oracle)
					}
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							z := nativeArchive(t, files)
							defer z.Close()
							out := t.TempDir()
							paths := []string{}
							refused := false
							for n, raw := range files {
								if !strings.HasPrefix(n, "NarrowStoreOwner") {
									if e := os.WriteFile(filepath.Join(out, n), raw, 0600); e != nil {
										t.Fatal(e)
									}
									continue
								}
								source, e := z.ReadFile(n)
								if e != nil {
									t.Fatal(e)
								}
								if strings.Contains(string(source), DecompileStubMarker) {
									refused = true
									continue
								}
								p := filepath.Join(out, strings.TrimSuffix(n, ".class")+".java")
								if e := os.WriteFile(p, source, 0600); e != nil {
									t.Fatal(e)
								}
								paths = append(paths, p)

							}
							if refused {
								t.Fatal("valid original narrowing/shared-word packet was not reconstructed")
							}
							if data, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, paths...)...).CombinedOutput(); e != nil {
								t.Fatalf("unsafe projected source:%v\n%s", e, data)
							}
							if got := t04RunJava(t, java, out, "NarrowStoreDriver"); got != oracle {
								t.Fatalf("rebuilt=%q original=%q", got, oracle)
							}
							for n, original := range files {
								if !strings.HasPrefix(n, "NarrowStoreOwner") {
									continue
								}
								raw, e := os.ReadFile(filepath.Join(out, n))
								if e != nil {
									t.Fatal(e)
								}
								if want, got := nativeBinaryShape(t, original), nativeBinaryShape(t, raw); got != want {
									t.Fatalf("original ABI %s changed\n%s\n%s", n, want, got)
								}
							}
						})
					}
				})
			}
		})
	}
}
