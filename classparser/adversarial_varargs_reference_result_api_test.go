package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const publicVarargsResultFixture = `import java.util.function.IntFunction;
interface ConvertedResultAPIShort {short apply(byte value);}
class ConvertedResultAPITarget {
 int calls;boolean fail;static Object prior;static final RuntimeException failure=new RuntimeException("original result payload");
 void enter(Object values){if(values==prior)throw new AssertionError("reused pack");prior=values;calls++;if(fail)throw failure;}
 public int ints(int... values){enter(values);return values[0];}
 public byte bytes(byte... values){enter(values);return values[0];}
}
class ConvertedResultAPIOwner {
 static IntFunction<Number> boxed(ConvertedResultAPITarget target){return target::ints;}
 static ConvertedResultAPIShort widened(ConvertedResultAPITarget target){return target::bytes;}
}
class ConvertedResultAPIDriver {
 static void require(boolean value){if(!value)throw new AssertionError("public result conversion frontier");}
 public static void main(String[] args){ConvertedResultAPITarget target=new ConvertedResultAPITarget();int rows=0;
  for(int value:new int[]{Integer.MIN_VALUE,-1,0,129,Integer.MAX_VALUE}){int before=target.calls;Number actual=ConvertedResultAPIOwner.boxed(target).apply(value);require(actual.getClass()==Integer.class&&actual.intValue()==value&&target.calls==before+1&&((int[])ConvertedResultAPITarget.prior)[0]==value);rows++;}
  for(byte value:new byte[]{Byte.MIN_VALUE,-1,0,1,Byte.MAX_VALUE}){int before=target.calls;short actual=ConvertedResultAPIOwner.widened(target).apply(value);require(actual==(short)value&&target.calls==before+1&&((byte[])ConvertedResultAPITarget.prior)[0]==value);rows++;}
  for(int route=0;route<2;route++){int before=target.calls;Throwable caught=null;try{if(route==0)ConvertedResultAPIOwner.boxed(null);else ConvertedResultAPIOwner.widened(null);}catch(Throwable t){caught=t;}require(caught instanceof NullPointerException&&target.calls==before);rows++;}
  target.fail=true;for(int route=0;route<2;route++){int before=target.calls;Throwable caught=null;try{if(route==0)ConvertedResultAPIOwner.boxed(target).apply(3);else ConvertedResultAPIOwner.widened(target).apply((byte)3);}catch(Throwable t){caught=t;}require(caught==ConvertedResultAPITarget.failure&&target.calls==before+1);rows++;}
  require(rows==14);System.out.println("14:public:result:conversion:boundary");
 }
}`

func TestAdversarialVarargsResultsPreserveBehaviorThroughBothPublicModes(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			for _, named := range []bool{false, true} {
				label := "absent"
				if named {
					label = "named"
				}
				t.Run(label, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							files := compileLambdaParameterMetadataFixture(t, compiler, profile, publicVarargsResultFixture, "ConvertedResultAPIOwner", debug, named)
							write := func(root, name string, raw []byte) {
								t.Helper()
								if err := os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							original := t.TempDir()
							for n, raw := range files {
								write(original, n, raw)
							}
							want := t04RunJava(t, java, original, "ConvertedResultAPIDriver")
							if want != "14:public:result:conversion:boundary\n" {
								t.Fatal("independent original timing model", want)
							}
							for _, mode := range []DecompileMode{Precision, Compatibility} {
								t.Run(string(mode), func(t *testing.T) {
									input := bytes.Clone(files["ConvertedResultAPIOwner.class"])
									result, err := DecompileWithOptions(input, DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }})
									if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) || !bytes.Equal(input, files["ConvertedResultAPIOwner.class"]) {
										t.Fatalf("public mode status=%s error=%v diagnostics=%+v\n%s", result.Status, err, result.Diagnostics, result.Source)
									}
									out := t.TempDir()
									for n, raw := range files {
										if n != "ConvertedResultAPIOwner.class" {
											write(out, n, raw)
										}
									}
									file := filepath.Join(out, "ConvertedResultAPIOwner.java")
									write(out, "ConvertedResultAPIOwner.java", []byte(result.Source))
									args := []string{"-proc:none", "--release", "8"}
									if profile == NativeJavac8 {
										args = []string{"-proc:none", "-source", "8", "-target", "8"}
									}
									if named {
										args = append(args, "-parameters")
									}
									args = append(args, "-g:"+debug, "-classpath", out, "-sourcepath", out, "-d", out, file)
									if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
										t.Fatalf("compile without original owner:%v\n%s\n%s", err, log, result.Source)
									}
									if got := t04RunJava(t, java, out, "ConvertedResultAPIDriver"); got != want {
										t.Fatalf("public mode original-first behavior=%q want=%q", got, want)
									}
									candidate, err := os.ReadFile(filepath.Join(out, "ConvertedResultAPIOwner.class"))
									if err != nil {
										t.Fatal(err)
									}
									nativeLexicalExactSignatures(t, "ConvertedResultAPIOwner.class", input, candidate)
									if x, y := lambdaParameterMetadataTables(t, input), lambdaParameterMetadataTables(t, candidate); x != y {
										t.Fatalf("public mode changed original capture/parameter roles\n%s\n%s", x, y)
									}
								})
							}
						})
					}
				})
			}
		})
	}
}
