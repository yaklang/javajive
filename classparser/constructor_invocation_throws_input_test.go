package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestAdversarialConstructorInvocationGenericThrowsAllLiteralConstantsFresh(t *testing.T) {
	const source = `
class InputLiteralState {static int i;static float f;static long l;static double d;static String s,trace;static final java.io.IOException io=new java.io.IOException("same");static final RuntimeException runtime=new RuntimeException("same-runtime");}
class InputLiteralBase {<E extends Throwable>InputLiteralBase(int i,float f,long l,double d,String s,Throwable value,boolean fail)throws E{InputLiteralState.i=i;InputLiteralState.f=f;InputLiteralState.l=l;InputLiteralState.d=d;InputLiteralState.s=s;InputLiteralState.trace+="B";if(fail)throw(E)value;}}
class InputLiteralChild extends InputLiteralBase {InputLiteralChild(boolean fail,Throwable value){super(1000009,1.25f,10000000009L,1.125d,"left",value,fail);InputLiteralState.trace+="C";}}
class InputLiteralDriver {public static void main(String[]args)throws Exception{int mode=Integer.parseInt(args[0]),rows=0;if(InputLiteralChild.class.getDeclaredConstructors()[0].getExceptionTypes().length!=0)throw new AssertionError("no-throws ABI");for(Throwable value:new Throwable[]{InputLiteralState.io,InputLiteralState.runtime,null})for(boolean fail:new boolean[]{false,true}){InputLiteralState.trace="";Throwable caught=null;try{new InputLiteralChild(fail,value);}catch(Throwable ex){caught=ex;}if(InputLiteralState.i!=(mode==1?1000011:1000009)||Float.floatToRawIntBits(InputLiteralState.f)!=Float.floatToRawIntBits(mode==2?-3.5f:1.25f)||InputLiteralState.l!=(mode==3?10000000011L:10000000009L)||Double.doubleToRawLongBits(InputLiteralState.d)!=Double.doubleToRawLongBits(mode==4?-7.75d:1.125d)||!InputLiteralState.s.equals(mode==5?"right":"left"))throw new AssertionError("original CP payload");if(fail){if(value==null?!(caught instanceof NullPointerException):caught!=value)throw new AssertionError("same exception",caught);if(!InputLiteralState.trace.equals("B"))throw new AssertionError("failure order");}else if(caught!=null||!InputLiteralState.trace.equals("BC"))throw new AssertionError("normal order",caught);rows++;}System.out.println(rows+":"+mode+":same:literal:order:no-throws");}}
`
	modern, java := t04Tools(t)
	for _, compiler := range []struct {
		name, javac string
		native      bool
	}{{"modern", modern, false}, {"native8", os.Getenv("JAVA8_JAVAC"), true}} {
		if compiler.javac == "" {
			t.Skip("JAVA8_JAVAC required for native8 original evidence")
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(compiler.name+"/"+debug, func(t *testing.T) {
				original := t.TempDir()
				file := filepath.Join(original, "InputLiteralChild.java")
				if err := os.WriteFile(file, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"-proc:none", "--release", "8"}
				if compiler.native {
					args = []string{"-proc:none", "-source", "8", "-target", "8"}
				}
				args = append(args, "-g:"+debug, "-d", original, file)
				if out, err := exec.Command(compiler.javac, args...).CombinedOutput(); err != nil {
					t.Fatalf("original compile: %v\n%s", err, out)
				}
				run := func(path string, mode int) {
					t.Helper()
					out, err := exec.Command(java, "-Xverify:all", "-cp", path, "InputLiteralDriver", fmt.Sprint(mode)).CombinedOutput()
					want := fmt.Sprintf("6:%d:same:literal:order:no-throws\n", mode)
					if err != nil || string(out) != want {
						t.Fatalf("original JVM mode%d: %v %q", mode, err, out)
					}
				}
				run(original, 0)
				files := classMapFromDir(t, original)
				for mode := 1; mode <= 5; mode++ {
					t.Run(fmt.Sprint(mode), func(t *testing.T) {
						object, err := Parse(bytes.Clone(files["InputLiteralChild"]))
						if err != nil {
							t.Fatal(err)
						}
						d := NewClassObjectDumper(object)
						d.options.TargetSourceVersion = 8
						d.foldSiblingResolver = func(n string) ([]byte, bool) { raw, ok := files[n]; return bytes.Clone(raw), ok }
						d.FuncCtx = &class_context.ClassContext{ClassName: object.GetClassName(), SupperClassName: object.GetSupperClassName(), FunctionName: "<init>", CurrentMethodDesc: "(ZLjava/lang/Throwable;)V", InvocationMetadata: d.buildInvocationMetadata()}
						var method *MemberInfo
						var code *CodeAttribute
						for _, m := range object.Methods {
							n, _ := object.getUtf8(m.NameIndex)
							if n == "<init>" {
								method = m
								for _, a := range m.Attributes {
									if c, ok := a.(*CodeAttribute); ok {
										code = c
									}
								}
							}
						}
						if method == nil || code == nil {
							t.Fatal("original constructor missing")
						}
						decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(d.ConstantPool, i) })
						if err := decoder.ParseOpcode(); err != nil {
							t.Fatal(err)
						}
						pc := -1
						var member *values.JavaClassMember
						for _, op := range decoder.Opcodes() {
							if m := constructorMotionMember(object, op, core.OP_INVOKESPECIAL); m != nil && m.Name == "InputLiteralBase" {
								pc = int(op.CurrentOffset)
								member = m
							}
						}
						if member == nil {
							t.Fatal("original call missing")
						}
						flag := constructorThrowsTestParameter(1, types.NewJavaPrimer(types.JavaBoolean), "fail")
						value := constructorThrowsTestParameter(2, types.NewJavaClass("java.lang.Throwable"), "value")
						call := &values.FunctionCallExpression{ClassName: member.Name, FunctionName: "<init>", Descriptor: member.Description, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: pc, Object: constructorThrowsTestReceiver(object.GetClassName()), Arguments: []values.JavaValue{values.NewJavaLiteral(1000009, types.NewJavaPrimer(types.JavaInteger)), values.NewJavaLiteral(float32(1.25), types.NewJavaPrimer(types.JavaFloat)), values.NewJavaLiteral(int64(10000000009), types.NewJavaPrimer(types.JavaLong)), values.NewJavaLiteral(float64(1.125), types.NewJavaPrimer(types.JavaDouble)), values.NewJavaLiteral("left", types.NewJavaPrimer(types.JavaString)), value, flag}}
						plan := &constructorSourceBoundary{pc: pc, delegate: call, params: []values.JavaValue{flag, value}}
						receipt, known := d.proveConstructorInvocationThrows(code, method, member, pc, plan)
						if !known || !receipt.allUnchecked || !receipt.fresh(d.FuncCtx) {
							t.Fatal("canonical original literal operands must first prove", known)
						}
						plan.throwsReceipt = receipt
						if !receipt.bindSourceParameters(plan, plan.params, d.FuncCtx) {
							t.Fatal("canonical original header")
						}
						beforeCode := bytes.Clone(code.Code)
						beforeDecl := constructorInvocationThrowsDeclarations(t, object.Bytes())
						changed := false
						for _, constant := range object.ConstantPool {
							switch c := constant.(type) {
							case *ConstantIntegerInfo:
								if mode == 1 && c.Value == 1000009 {
									c.Value = 1000011
									changed = true
								}
							case *ConstantFloatInfo:
								if mode == 2 && c.Value == 1.25 {
									c.Value = -3.5
									changed = true
								}
							case *ConstantLongInfo:
								if mode == 3 && c.Value == 10000000009 {
									c.Value = 10000000011
									changed = true
								}
							case *ConstantDoubleInfo:
								if mode == 4 && c.Value == 1.125 {
									c.Value = -7.75
									changed = true
								}
							case *ConstantStringInfo:
								if mode == 5 {
									u, ok := object.ConstantPool[c.StringIndex-1].(*ConstantUtf8Info)
									if ok && u.Value == "left" {
										u.SetString("right")
										changed = true
									}
								}
							}
						}
						if !changed || !bytes.Equal(beforeCode, code.Code) || beforeDecl != constructorInvocationThrowsDeclarations(t, object.Bytes()) {
							t.Fatal("mutation must change only an executable constant payload")
						}
						mutant := t.TempDir()
						for name, raw := range files {
							if name == object.GetClassName() {
								raw = object.Bytes()
							}
							if err := os.WriteFile(filepath.Join(mutant, name+".class"), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
						run(mutant, mode)
						if receipt.fresh(d.FuncCtx) {
							t.Fatal("CP-only payload retained old typed source binding")
						}
						if source, err := plan.renderDelegation(d.FuncCtx); err == nil {
							t.Fatalf("stale rendering succeeded: %s", source)
						}
					})
				}
			})
		}
	}
}

