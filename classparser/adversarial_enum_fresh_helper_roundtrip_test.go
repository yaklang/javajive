package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

const enumFreshHelperFixture = `public enum PacketValues {FIRST,MIDDLE,LAST;
 public static long fold(long input){long result=input;for(PacketValues value:values())result=result*31+value.ordinal();return result;}
}
class PacketValuesDriver {public static void main(String[]args)throws Exception {
 int rows=0;for(long input:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){long expected=input;
  for(int ordinal=0;ordinal<3;ordinal++)expected=java.math.BigInteger.valueOf(expected).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(ordinal)).longValue();
  if(PacketValues.fold(input)!=expected)throw new AssertionError("enum order/overflow");
  PacketValues[] first=PacketValues.values(),second=PacketValues.class.getEnumConstants();if(first==second||first.length!=3||second.length!=3)throw new AssertionError("array identity");
  for(int i=0;i<3;i++)if(first[i]!=second[i]||first[i].ordinal()!=i||PacketValues.valueOf(first[i].name())!=first[i])throw new AssertionError("constant identity");
  first[0]=null;if(PacketValues.values()[0]!=PacketValues.FIRST)throw new AssertionError("fresh clone");rows++;
 }
 if(PacketValues.class.getDeclaredMethods().length!=3)throw new AssertionError("extra fresh compiler helper is observable");
 System.out.println(rows+":enum:values:clone:binding:overflow:method-universe");
}}
`

func enumFreshOracleRun(t *testing.T, java, mode, dir, driver string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	log, err := exec.CommandContext(ctx, java, "-Xverify:all", mode, "-cp", dir, driver).CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("enum oracle exceeded its per-world deadline: %s %s\n%s", driver, mode, log)
	}
	return log, err
}

func TestAdversarialEnumFreshHelperPreservesOriginalMethodUniverse(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, owner := range []string{"PacketValues", "RenamedPacketValues"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				fixture := strings.ReplaceAll(enumFreshHelperFixture, "PacketValues", owner)
				driver := owner + "Driver"
				files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, false)
				run := func(dir string, want bool) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						out, err := enumFreshOracleRun(t, java, mode, dir, driver)
						if want && (err != nil || string(out) != "5:enum:values:clone:binding:overflow:method-universe\n") {
							t.Fatalf("original-first enum observer: %v\n%s", err, out)
						}
						if !want && (err == nil || !strings.Contains(string(out), "extra fresh compiler helper is observable")) {
							t.Fatalf("fresh helper negative control: %v\n%s", err, out)
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files), true)
				jar := filepath.Join(t.TempDir(), "enum.jar")
				if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"]}), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				src, err := a.ReadFile(owner + ".class")
				if err != nil || legacySourceHasFailureMarker(string(src)) {
					t.Fatalf("enum source: %v\n%s", err, src)
				}
				out := sourceArtifactWriteFiles(t, map[string][]byte{driver + ".class": files[driver+".class"]})
				path := filepath.Join(out, owner+".java")
				if err := os.WriteFile(path, src, 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
					t.Fatal("source only enum compile", err, string(log))
				}
				run(out, false)
				fresh, err := os.ReadFile(filepath.Join(out, owner+".class"))
				if err != nil {
					t.Fatal(err)
				}
				if p, err := a.source.RestoreClassMetadata(owner+".class", fresh); err == nil || p != nil {
					t.Fatal("ordinary metadata gate incorrectly permits extra helper")
				}
				finished, err := a.FinishClass(owner+".class", fresh)
				if err != nil {
					t.Fatal("explicit reconstruction must discharge the fresh array helper before metadata", err)
				}
				if err := os.WriteFile(filepath.Join(out, owner+".class"), finished, 0600); err != nil {
					t.Fatal(err)
				}
				run(out, true)
			})
		}
	}
}

