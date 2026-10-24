package javaclassparser

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

const reviewGenericCastSource = `
class ReviewGenericCastState {
 static String trace="";
 static final java.io.IOException io=new java.io.IOException("original-io");
 static final RuntimeException runtime=new RuntimeException("original-runtime");
}
class ReviewGenericCastBase {
 <E extends Throwable>ReviewGenericCastBase(boolean fail,E value)throws E {
  ReviewGenericCastState.trace+="B";if(fail)throw value;
 }
}
class ReviewGenericCastChild extends ReviewGenericCastBase {
 ReviewGenericCastChild(boolean fail,Object value){super(fail,(RuntimeException)value);ReviewGenericCastState.trace+="C";}
 static Class<?> checkedTarget(){return java.io.IOException.class;}
}
class ReviewGenericCastDriver {
 public static void main(String[] args)throws Exception {
  boolean checked=args[0].equals("I");int rows=0;
  if(ReviewGenericCastChild.class.getDeclaredConstructors()[0].getExceptionTypes().length!=0)throw new AssertionError("original no-throws ABI");
  for(Object value:new Object[]{ReviewGenericCastState.io,ReviewGenericCastState.runtime,new Object(),null})for(boolean fail:new boolean[]{false,true}){
   ReviewGenericCastState.trace="";Throwable found=null;
   try{new ReviewGenericCastChild(fail,value);}catch(Throwable caught){found=caught;}
   boolean pass=value==null||(checked?value instanceof java.io.IOException:value instanceof RuntimeException);
   if(!pass){if(!(found instanceof ClassCastException)||!ReviewGenericCastState.trace.equals(""))throw new AssertionError("cast/order",found);}
   else if(fail){if(value==null?!(found instanceof NullPointerException):found!=value)throw new AssertionError("original throw identity",found);if(!ReviewGenericCastState.trace.equals("B"))throw new AssertionError("throw order");}
   else if(found!=null||!ReviewGenericCastState.trace.equals("BC"))throw new AssertionError("success/order",found);
   rows++;
  }
  System.out.println(rows+":"+args[0]+":same-identity:cast-before-base:no-throws-ABI");
 }
}`