func TestAdversarialConstructorInvocationGenericThrowsCanonicalLiteralDomain(t *testing.T) {
	for _, row := range []struct {
		name, descriptor string
		data             any
		want             bool
	}{{"null", "Ljava/lang/Object;", "null", true}, {"nil payload", "Ljava/lang/Object;", nil, false}, {"nominal reference text", "Ljava/lang/Throwable;", "value", false}, {"integer", "I", 1, true}, {"boolean int", "Z", 1, true}, {"boolean wide", "Z", 2, false}, {"integer overflow", "I", int64(1) << 40, false}, {"wide", "J", int64(1), true}, {"wrong wide payload", "J", 1, false}, {"float", "F", float32(1), true}, {"double", "D", float64(1), true}, {"string", "Ljava/lang/String;", "tag", true}, {"narrow byte literal", "B", 1, false}} {
		t.Run(row.name, func(t *testing.T) {
			typ, err := types.ParseDescriptor(row.descriptor)
			if err != nil {
				t.Fatal(err)
			}
			literal := values.NewJavaLiteral(row.data, typ)
			if got := constructorThrowsCanonicalLiteral(literal, row.descriptor); got != row.want {
				t.Fatalf("canonical domain %v want%v", got, row.want)
			}
		})
	}
}

func TestAdversarialConstructorInvocationGenericThrowsConditionalNeedsSelectedEdgeProof(t *testing.T) {
	source := strings.Replace(constructorInvocationThrowsFixture, "boolean pick,java.io.IOException left,java.io.IOException right)throws Throwable{super(fail,pick?left:right);}", "boolean pick,java.io.IOException left,java.io.IOException right)throws java.io.IOException{super(fail,pick?left:right);}", 1)
	source = strings.Replace(source, "boolean checked=kind==2||kind==3||kind==4||kind==5||kind==8||kind==9;", "boolean checked=kind==2||kind==3||kind==4||kind==5||kind==8||kind==9||kind==12;", 1)
	source = strings.Replace(source, "boolean broad=kind==11||kind==12||kind==13;", "boolean broad=kind==11||kind==13;", 1)
	_, java := t04Tools(t)
	for _, native := range []bool{false, true} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("%v/%s", native, debug), func(t *testing.T) {
				var files map[string][]byte
				if native {
					files = nativePrivateEnumCompile(t, source, "CtorThrowLoose", debug)
				} else {
					files = nativeCompileDebugClasses(t, source, debug)
				}
				original := t.TempDir()
				for name, raw := range files {
					if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if got := t04RunJava(t, java, original, "CtorThrowDriver"); got != "44:generic-ctor:same:order:overload:abi\n" {
					t.Fatal("independent original conditional oracle", got)
				}
				result, err := DecompileWithOptions(bytes.Clone(files["CtorThrowConditional.class"]), DecompileOptions{Mode: Precision, TargetSourceVersion: 8, Resolve: func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return bytes.Clone(raw), ok }})
				if err != nil || !strings.Contains(result.Source, DecompileStubMarker) || !strings.Contains(result.Source, "checked constructor delegation requires original declared throws") {
					t.Fatalf("a source phi without a selected-edge receipt must not obtain narrower checked throws: %v\n%s", err, result.Source)
				}
			})
		}
	}
}
