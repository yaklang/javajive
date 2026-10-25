package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const enumEntryEffectFixture = `class PreludeProbe {
 static String trace="";
 static int choose(){trace+="P;";String text=System.getProperty("prelude.mode");if(text.equals("fail"))throw new IllegalArgumentException("prelude");return Integer.parseInt(text);}
 static void record(int ordinal,int mode){trace+="C"+ordinal+":"+mode+";";}
}
public enum PreludeChoice {
 A,B;static int mode;
 PreludeChoice(){PreludeProbe.record(ordinal(),read());}
 static int read(){return mode;}
 static {mode=PreludeProbe.choose();}
}
class PreludeDriver {public static void main(String[] args)throws Exception {
 System.setProperty("prelude.mode",args[0]);String expected="P;";
 if(args[0].equals("fail")){
  try{Class.forName("PreludeChoice");throw new AssertionError("missing prelude failure");}
  catch(ExceptionInInitializerError error){if(!(error.getCause() instanceof IllegalArgumentException)||!error.getCause().getMessage().equals("prelude"))throw new AssertionError(error);}
  try{Class.forName("PreludeChoice");throw new AssertionError("erroneous class reused");}catch(NoClassDefFoundError error){}
 }else{
  int mode=Integer.parseInt(args[0]);PreludeChoice[] values=PreludeChoice.values();
  if(values.length!=2||PreludeChoice.read()!=mode)throw new AssertionError("state");
  for(int i=0;i<2;i++){expected+="C"+i+":"+mode+";";if(values[i].ordinal()!=i||PreludeChoice.valueOf(values[i].name())!=values[i])throw new AssertionError("identity");}
  values[0]=null;if(PreludeChoice.values()[0]==null)throw new AssertionError("array alias");
 }
 if(!expected.equals(PreludeProbe.trace))throw new AssertionError(PreludeProbe.trace+" != "+expected);
 System.out.println("prelude:before:allocation:failure:identity");}}
`

// Author an instrumenter's entry edge, independently of the production proof.
// javac8 ends this fixture with INVOKESTATIC choose; PUTSTATIC mode; RETURN.
// Move only that six-byte effect packet behind a return and route entry through
// it. Class version 49 uses the JVM's verifier rather than authored frame data.
func enumEntryEffectOriginal(t *testing.T, raw []byte) []byte {
	t.Helper()
	obj := mustParseLowering(t, bytes.Clone(raw))
	for _, method := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, method.NameIndex)
		if name != "<clinit>" {
			continue
		}
		code := constructorLoweringCode(method)
		if obj.MajorVersion != 49 || code == nil || code.MaxLocals != 0 || len(code.ExceptionTable) != 0 || len(code.Code) < 8 {
			t.Fatal("authored native initializer shape")
		}
		old := bytes.Clone(code.Code)
		cut := len(old) - 7
		if old[cut] != 0xb8 || old[cut+3] != 0xb3 || old[len(old)-1] != 0xb1 {
			t.Fatal("authored six-byte effect packet")
		}
		body := make([]byte, len(old)+6)
		prelude := cut + 4
		body[0] = 0xa7
		binary.BigEndian.PutUint16(body[1:], uint16(prelude))
		copy(body[3:], old[:cut])
		body[cut+3] = 0xb1
		copy(body[prelude:], old[cut:cut+6])
		body[prelude+6] = 0xa7
		binary.BigEndian.PutUint16(body[prelude+7:], uint16(int16(3-(prelude+6))))
		code.Code = body
		for _, attr := range code.Attributes {
			switch table := attr.(type) {
			case *LineNumberTableAttribute:
				for _, row := range table.LineNumberTable {
					pc := int(row.StartPc)
					if pc < cut {
						row.StartPc += 3
					} else if pc < len(old)-1 {
						row.StartPc = uint16(prelude + pc - cut)
					} else {
						row.StartPc = uint16(cut + 3)
					}
				}
			default:
				t.Fatalf("unmodeled authored initializer attribute %T", attr)
			}
		}
		code.AttrLen += 6
		return obj.Bytes()
	}
	t.Fatal("missing initializer")
	return nil
}

func TestAdversarialEnumEntryEffectsPrecedeConstantAllocation(t *testing.T) {
	javac, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		for _, owner := range []string{"PreludeChoice", "ChangedEntryChoice"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(string(profile)+"/"+owner+"/"+debug, func(t *testing.T) {
					fixture := strings.ReplaceAll(enumEntryEffectFixture, "PreludeChoice", owner)
					files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, fixture, owner, debug, nil, 5, false)
					files[owner+".class"] = enumEntryEffectOriginal(t, files[owner+".class"])
					run := func(dir string) {
						t.Helper()
						for _, mode := range []string{"-ea", "-da"} {
							for _, value := range []string{"7", "-3", "fail"} {
								if out, err := enumFreshOracleRun(t, java, mode, dir, "PreludeDriver", value); err != nil || string(out) != "prelude:before:allocation:failure:identity\n" {
									t.Fatal("original initialization order", err, string(out))
								}
							}
						}
					}
					run(sourceArtifactWriteFiles(t, files))
					jar := filepath.Join(t.TempDir(), "original.jar")
					if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"]}), 0600); err != nil {
						t.Fatal(err)
					}
					a, err := NewJarReconstructionArtifactFromLocal(jar, 8, profile, func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok })
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					source, err := a.ReadFile(owner + ".class")
					if err != nil || legacySourceHasFailureMarker(string(source)) {
						t.Fatal("complete source", err, string(source))
					}
					delete(files, owner+".class")
					dir := sourceArtifactWriteFiles(t, files)
					path := filepath.Join(dir, owner+".java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					compiler, args := javac, []string{"-proc:none", "--release", "8"}
					if profile == NativeJavac8 {
						compiler = native
						args = []string{"-proc:none", "-source", "8", "-target", "8"}
					}
					args = append(args, "-g:"+debug, "-cp", dir, "-d", dir, path)
					if out, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
						t.Fatal("fresh compiler", err, string(out), string(source))
					}
					path = filepath.Join(dir, owner+".class")
					fresh, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					finished, err := a.FinishClass(owner+".class", fresh)
					if err != nil {
						t.Fatal("entry effect completion", err, string(source))
					}
					if err := os.WriteFile(path, finished, 0600); err != nil {
						t.Fatal(err)
					}
					run(dir)
				})
			}
		}
	}

}
