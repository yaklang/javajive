package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const enumInitializerTailFixture = `class TailEffects {static String trace="";}
public enum TailEnum {
 A(3),B(7),C(11); final int code; static final int[] map; static long digest;
 TailEnum(int n){code=n;TailEffects.trace+="C"+n+";";}
 static {map=new int[3];long wide=Long.MIN_VALUE;for(TailEnum v:values()){
 int result;switch(v.code){case 3:result=17;break;case 7:result=-5;break;default:result=23;}
 map[v.ordinal()]=result;wide=wide*31+result;TailEffects.trace+="S"+result+";";
 if(Integer.getInteger("tail.fail",0)==v.code)throw new IllegalStateException("stop:"+v.code);
 }digest=wide;}
 public int read(){return map[ordinal()];}
}
class TailDriver {public static void main(String[]args)throws Exception {
 int failure=Integer.parseInt(args[0]);System.setProperty("tail.fail",args[0]);System.setProperty("tail.branch",args.length>1?args[1]:"false");
 String expected="C3;C7;C11;";int[]codes={3,7,11},answers={17,-5,23};long digest=Long.MIN_VALUE;
 for(int i=0;i<3;i++){expected+="S"+answers[i]+";";digest=digest*31+answers[i];if(codes[i]==failure)break;}
 if(failure!=0){try{Class.forName("TailEnum");throw new AssertionError("missing failure");}
 catch(ExceptionInInitializerError e){if(!(e.getCause() instanceof IllegalStateException)||!e.getCause().getMessage().equals("stop:"+failure))throw new AssertionError(e);}
 try{Class.forName("TailEnum");throw new AssertionError("erroneous class reused");}catch(NoClassDefFoundError e){}
 }else{TailEnum[]v=TailEnum.values();if(v.length!=3||TailEnum.digest!=digest)throw new AssertionError("digest");
 for(int i=0;i<3;i++)if(v[i].read()!=answers[i]||v[i].ordinal()!=i||TailEnum.valueOf(v[i].name())!=v[i])throw new AssertionError("identity/order");
 v[0]=null;if(TailEnum.values()[0]==null)throw new AssertionError("values alias");
 if(TailEnum.class.getDeclaredMethods().length!=3)throw new AssertionError("helper leaked");}
 if(!TailEffects.trace.equals(expected))throw new AssertionError(TailEffects.trace+" != "+expected);
 System.out.println("tail:dispatch:effects:overflow:failure:identity");}}
`

func TestAdversarialEnumInitializerTailKeepsControlFlowAndEffects(t *testing.T) {
	compiler, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, kind := range []string{"lookup", "table", "uninitialized", "generic locals"} {
			t.Run(debug+"/"+kind, func(t *testing.T) {
				fixture := enumInitializerTailFixture
				if kind == "table" {
					fixture = strings.Replace(fixture, "switch(v.code){case 3:", "switch(v.ordinal()){case 0:", 1)
					fixture = strings.Replace(fixture, "case 7:", "case 1:", 1)
				}
				if kind == "uninitialized" {
					fixture = strings.Replace(fixture, "}digest=wide;}", "}digest=new TailBox(Boolean.getBoolean(\"tail.branch\") ? wide+17 : wide).value;}", 1)
					fixture = strings.Replace(fixture, "if(failure!=0)", "if(Boolean.getBoolean(\"tail.branch\"))digest+=17;if(failure!=0)", 1)
					fixture += "class TailBox {final long value;TailBox(long value){this.value=value;}}"
				}
				if kind == "generic locals" {
					fixture = strings.Replace(fixture, "for(TailEnum v:values())", "java.util.List<TailEnum> all=java.util.Arrays.asList(values());for(TailEnum v:all)", 1)
				}
				files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "TailEnum", debug, false)
				run := func(dir string) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						for _, failure := range []string{"0", "3", "7", "11"} {
							branches := []string{"false"}
							if kind == "uninitialized" {
								branches = append(branches, "true")
							}
							for _, branch := range branches {
								out, err := enumFreshOracleRun(t, java, mode, dir, "TailDriver", failure, branch)
								if err != nil || string(out) != "tail:dispatch:effects:overflow:failure:identity\n" {
									t.Fatal("unchanged original driver", err, string(out))
								}
							}
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files))
				owned := map[string][]byte{"TailEnum.class": files["TailEnum.class"]}
				jar := filepath.Join(t.TempDir(), "input.jar")
				if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(n string) ([]byte, bool) { b, k := files[n+".class"]; return b, k })
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				source, err := a.ReadFile("TailEnum.class")
				if err != nil || legacySourceHasFailureMarker(string(source)) {
					t.Fatal("complete source", err, string(source))
				}
				external := map[string][]byte{}
				for n, b := range files {
					if n != "TailEnum.class" {
						external[n] = b
					}
				}
				out := sourceArtifactWriteFiles(t, external)
				path := filepath.Join(out, "TailEnum.java")
				if err := os.WriteFile(path, source, 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(compiler, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
					t.Fatal("fresh source compile", err, string(log), string(source))
				}
				path = filepath.Join(out, "TailEnum.class")
				fresh, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				saved := bytes.Clone(fresh)
				finished, err := a.FinishClass("TailEnum.class", fresh)
				if err != nil {
					for label, raw := range map[string][]byte{"original": files["TailEnum.class"], "fresh": fresh} {
						obj := mustParseLowering(t, raw)
						fields := map[sourceFieldIdentity]*MemberInfo{}
						for _, f := range obj.Fields {
							n, _ := sourceBridgeUTF8(obj, f.NameIndex)
							d, _ := sourceBridgeUTF8(obj, f.DescriptorIndex)
							fields[sourceFieldIdentity{n, d}] = f
						}
						for _, m := range obj.Methods {
							n, _ := sourceBridgeUTF8(obj, m.NameIndex)
							d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
							t.Log(label, n, d, m.AccessFlags)
							if n != "<clinit>" {
								continue
							}
							ops, ok := nativeEnumMethodOps(obj, m, nil)
							code := constructorLoweringCode(m)
							boundary, bk := enumArrayConstantBoundary(obj, m, ops, []string{"A", "B", "C"}, fields, nil, nil)
							t.Log(label, "ops", ok, len(ops), "spans", enumArrayInstructionSpans(code.Code, ops), "boundary", boundary, bk, "locals", code.MaxLocals)
							if bk {
								tail := boundary + 2
								if label == "original" {
									tail = boundary + 15
								}
								t.Log(label, "suffix", tail, enumArraySuffixClosed(code, ops, int(ops[tail].CurrentOffset)))
							}
						}
					}
					t.Fatal("initializer and method universe", err)
				}
				if !bytes.Equal(saved, fresh) {
					t.Fatal("input mutation")
				}
				if again, err := a.FinishClass("TailEnum.class", finished); err != nil || !bytes.Equal(again, finished) {
					t.Fatal("idempotence", err)
				}
				if err := os.WriteFile(path, finished, 0600); err != nil {
					t.Fatal(err)
				}
				run(out)
			})
		}
	}
}