func TestAdversarialEnumFreshHelperBoundaryCountsKeepFreshInstructions(t *testing.T) {
	compiler, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	// Keep the test's filesystem paths ASCII. Some supported host JVMs fail
	// their classpath URI decoding before loading an original fixture when
	// testing.T places a supplementary character in its temporary directory.
	// The actual class name and descriptors still exercise the full UTF-16 name.
	for ownerIndex, owner := range []string{"FreshBoundaryChoice", "边界𐐀Choice"} {
		for _, count := range []int{0, 1, 129} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("owner%d/%d/%s", ownerIndex, count, debug), func(t *testing.T) {
					constants := make([]string, count)
					for i := range constants {
						constants[i] = fmt.Sprintf("C%d", i)
					}
					fixture := "public enum " + owner + "{" + strings.Join(constants, ",") + `;
 public static long fold(long input){long result=input;for(` + owner + ` value:values())result=result*31+value.ordinal();return result;}}
class ` + owner + `Driver{public static void main(String[]args)throws Exception{
 int count=` + fmt.Sprint(count) + `,rows=0;
 if(!` + owner + `.class.isEnum()||java.util.EnumSet.allOf(` + owner + `.class).size()!=count||` + owner + `.class.getDeclaredMethods().length!=3)throw new AssertionError("complete original enum universe");
 for(long input:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){long expected=input;
  for(int i=0;i<count;i++)expected=java.math.BigInteger.valueOf(expected).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(i)).longValue();
  if(` + owner + `.fold(input)!=expected)throw new AssertionError("ordered array fold/overflow");
  ` + owner + `[] left=` + owner + `.values(),right=` + owner + `.class.getEnumConstants();if(left==right||left.length!=count||right.length!=count)throw new AssertionError("array identity/length");
  for(int i=0;i<count;i++)if(left[i]!=right[i]||left[i].ordinal()!=i||!left[i].name().equals("C"+i)||` + owner + `.valueOf(left[i].name())!=left[i])throw new AssertionError("literal/ordinal/field binding");
  if(count>0){left[0]=null;if(` + owner + `.values()[0]==null)throw new AssertionError("clone mutation escaped");}rows++;
 }System.out.println(rows+":enum:boundary:"+count+":identity:overflow");}}
`
					files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, false)
					expect := fmt.Sprintf("5:enum:boundary:%d:identity:overflow\n", count)
					run := func(dir string) {
						t.Helper()
						for _, mode := range []string{"-ea", "-da"} {
							out, err := enumFreshOracleRun(t, java, mode, dir, owner+"Driver")
							if err != nil || string(out) != expect {
								t.Fatal("unchanged original-first boundary oracle", err, string(out))
							}
						}
					}
					run(sourceArtifactWriteFiles(t, files))
					jar := filepath.Join(t.TempDir(), "enum.jar")
					if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"]}), 0600); err != nil {
						t.Fatal(err)
					}
					a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					source, err := a.ReadFile(owner + ".class")
					if err != nil || legacySourceHasFailureMarker(string(source)) {
						t.Fatal("boundary source", err, string(source))
					}
					out := sourceArtifactWriteFiles(t, map[string][]byte{owner + "Driver.class": files[owner+"Driver.class"]})
					path := filepath.Join(out, owner+".java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(compiler, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
						t.Fatal("owned enum source-only compilation", err, string(log))
					}
					fresh, err := os.ReadFile(filepath.Join(out, owner+".class"))
					if err != nil {
						t.Fatal(err)
					}
					before := bytes.Clone(fresh)
					object := mustParseLowering(t, bytes.Clone(fresh))
					var helper, initializer *MemberInfo
					for _, method := range object.Methods {
						name, _ := sourceBridgeUTF8(object, method.NameIndex)
						if name == "<clinit>" {
							initializer = method
						} else if method.AccessFlags == 0x100a {
							helper = method
						}
					}
					if helper == nil || initializer == nil {
						t.Fatal("fresh generated helper absent")
					}
					code, packet := constructorLoweringCode(initializer), constructorLoweringCode(helper)
					ops, known := nativeEnumMethodOps(object, initializer, nil)
					if !known || len(ops) < 3 || !nativeEnumOpcode(ops[len(ops)-3], core.OP_INVOKESTATIC) || packet.Code[len(packet.Code)-1] != 0xb0 {
						t.Fatal("independent fresh instruction composition")
					}
					pc := int(ops[len(ops)-3].CurrentOffset)
					wantCode := append(bytes.Clone(code.Code[:pc]), packet.Code[:len(packet.Code)-1]...)
					wantCode = append(wantCode, code.Code[pc+3:]...)
					finished, err := a.FinishClass(owner+".class", fresh)
					if err != nil || !bytes.Equal(fresh, before) {
						t.Fatal("boundary transaction/immutable fresh bytes", err)
					}
					result := mustParseLowering(t, finished)
					for _, method := range result.Methods {
						name, _ := sourceBridgeUTF8(result, method.NameIndex)
						if name == "<clinit>" {
							if !bytes.Equal(constructorLoweringCode(method).Code, wantCode) {
								t.Fatal("initializer must use only composed fresh instructions")
							}
						} else {
							desc, _ := sourceBridgeUTF8(result, method.DescriptorIndex)
							matched := false
							for _, prior := range object.Methods {
								n, _ := sourceBridgeUTF8(object, prior.NameIndex)
								d, _ := sourceBridgeUTF8(object, prior.DescriptorIndex)
								if n == name && d == desc {
									matched = bytes.Equal(constructorLoweringCode(prior).Code, constructorLoweringCode(method).Code)
								}
							}
							if !matched {
								t.Fatal("unrelated fresh method body changed", name)
							}
						}
					}
					if again, err := a.FinishClass(owner+".class", finished); err != nil || !bytes.Equal(again, finished) {
						t.Fatal("boundary completion is not idempotent", err)
					}
					if err := os.WriteFile(filepath.Join(out, owner+".class"), finished, 0600); err != nil {
						t.Fatal(err)
					}
					run(out)
				})
			}
		}
	}
}

