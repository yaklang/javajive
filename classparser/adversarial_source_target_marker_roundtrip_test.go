package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const emptyAbstractBridgeFixture = `class EmptyAbstractBridgeScope{static class Parent{private Parent(){}long value(){return 0;}}static final class Child extends Parent{private Child(){super();}long value(){BridgeEffects.trace+="V";if(BridgeEffects.fail)throw BridgeEffects.error;return BridgeEffects.word;}}static Object make(){return new Child();}static long value(Object input){return ((Parent)input).value();}}
class EmptyBridgeDriver{public static void main(String[]args)throws Exception{Class<?>marker=Class.forName("EmptyAbstractBridgeScope$1");if(marker.getDeclaredFields().length!=0)throw new AssertionError("marker");int rows=0;for(long word:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){BridgeEffects.trace="";BridgeEffects.word=word;BridgeEffects.fail=fail;try{Object value=EmptyAbstractBridgeScope.make();if(EmptyAbstractBridgeScope.make()==value||EmptyAbstractBridgeScope.value(value)!=word||fail||!BridgeEffects.trace.equals("V"))throw new AssertionError("virtual dispatch/words/effects");}catch(RuntimeException e){if(!fail||e!=BridgeEffects.error||!BridgeEffects.trace.equals("V"))throw new AssertionError("failure identity/partial effect",e);}rows++;}System.out.println(rows+":empty-abstract-private-super:marker:dispatch:identity:effects");}}`

// The same original Java8 bridge family must remain executable and complete
// when the source compiler uses nestmate access instead of synthetic bridges.
func TestAdversarialSourceTargetKeepsOriginalBridgeMarkerRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	for _, owner := range []string{"EmptyAbstractBridgeScope", "RenamedMarkerScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				source := strings.ReplaceAll(emptyAbstractBridgeFixture, "EmptyAbstractBridgeScope", owner) + `class BridgeEffects{static long word;static String trace;static boolean fail;static final RuntimeException error=new RuntimeException("identity");}`
				files := nativeCompileDebugClasses(t, source, debug)
				// Retain the original physical Java8 constructor bridge while independently
				// describing the private abstract declaration in every InnerClasses table.
				for name, raw := range files {
					cf, e := Parse(raw)
					if e != nil {
						t.Fatal(e)
					}
					for _, a := range cf.Attributes {
						if table, ok := a.(*InnerClassesAttribute); ok {
							for _, row := range table.Classes {
								n, known := sourceBridgeClassName(cf, row.InnerClassInfoIndex)
								if known && (n == owner+"$Parent" || n == owner+"$Child") {
									row.InnerClassAccessFlags = (row.InnerClassAccessFlags &^ uint16(7)) | 2
								}
								if known && n == owner+"$Parent" {
									row.InnerClassAccessFlags |= 0x400
								}
							}
						}
					}
					if name == owner+"$Parent.class" {
						cf.AccessFlags |= 0x400
						for _, m := range cf.Methods {
							n, _ := sourceBridgeUTF8(cf, m.NameIndex)
							if n == "value" {
								m.AccessFlags |= 0x400
								var attrs []AttributeInfo
								for _, a := range m.Attributes {
									if _, ok := a.(*CodeAttribute); !ok {
										attrs = append(attrs, a)
									}
								}
								m.Attributes = attrs
							}
						}
					}
					files[name] = cf.Bytes()
				}
				original := t.TempDir()
				for name, raw := range files {
					if e := os.WriteFile(filepath.Join(original, name), raw, 0600); e != nil {
						t.Fatal(e)
					}
				}
				want := t04RunJava(t, java, original, "EmptyBridgeDriver")
				if want != "8:empty-abstract-private-super:marker:dispatch:identity:effects\n" {
					t.Fatal(want)
				}
				for _, release := range []int{8, 11} {
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(strconv.Itoa(release)+"/"+policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							archivePath := filepath.Join(t.TempDir(), "original.jar")
							if e := os.WriteFile(archivePath, t23Zip(t, files), 0600); e != nil {
								t.Fatal(e)
							}
							z, e := NewJarFSFromLocalWithSourceVersion(archivePath, release, nil)
							if e != nil {
								t.Fatal(e)
							}
							defer z.Close()
							out := t.TempDir()
							var paths []string
							for name, raw := range files {
								if !strings.HasPrefix(name, owner) {
									if e := os.WriteFile(filepath.Join(out, name), raw, 0600); e != nil {
										t.Fatal(e)
									}
									continue
								}
								src, e := z.ReadFile(name)
								if e != nil || strings.Contains(string(src), DecompileStubMarker) {
									t.Fatalf("source %s:%v\n%s", name, e, src)
								}
								path := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
								if e := os.WriteFile(path, src, 0600); e != nil {
									t.Fatal(e)
								}
								paths = append(paths, path)
							}
							args := append([]string{"-proc:none", "--release", strconv.Itoa(release), "-cp", out, "-d", out}, paths...)
							if log, e := exec.Command(javac, args...).CombinedOutput(); e != nil {
								t.Fatalf("compile:%v\n%s", e, log)
							}
							if got := t04RunJava(t, java, out, "EmptyBridgeDriver"); got != want {
								t.Fatalf("got %q expected %q", got, want)
							}
							for name := range files {
								if _, e := os.Stat(filepath.Join(out, name)); e != nil {
									t.Fatalf("original class disappeared:%s:%v", name, e)
								}
							}
						})
					}
				}
			})
		}
	}
}
