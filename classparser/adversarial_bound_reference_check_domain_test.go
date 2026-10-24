package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialBoundReferenceEagerCheckMustDominateFactory(t *testing.T) {
	compiler, java := t04Tools(t)
	for _, bypass := range []bool{false, true} {
		label := "unconditional"
		literal := "false"
		if bypass {
			label = "conditional check"
			literal = "true"
		}
		t.Run(label, func(t *testing.T) {
			fixture := strings.ReplaceAll(`import java.util.function.Consumer;
class CheckTarget {public void append(Object...values){}}
public class CheckOwner {static Consumer<Object> make(CheckTarget target,boolean check){return target::append;}}
class CheckDriver {public static void main(String[]args){int rows=0;for(CheckTarget target:new CheckTarget[]{null,new CheckTarget()})for(boolean check:new boolean[]{false,true}){boolean made=false;Throwable error=null;try{Consumer<Object> action=CheckOwner.make(target,check);made=true;action.accept(null);}catch(Throwable e){error=e;}if(made!=(target!=null||$BYPASS&&!check)||target==null&&!(error instanceof NullPointerException)||target!=null&&error!=null)throw new AssertionError("original conditional null timing",error);rows++;}System.out.println(rows+":checked:factory:domain");}}`, "$BYPASS", literal)
			files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, fixture, "CheckOwner", "none", false)
			obj := mustParseLowering(t, files["CheckOwner.class"])
			var method, impl *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "make" {
					method = m
				}
				if m.AccessFlags == 0x100a {
					impl = m
				}
			}
			if method == nil || impl == nil {
				t.Fatal("original factory/adapter absent")
			}
			code := constructorLoweringCode(method)
			decode := func() *core.Decompiler {
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if e := d.ParseOpcode(); e != nil {
					t.Fatal(e)
				}
				return d
			}
			if bypass {
				var check, dynamic []byte
				for _, op := range constructorMotionOps(decode()) {
					if nativeMemberNullCheck(obj, op) {
						check = append([]byte{byte(op.Instr.OpCode)}, op.Data...)
					}
					if op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
						dynamic = append([]byte{byte(op.Instr.OpCode)}, op.Data...)
					}
				}
				if len(check) != 3 || len(dynamic) != 5 {
					t.Fatal("original null-check/factory packet changed")
				}
				code.Code = []byte{core.OP_ALOAD_0, core.OP_ILOAD_1, core.OP_IFEQ, 0, 10, core.OP_POP, core.OP_ALOAD_0, core.OP_DUP}
				code.Code = append(code.Code, check...)
				code.Code = append(code.Code, core.OP_POP)
				code.Code = append(code.Code, dynamic...)
				code.Code = append(code.Code, core.OP_ARETURN)
				class := obj.ConstantPoolManager.AddNewClassInfo("CheckTarget")
				obj.ConstantPoolManager.AddUtf8Info("StackMapTable")
				code.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "StackMapTable", Length: 6, Info: []byte{0, 1, 76, 7, byte(class >> 8), byte(class)}}}
				code.AttrLen = 42
				code.MaxStack = 2
				code.MaxLocals = 2
				code.ExceptionTable = nil
			}
			files["CheckOwner.class"] = obj.Bytes()
			if got := t04RunJava(t, java, sourceArtifactWriteFiles(t, files), "CheckDriver"); got != "4:checked:factory:domain\n" {
				t.Fatal("independent original oracle", got)
			}
			ops := constructorMotionOps(decode())
			at := -1
			for i, op := range ops {
				if op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
					at = i
				}
			}
			_, _, known := nativeBoundAdapterCaptureOrigin(obj, method, impl, code, ops, at, []string{"LCheckTarget;"}, nil)
			if known == bypass {
				t.Fatalf("eager check proof=%v, bypass=%v", known, bypass)
			}
		})
	}
}
