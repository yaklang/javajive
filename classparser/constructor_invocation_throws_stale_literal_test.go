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

const reviewGenericLiteralSource = `
class ReviewGenericLiteralState {
 static String trace="";static int observed;
 static final java.io.IOException io=new java.io.IOException("original-io");
 static final RuntimeException runtime=new RuntimeException("original-runtime");
 @SuppressWarnings("unchecked")static<E extends Throwable>void effect(boolean fail,Throwable value)throws E{if(fail)throw(E)value;}
}
class ReviewGenericLiteralBase {
 <E extends Throwable>ReviewGenericLiteralBase(int tag,Throwable value,boolean fail)throws E {
  ReviewGenericLiteralState.trace+="B";ReviewGenericLiteralState.observed=tag;
  ReviewGenericLiteralState.<E>effect(fail,value);ReviewGenericLiteralState.trace+="E";
 }
}
class ReviewGenericLiteralChild extends ReviewGenericLiteralBase {
 ReviewGenericLiteralChild(boolean fail,Throwable value){super(1000009,value,fail);ReviewGenericLiteralState.trace+="C";}
}
class ReviewGenericLiteralDriver {
 public static void main(String[] args)throws Exception {
  int expected=Integer.parseInt(args[0]),rows=0;
  if(ReviewGenericLiteralChild.class.getDeclaredConstructors()[0].getExceptionTypes().length!=0)throw new AssertionError("original no-throws ABI");
  for(Throwable value:new Throwable[]{ReviewGenericLiteralState.io,ReviewGenericLiteralState.runtime,null})for(boolean fail:new boolean[]{false,true}){
   ReviewGenericLiteralState.trace="";ReviewGenericLiteralState.observed=0;Throwable found=null;
   try{new ReviewGenericLiteralChild(fail,value);}catch(Throwable caught){found=caught;}
   if(ReviewGenericLiteralState.observed!=expected)throw new AssertionError("argument value identity");
   if(fail){if(value==null?!(found instanceof NullPointerException):found!=value)throw new AssertionError("original throw identity",found);if(!ReviewGenericLiteralState.trace.equals("B"))throw new AssertionError("throw order");}
   else if(found!=null||!ReviewGenericLiteralState.trace.equals("BEC"))throw new AssertionError("success/order",found);
   rows++;
  }
  System.out.println(rows+":"+expected+":same-identity:constant-before-base:no-throws-ABI");
 }
}`

