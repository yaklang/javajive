package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Each join receives original parameter values after its selected side effect.
// The original methods contain no zero/null local initialization. Check this
// property on recompiled bytecode as well as values, raw NaN/zero bits, identity
// and effects, so a source recovery pass cannot silently fabricate defaults.
func TestAdversarialBranchDeclarationsKeepOnlyOriginalAssignments(t *testing.T) {
	const driver = `class DecisionDriver{
 static Object[] values(Class<?> c){if(c==boolean.class)return new Object[]{false,true};if(c==byte.class)return new Object[]{(byte)-128,(byte)-1,(byte)0,(byte)127};if(c==short.class)return new Object[]{(short)-32768,(short)0,(short)32767};if(c==char.class)return new Object[]{(char)0,(char)1,(char)65535};if(c==int.class)return new Object[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE};if(c==long.class)return new Object[]{Long.MIN_VALUE,-1L,0L,Long.MAX_VALUE};if(c==float.class)return new Object[]{-0.0F,0.0F,Float.intBitsToFloat(0x7fc00123),Float.NEGATIVE_INFINITY,3.5F};if(c==double.class)return new Object[]{-0.0,0.0,Double.longBitsToDouble(0x7ff8000000000123L),Double.POSITIVE_INFINITY,3.5};return new Object[]{null,new Object(),new Object()};}
 public static void main(String[]args)throws Exception{int rows=0;for(java.lang.reflect.Method m:Decision.class.getDeclaredMethods())if(m.getName().equals("pick")){Class<?> c=m.getParameterTypes()[1];for(Object a:values(c))for(Object b:values(c))for(boolean flag:new boolean[]{false,true}){Decision.trace="";Object want=flag?a:b;Object got=m.invoke(null,flag,a,b);boolean equal=c==Object.class?got==want:c==float.class?Float.floatToRawIntBits((Float)got)==Float.floatToRawIntBits((Float)want):c==double.class?Double.doubleToRawLongBits((Double)got)==Double.doubleToRawLongBits((Double)want):got.equals(want);if(!equal||!Decision.trace.equals(flag?"L":"R"))throw new AssertionError("value/effects "+c);rows++;}}System.out.println(rows+":branches:no-invented-defaults");}}
`
	source := "public class Decision{static String trace;static void left(){trace=\"L\";}static void right(){trace=\"R\";}"
	for _, typ := range []string{"boolean", "byte", "short", "char", "int", "long", "float", "double", "Object"} {
		source += fmt.Sprintf("public static %s pick(boolean flag,%s a,%s b){%s value;if(flag){left();value=a;}else{right();value=b;}return value;}", typ, typ, typ, typ)
	}
	source += "}" + driver

	modern, java := t04Tools(t)
	for _, owner := range []string{"Decision", "BranchChoice"} {
		t.Run(owner, func(t *testing.T) {
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(string(profile), func(t *testing.T) {
					compiler := modern
					args := []string{"-proc:none", "--release", "8"}
					if profile == NativeJavac8 {
						compiler = os.Getenv("JAVA8_JAVAC")
						if compiler == "" {
							t.Fatal("native javac8 required")
						}
						args = []string{"-proc:none", "-source", "8", "-target", "8"}
					}
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							files := compileLambdaParameterMetadataFixture(t, compiler, profile, strings.ReplaceAll(source, "Decision", owner), owner, debug, false)
							check := func(raw []byte) {
								t.Helper()
								obj, err := Parse(raw)
								if err != nil {
									t.Fatal(err)
								}
								methods := 0
								for _, m := range obj.Methods {
									name, _ := obj.getUtf8(m.NameIndex)
									if name != "pick" {
										continue
									}
									methods++
									for _, a := range m.Attributes {
										c, ok := a.(*CodeAttribute)
										if !ok {
											continue
										}
										d := core.NewDecompiler(c.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
										if err := d.ParseOpcode(); err != nil {
											t.Fatal(err)
										}
										ops := constructorMotionOps(d)
										for i := 1; i < len(ops); i++ {
											code := ops[i].Instr.OpCode
											if code < core.OP_ISTORE || code > core.OP_ASTORE_3 {
												continue
											}
											switch ops[i-1].Instr.OpCode {
											case core.OP_ACONST_NULL, core.OP_ICONST_0, core.OP_LCONST_0, core.OP_FCONST_0, core.OP_DCONST_0:
												t.Fatalf("invented default local write in %s at pc %d", name, ops[i].CurrentOffset)
											}
										}
									}
								}
								if methods != 9 {
									t.Fatal("method inventory", methods)
								}
							}

							check(files[owner+".class"])
							write := func(dir string, includeOwner bool) {
								for name, raw := range files {
									if !includeOwner && name == owner+".class" {
										continue
									}
									if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
										t.Fatal(err)
									}
								}
							}
							original := t.TempDir()
							write(original, true)
							want := "258:branches:no-invented-defaults\n"
							if got := t04RunJava(t, java, original, owner+"Driver"); got != want {
								t.Fatal("original independent oracle", got)
							}
							for _, policy := range []string{"archive", string(Precision), string(Compatibility)} {
								t.Run(policy, func(t *testing.T) {
									var source string
									if policy == "archive" {
										z := nativeArchive(t, files)
										defer z.Close()
										z.targetSourceVersion = 8
										z.sourceCompiler = profile
										raw, err := z.ReadFile(owner + ".class")
										if err != nil {
											t.Fatal(err)
										}
										source = string(raw)
									} else {
										r, err := DecompileWithOptions(files[owner+".class"], DecompileOptions{Mode: DecompileMode(policy), SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }})
										if err != nil || r.Status != "complete" || len(r.StubMethods) != 0 {
											t.Fatalf("decompile: %v %+v", err, r)
										}
										source = r.Source
									}
									t.Cleanup(func() {
										if t.Failed() {
											t.Logf("reconstructed owner:\n%s", source)
										}
									})
									out := t.TempDir()
									write(out, false)
									path := filepath.Join(out, owner+".java")
									if err := os.WriteFile(path, []byte(source), 0600); err != nil {
										t.Fatal(err)
									}
									command := append(append([]string(nil), args...), "-cp", out, "-d", out, path)
									if log, err := exec.Command(compiler, command...).CombinedOutput(); err != nil {
										t.Fatalf("compile: %v %s", err, log)
									}
									if got := t04RunJava(t, java, out, owner+"Driver"); got != want {
										t.Fatal("rebuilt oracle", got)
									}
									rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
									if err != nil {
										t.Fatal(err)
									}
									check(rebuilt)
									if got, want := nativeBinaryShape(t, rebuilt), nativeBinaryShape(t, files[owner+".class"]); got != want {
										t.Fatalf("declaration ABI: %s want %s", got, want)
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