func TestAdversarialEnumFreshHelperNativeOwnerMustReturnWholeSource(t *testing.T) {
	compiler, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, owner := range []string{"NativeEnumHost", "RenamedNativeEnumHost"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				fixture := `public class ` + owner + `{public enum Mode{A,B,C;public static long fold(long input){long result=input;for(Mode x:values())result=result*31+x.ordinal();return result;}}}
class NativeEnumDriver{public static void main(String[]args){int rows=0;
 if(` + owner + `.Mode.class.getDeclaringClass()!=` + owner + `.class||` + owner + `.Mode.class.getDeclaredMethods().length!=3)throw new AssertionError("native enum declaration universe");
 for(long input:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){long expected=input;for(int i=0;i<3;i++)expected=java.math.BigInteger.valueOf(expected).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(i)).longValue();if(` + owner + `.Mode.fold(input)!=expected)throw new AssertionError("native enum values binding/overflow");rows++;}
 System.out.println(rows+":native:enum:owner:method-universe:binding");}}
`
				files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, false)
				run := func(dir string) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						log, err := enumFreshOracleRun(t, java, mode, dir, "NativeEnumDriver")
						if err != nil || string(log) != "5:native:enum:owner:method-universe:binding\n" {
							t.Fatal("original-first native owner observer", err, string(log))
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files))
				child := owner + "$Mode.class"
				jar := filepath.Join(t.TempDir(), "owner.jar")
				if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"], child: files[child]}), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				comment, err := a.ReadFile(child)
				if err != nil || !strings.Contains(string(comment), "original member body owned") {
					t.Fatal("native child-first source control", err, string(comment))
				}
				// Reading through the older metadata source API may finish a
				// native family, but cannot discharge the new executable client.
				source, err := a.source.ReadFile(owner + ".class")
				if err != nil || legacySourceHasFailureMarker(string(source)) {
					t.Fatal("complete native family source", err, string(source))
				}
				out := sourceArtifactWriteFiles(t, map[string][]byte{"NativeEnumDriver.class": files["NativeEnumDriver.class"]})
				path := filepath.Join(out, owner+".java")
				if err := os.WriteFile(path, source, 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(compiler, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
					t.Fatal("no owned originals on compiler classpath", err, string(log))
				}
				fresh, err := os.ReadFile(filepath.Join(out, child))
				if err != nil {
					t.Fatal(err)
				}
				if got, err := a.FinishClass(child, fresh); err == nil || got != nil {
					t.Fatal("child comment or unpublished native owner granted executable edits")
				}
				// An owning-source conflict is discovered after origin lookup.
				// It must not activate even the otherwise valid enum child.
				authority := a.source.jar.sourceConstructorLowering
				authority.mu.Lock()
				if authority.active == nil {
					authority.active = map[string]*sourceConstructorLoweringPublication{}
				}
				authority.active[owner] = &sourceConstructorLoweringPublication{}
				authority.mu.Unlock()
				if _, err := a.ReadFile(owner + ".class"); err == nil {
					t.Fatal("conflicting source generations were not refused")
				}
				authority.mu.Lock()
				_, leaked := authority.enumActive[owner+"$Mode"]
				delete(authority.active, owner)
				authority.mu.Unlock()
				if leaked {
					t.Fatal("failed public source return activated enum edits")
				}
				returned, err := a.ReadFile(owner + ".class")
				if err != nil || !bytes.Equal(returned, source) {
					t.Fatal("exact whole-source publication", err)
				}
				for _, name := range []string{owner + ".class", child} {
					input, err := os.ReadFile(filepath.Join(out, name))
					if err != nil {
						t.Fatal(err)
					}
					finished, err := a.FinishClass(name, input)
					if err != nil {
						t.Fatal("native owning source must discharge its actual enum output", name, err)
					}
					if err := os.WriteFile(filepath.Join(out, name), finished, 0600); err != nil {
						t.Fatal(err)
					}
				}
				run(out)
			})
		}
	}
}