func TestAdversarialConstructorInvocationGenericThrowsRejectsStaleLDCConstant(t *testing.T) {
	evidence := os.Getenv("JDEC_REVIEW_EVIDENCE")
	if evidence == "" {
		evidence = t.TempDir()
	}
	modern, java := t04Tools(t)
	for _, compiler := range []struct {
		name, javac string
		native      bool
	}{{"modern", modern, false}, {"native8", os.Getenv("JAVA8_JAVAC"), true}} {
		if compiler.javac == "" {
			continue
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(compiler.name+"/"+debug, func(t *testing.T) {
				dir := filepath.Join(evidence, "ldc", compiler.name, debug)
				original, mutant := filepath.Join(dir, "original"), filepath.Join(dir, "changed-cp")
				for _, d := range []string{original, mutant} {
					if err := os.MkdirAll(d, 0700); err != nil {
						t.Fatal(err)
					}
				}
				file := filepath.Join(original, "ReviewGenericLiteralChild.java")
				if err := os.WriteFile(file, []byte(reviewGenericLiteralSource), 0600); err != nil {
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
					out, err := exec.Command(java, "-Xverify:all", "-cp", cp, "ReviewGenericLiteralDriver", mode).CombinedOutput()
					os.WriteFile(filepath.Join(cp, "original-jvm.log"), out, 0600)
					if err != nil {
						t.Fatalf("authored original JVM: %v\n%s", err, out)
					}
					t.Log(string(out))
				}
				run(original, "1000009")
				files := classMapFromDir(t, original)
				object, err := Parse(bytes.Clone(files["ReviewGenericLiteralChild"]))
				if err != nil {
					t.Fatal(err)
				}
				d := NewClassObjectDumper(object)
				d.options.TargetSourceVersion = 8
				d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n]; return bytes.Clone(b), ok }
				d.FuncCtx = &class_context.ClassContext{ClassName: object.GetClassName(), SupperClassName: object.GetSupperClassName(), FunctionName: "<init>", CurrentMethodDesc: "(ZLjava/lang/Throwable;)V", InvocationMetadata: d.buildInvocationMetadata()}
				var method *MemberInfo
				var code *CodeAttribute
				for _, m := range object.Methods {
					if n, _ := object.getUtf8(m.NameIndex); n == "<init>" {
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
				pc, ldcPC, index := -1, -1, 0
				var member *values.JavaClassMember
				for _, op := range decoder.Opcodes() {
					if op == nil || op.Instr == nil {
						continue
					}
					if op.Instr.OpCode == core.OP_LDC {
						candidate := int(op.Data[0])
						if n, ok := object.ConstantPool[candidate-1].(*ConstantIntegerInfo); ok && n.Value == 1000009 {
							ldcPC = int(op.CurrentOffset)
							index = candidate
						}
					}
					if op.Instr.OpCode == core.OP_LDC_W {
						candidate := int(core.Convert2bytesToInt(op.Data[:2]))
						if n, ok := object.ConstantPool[candidate-1].(*ConstantIntegerInfo); ok && n.Value == 1000009 {
							ldcPC = int(op.CurrentOffset)
							index = candidate
						}
					}
					if m := constructorMotionMember(object, op, core.OP_INVOKESPECIAL); m != nil && m.Name == "ReviewGenericLiteralBase" {
						pc = int(op.CurrentOffset)
						member = m
					}
				}
				if pc < 0 || ldcPC < 0 || index == 0 {
					t.Fatal("original LDC shape missing")
				}
				literal := object.ConstantPool[index-1].(*ConstantIntegerInfo)
				if literal.Value != 1000009 {
					t.Fatal("original literal changed", literal.Value)
				}
				fail := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
				fail.IsParam = true
				fail.MarkOriginalParameter(1)
				value := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
				value.IsParam = true
				value.MarkOriginalParameter(2)
				receiver := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass(object.GetClassName()))
				receiver.IsThis, receiver.IsParam = true, true
				receiver.MarkOriginalParameter(0)
				receiver.MarkOriginalReceiver()
				d.FuncCtx.LocalNames = map[*coreutils.VariableId]string{fail.Id: "fail", value.Id: "value", receiver.Id: "this"}
				ft, err := types.ParseMethodDescriptor(member.Description)
				if err != nil {
					t.Fatal(err)
				}
				call := &values.FunctionCallExpression{ClassName: member.Name, FunctionName: "<init>", Descriptor: member.Description, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: pc, Object: receiver, FuncType: ft.FunctionType(), Arguments: []values.JavaValue{values.NewJavaLiteral(literal.Value, types.NewJavaPrimer(types.JavaInteger)), value, fail}}
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
				literal.Value = 1000011
				if !bytes.Equal(beforeCode, code.Code) {
					t.Fatal("CP-only mutation changed Code")
				}
				for n, b := range files {
					if n == "ReviewGenericLiteralChild" {
						b = object.Bytes()
					}
					if err := os.WriteFile(filepath.Join(mutant, n+".class"), b, 0600); err != nil {
						t.Fatal(err)
					}
				}
				run(mutant, "1000011")
				fresh := receipt.fresh(d.FuncCtx)
				source, renderErr := plan.renderDelegation(d.FuncCtx)
				claimed := renderErr == nil && receipt.claimHeader(plan, call, pc, source, d.FuncCtx)
				emitted := claimed && receipt.confirmEmission(plan, d.FuncCtx, "\n"+source+";\n", "")
				record := map[string]any{"code_unchanged": bytes.Equal(beforeCode, code.Code), "ldc_pc": ldcPC, "literal_constant_index": index, "original_literal": 1000009, "changed_literal": 1000011, "receipt_fresh": fresh, "rendered_source": source, "render_error": renderErr, "header_claimed": claimed, "emission_confirmed": emitted, "consumed": receipt.consumed(d.FuncCtx)}
				encoded, _ := json.MarshalIndent(record, "", "  ")
				os.WriteFile(filepath.Join(dir, "receipt-observation.json"), append(encoded, '\n'), 0600)
				if fresh || renderErr == nil || claimed || emitted {
					t.Errorf("stale LDC semantics accepted by typed receipt: %s", encoded)
				}
			})
		}
	}
}
