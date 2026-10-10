package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousInitializerCastRequiresOriginalRuntimeTarget(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousCastInitializerFixture)
	for _, variant := range []string{"original", "nil opcode", "wrong opcode", "operand bytes", "wrong pool target", "bad pool index", "synthetic cast", "binding cast", "negative origin", "wrong origin", "changed target", "missing target", "missing context", "class formal shadow", "method formal shadow", "lexical formal shadow", "malformed signature", "budget"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(append([]byte(nil), files["CastInitOwner$1.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			index := obj.ConstantPoolManager.AddNewClassInfo("java/lang/String")
			op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_CHECKCAST}, Data: []byte{byte(index >> 8), byte(index)}, CurrentOffset: 17}
			value := values.NewOriginalCheckCast(values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object")), types.NewJavaClass("java.lang.String"), 17)
			ctx := &class_context.ClassContext{}
			var work *workbudget.Budget
			switch variant {
			case "nil opcode":
				op = nil
			case "wrong opcode":
				op.Instr.OpCode = core.OP_INSTANCEOF
			case "operand bytes":
				op.Data = op.Data[:1]
			case "wrong pool target":
				index = obj.ConstantPoolManager.AddNewClassInfo("java/lang/Object")
				op.Data = []byte{byte(index >> 8), byte(index)}
			case "bad pool index":
				op.Data = []byte{255, 255}
			case "synthetic cast":
				value.OriginalCheckCast = false
			case "binding cast":
				value.Binding = true
			case "negative origin":
				value.OriginPC = -1
			case "wrong origin":
				value.OriginPC = 18
			case "changed target":
				value.TargetType = types.NewJavaClass("java.lang.Object")
			case "missing target":
				value.TargetType = nil
			case "missing context":
				ctx = nil
			case "class formal shadow":
				ctx.ClassSig = "<String:Ljava/lang/Object;>Ljava/lang/Object;"
			case "method formal shadow":
				ctx.CurrentMethodSig = "<String:Ljava/lang/Object;>()V"
			case "lexical formal shadow":
				ctx.LexicalTypeParamSignatures = []string{"<String:Ljava/lang/Object;>Ljava/lang/Object;"}
			case "malformed signature":
				ctx.CurrentMethodSig = "("
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			}
			if got := nativeAnonymousInitializerOriginalCast(obj, op, value, ctx, work); got != (variant == "original") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
