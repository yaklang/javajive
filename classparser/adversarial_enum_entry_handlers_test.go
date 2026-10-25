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

const enumEntryHandlersFixture = `class PreludeProbe {
 static String trace="";
 static int choose(){trace+="P;";String text=System.getProperty("prelude.mode");if(text.equals("fail"))throw new IllegalStateException("prelude");if(text.equals("secure"))throw new SecurityException("private");return Integer.parseInt(text);}
 static void record(int ordinal,int mode){trace+="C"+ordinal+":"+mode+";";}
}
public enum PreludeChoice {
 A,B;static int mode;
 PreludeChoice(){PreludeProbe.record(ordinal(),read());}
 static int read(){return mode;}
 static {try{mode=PreludeProbe.choose();}catch(IllegalArgumentException unused){mode=-7;}catch(SecurityException unused){mode=11;}}
}
class PreludeDriver {public static void main(String[] args)throws Exception {
 System.setProperty("prelude.mode",args[0]);String expected="P;";
 if(args[0].equals("fail")){
  try{Class.forName("PreludeChoice");throw new AssertionError("missing prelude failure");}
  catch(ExceptionInInitializerError error){if(!(error.getCause() instanceof IllegalStateException)||!error.getCause().getMessage().equals("prelude"))throw new AssertionError(error);}
  try{Class.forName("PreludeChoice");throw new AssertionError("erroneous class reused");}catch(NoClassDefFoundError error){}
 }else{
  int mode=args[0].equals("bad")?-7:args[0].equals("secure")?11:Integer.parseInt(args[0]);PreludeChoice[] values=PreludeChoice.values();
  if(values.length!=2||PreludeChoice.read()!=mode)throw new AssertionError("state");
  for(int i=0;i<2;i++){expected+="C"+i+":"+mode+";";if(values[i].ordinal()!=i||PreludeChoice.valueOf(values[i].name())!=values[i])throw new AssertionError("identity");}
  values[0]=null;if(PreludeChoice.values()[0]==null)throw new AssertionError("array alias");
 }
 if(!expected.equals(PreludeProbe.trace))throw new AssertionError(PreludeProbe.trace+" != "+expected);
 System.out.println("prelude:before:allocation:failure:identity");}}
`

// Author the entry trampoline directly from the independently compiled static
// block. Every handler and internal branch remains within that block. The
// original JVM runs this transformed class before any decompiler is involved.
func enumEntryHandlersOriginal(t *testing.T, raw []byte) []byte {
	t.Helper()
	obj := mustParseLowering(t, bytes.Clone(raw))
	for _, m := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if name != "<clinit>" {
			continue
		}
		c := constructorLoweringCode(m)
		if obj.MajorVersion != 49 || c == nil || len(c.ExceptionTable) != 2 {
			t.Fatal("authored handler fixture")
		}
		old := bytes.Clone(c.Code)
		cut := int(c.ExceptionTable[0].StartPc)
		if cut < 1 || old[len(old)-1] != 0xb1 {
			t.Fatal("authored entry cut")
		}
		b := make([]byte, len(old)+6)
		b[0] = 0xa7
		binary.BigEndian.PutUint16(b[1:], uint16(cut+4))
		copy(b[3:], old[:cut])
		b[cut+3] = 0xb1
		copy(b[cut+4:], old[cut:len(old)-1])
		b[len(b)-3] = 0xa7
		binary.BigEndian.PutUint16(b[len(b)-2:], uint16(int16(3-(len(b)-3))))
		for _, h := range c.ExceptionTable {
			if int(h.StartPc) < cut || int(h.HandlerPc) < cut || int(h.EndPc) >= len(old) {
				t.Fatal("crossing authored handler")
			}
			h.StartPc += 4
			h.EndPc += 4
			h.HandlerPc += 4
			// Discarding the exception and storing it in a never-read local have
			// the same effect. Keep the spare local capacity, but remove its debug row.
			if b[h.HandlerPc] != 0x4b {
				t.Fatal("expected one-byte unused exception store")
			}
			b[h.HandlerPc] = 0x57
		}
		c.Code = b
		kept := c.Attributes[:0]
		for _, a := range c.Attributes {
			switch table := a.(type) {
			case *LineNumberTableAttribute:
				for _, r := range table.LineNumberTable {
					if int(r.StartPc) < cut {
						r.StartPc += 3
					} else {
						r.StartPc += 4
					}
				}
				kept = append(kept, a)
			case *UnparsedAttribute:
				if table.Name != "LocalVariableTable" {
					t.Fatalf("unmodeled original PC attribute %s", table.Name)
				}
				// The test explicitly removed all corresponding ASTORE instructions.
				c.AttrLen -= 6 + table.Length
			default:
				t.Fatalf("unmodeled original PC attribute %T", a)
			}
		}
		c.Attributes = kept
		c.AttrLen += 6
		return obj.Bytes()
	}
	t.Fatal("missing authored initializer")
	return nil
}
func TestAdversarialEnumEntryHandlersPrecedeConstantAllocation(t *testing.T) {
	enumEntryHandlersRoundTrip(t, false, false)
}

func TestAdversarialEnumEntryHandlerPhiKeepsFinalFieldWrite(t *testing.T) {
	enumEntryHandlersRoundTrip(t, true, false)
}

func TestAdversarialEnumEntryHandlersKeepConstantArguments(t *testing.T) {
	enumEntryHandlersRoundTrip(t, true, true)
}

func enumEntryHandlersRoundTrip(t *testing.T, finalField, extraArguments bool) {
	t.Helper()
	javac, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		for _, owner := range []string{"PreludeChoice", "ChangedEntryChoice"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(string(profile)+"/"+owner+"/"+debug, func(t *testing.T) {
					fixture := strings.ReplaceAll(enumEntryHandlersFixture, "PreludeChoice", owner)
					if extraArguments {
						fixture = strings.ReplaceAll(fixture, "A,B;static int mode;", "A(true,false),B(false,true);final int mask;static int mode;")
						fixture = strings.ReplaceAll(fixture, owner+"(){PreludeProbe.record(ordinal(),read());}", owner+"(boolean a,boolean b){mask=(a?1:0)+(b?2:0);PreludeProbe.record(ordinal(),read());}")
						fixture = strings.ReplaceAll(fixture, "values[i].ordinal()!=i", "values[i].mask!=i+1||values[i].ordinal()!=i")
					}
					files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, fixture, owner, debug, nil, 5, false)
					files[owner+".class"] = enumEntryHandlersOriginal(t, files[owner+".class"])
					if finalField {
						// This JVM-legal final field has one write on each
						// terminal path of <clinit>. Java definite assignment
						// needs a joined local before its source assignment.
						object := mustParseLowering(t, bytes.Clone(files[owner+".class"]))
						found := false
						for _, f := range object.Fields {
							n, _ := sourceBridgeUTF8(object, f.NameIndex)
							if n == "mode" {
								f.AccessFlags |= 0x10
								found = true
							}
						}
						if !found {
							t.Fatal("missing authored final field")
						}
						files[owner+".class"] = object.Bytes()
					}
					run := func(dir string) {
						t.Helper()
						for _, mode := range []string{"-ea", "-da"} {
							for _, value := range []string{"7", "-3", "bad", "secure", "fail"} {
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
						t.Logf("receipts pending=%#v active=%#v", a.source.jar.sourceConstructorLowering.enumPending, a.source.jar.sourceConstructorLowering.enumActive)
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