// This is an authored original-first stale-receipt test. Its only classfile
// mutation changes a CHECKCAST class constant; the entire Code byte slice,
// method Signature, Exceptions, flags and selected constructor are unchanged.
func TestAdversarialConstructorInvocationGenericThrowsRejectsStaleCheckcastConstant(t *testing.T) {
	evidence := os.Getenv("JDEC_REVIEW_EVIDENCE")
	if evidence == "" {
		evidence = t.TempDir()
	}
	modern, java := t04Tools(t)
	for _, compiler := range []struct {
		name, javac string
		native      bool
	}{
		{"modern", modern, false}, {"native8", os.Getenv("JAVA8_JAVAC"), true},
	} {
		if compiler.javac == "" {
			continue
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(compiler.name+"/"+debug, func(t *testing.T) {
				dir := filepath.Join(evidence, compiler.name, debug)
				original, mutant := filepath.Join(dir, "original"), filepath.Join(dir, "changed-cp")
				for _, d := range []string{original, mutant} {
					if err := os.MkdirAll(d, 0700); err != nil {
						t.Fatal(err)
					}
				}
				file := filepath.Join(original, "ReviewGenericCastChild.java")
				if err := os.WriteFile(file, []byte(reviewGenericCastSource), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"-proc:none", "--release", "8"}
				if compiler.native {
					args = []string{"-proc:none", "-source", "8", "-target", "8"}
				}
				args = append(args, "-g:"+debug, "-d", original, file)
				out, err := exec.Command(compiler.javac, args...).CombinedOutput()
				os.WriteFile(filepath.Join(dir, "compile.log"), out, 0600)
				if err != nil {
					t.Fatalf("authored original compile: %v\n%s", err, out)
				}
				run := func(cp, mode string) {
					out, err := exec.Command(java, "-Xverify:all", "-cp", cp, "ReviewGenericCastDriver", mode).CombinedOutput()
					os.WriteFile(filepath.Join(cp, "original-jvm.log"), out, 0600)
					if err != nil {
						t.Fatalf("independent authored JVM %s: %v\n%s", mode, err, out)
					}
					t.Log(string(out))
				}
				run(original, "R")
				files := classMapFromDir(t, original)
				raw := bytes.Clone(files["ReviewGenericCastChild"])
				object, err := Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				d := NewClassObjectDumper(object)
				d.options.TargetSourceVersion = 8
				d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n]; return bytes.Clone(b), ok }
				d.FuncCtx = &class_context.ClassContext{ClassName: object.GetClassName(), SupperClassName: object.GetSupperClassName(), FunctionName: "<init>", CurrentMethodDesc: "(ZLjava/lang/Object;)V", InvocationMetadata: d.buildInvocationMetadata()}
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
				beforeCode := bytes.Clone(code.Code)
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(d.ConstantPool, i) })
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				pc, castPC, castIndex := -1, -1, 0
				var member *values.JavaClassMember
				for _, op := range decoder.Opcodes() {
					if op == nil || op.Instr == nil {
						continue
					}
					if op.Instr.OpCode == core.OP_CHECKCAST {
						castPC = int(op.CurrentOffset)
						castIndex = int(core.Convert2bytesToInt(op.Data[:2]))
					}
					if m := constructorMotionMember(object, op, core.OP_INVOKESPECIAL); m != nil && m.Name == "ReviewGenericCastBase" {
						pc = int(op.CurrentOffset)
						member = m
					}
				}
				if pc < 0 || castPC < 0 || castIndex == 0 {
					t.Fatal("authored bytecode shape changed")
				}
				fail := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
				fail.IsParam = true
				fail.MarkOriginalParameter(1)
				value := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
				value.IsParam = true
				value.MarkOriginalParameter(2)
				receiver := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass(object.GetClassName()))
				receiver.IsThis, receiver.IsParam = true, true
				receiver.MarkOriginalParameter(0)
				receiver.MarkOriginalReceiver()
				d.FuncCtx.LocalNames = map[*coreutils.VariableId]string{fail.Id: "fail", value.Id: "value", receiver.Id: "this"}
				cast := values.NewOriginalCheckCast(value, types.NewJavaClass("java.lang.RuntimeException"), castPC)
				ft, err := types.ParseMethodDescriptor(member.Description)
				if err != nil {
					t.Fatal(err)
				}
				call := &values.FunctionCallExpression{ClassName: member.Name, FunctionName: "<init>", Descriptor: member.Description, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: pc, Object: receiver, FuncType: ft.FunctionType(), Arguments: []values.JavaValue{fail, cast}}
				plan := &constructorSourceBoundary{pc: pc, delegate: call, params: []values.JavaValue{fail, value}}
				receipt, known := d.proveConstructorInvocationThrows(code, method, member, pc, plan)
				if !known || !receipt.allUnchecked {
					t.Fatal("authored original receipt not proved", known)
				}
				plan.throwsReceipt = receipt
				if !receipt.bindSourceParameters(plan, plan.params, d.FuncCtx) {
					t.Fatal("original structured source parameter binding")
				}
				originalSource, originalErr := plan.renderDelegation(d.FuncCtx)
				if originalErr != nil || !receipt.claimHeader(plan, call, pc, originalSource, d.FuncCtx) || !receipt.confirmEmission(plan, d.FuncCtx, "\n"+originalSource+";\n", "") || !receipt.consumed(d.FuncCtx) {
					t.Fatal("original receipt must fully consume before the independent stale experiment", originalErr)
				}
				receipt, known = d.proveConstructorInvocationThrows(code, method, member, pc, plan)
				if !known {
					t.Fatal("fresh independent stale experiment receipt")
				}
				plan.throwsReceipt = receipt
				if !receipt.bindSourceParameters(plan, plan.params, d.FuncCtx) {
					t.Fatal("stale experiment original structured parameter binding")
				}
				if !receipt.fresh(d.FuncCtx) {
					t.Fatal("original receipt not fresh")
				}
				target := object.ConstantPool[castIndex-1].(*ConstantClassInfo)
				if name, _ := object.getUtf8(target.NameIndex); name != "java/lang/RuntimeException" {
					t.Fatal("wrong original CHECKCAST", name)
				}
				var ioIndex uint16
				for _, constant := range object.ConstantPool {
					if c, ok := constant.(*ConstantClassInfo); ok && c != nil {
						if n, _ := object.getUtf8(c.NameIndex); n == "java/io/IOException" {
							ioIndex = c.NameIndex
						}
					}
				}
				if ioIndex == 0 {
					t.Fatal("authored target class constant missing")
				}
				target.NameIndex = ioIndex
				if !bytes.Equal(beforeCode, code.Code) {
					t.Fatal("CP-only mutation changed Code")
				}
				for n, b := range files {
					if n == "ReviewGenericCastChild" {
						b = object.Bytes()
					}
					if err := os.WriteFile(filepath.Join(mutant, n+".class"), b, 0600); err != nil {
						t.Fatal(err)
					}
				}
				run(mutant, "I")
				fresh := receipt.fresh(d.FuncCtx)
				source, renderErr := plan.renderDelegation(d.FuncCtx)
				claimed := renderErr == nil && receipt.claimHeader(plan, call, pc, source, d.FuncCtx)
				emitted := claimed && receipt.confirmEmission(plan, d.FuncCtx, "\n"+source+";\n", "")
				record := map[string]any{"code_unchanged": bytes.Equal(beforeCode, code.Code), "cast_pc": castPC, "cast_constant_index": castIndex, "original_cast": "java/lang/RuntimeException", "changed_cast": "java/io/IOException", "receipt_fresh": fresh, "rendered_source": source, "render_error": renderErr, "header_claimed": claimed, "emission_confirmed": emitted, "consumed": receipt.consumed(d.FuncCtx)}
				encoded, _ := json.MarshalIndent(record, "", "  ")
				os.WriteFile(filepath.Join(dir, "receipt-observation.json"), append(encoded, '\n'), 0600)
				if fresh || renderErr == nil || claimed || emitted {
					t.Errorf("stale CHECKCAST semantics accepted by typed receipt: %s", encoded)
				}
			})
		}
	}
}
