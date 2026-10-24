package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestAdversarialConstructorDiscardedPrefixRequiresNoOriginalRead(t *testing.T) {
	for _, variant := range []string{"control", "original declaration witness", "missing store witness", "changed RHS", "changed store PC", "changed call PC", "missing call PC", "parameter slot", "slot read", "wide overlapping read", "unrelated read", "increment", "legacy RET", "wrong store category", "wide store encoding", "graph budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			code := []byte{core.OP_ILOAD_1, core.OP_INVOKESTATIC, 0, 1, core.OP_ASTORE_2, core.OP_ALOAD_0, core.OP_ILOAD_1, core.OP_INVOKESPECIAL, 0, 2, core.OP_RETURN}
			typ := types.NewJavaClass("java.lang.Object")
			call := &values.FunctionCallExpression{ClassName: "Effects", FunctionName: "produce", Descriptor: "(I)Ljava/lang/Object;", FuncType: &types.JavaFuncType{ReturnType: typ}, Kind: values.InvokeStatic, IsStatic: true, OriginPC: 1, HasOriginPC: true}
			local := values.NewJavaRef(utils.NewRootVariableId(), call, typ)
			a := statements.NewAssignStatement(local, call, true)
			a.OriginPC, a.HasOriginPC = 4, true
			if variant == "original declaration witness" {
				local.MarkOriginalLocalDeclaration(4, 2, call)
			}
			if variant != "missing store witness" && variant != "original declaration witness" {
				slot := 2
				if variant == "parameter slot" {
					slot = 1
					code[4] = core.OP_ASTORE_1
				}
				a.MarkOriginalLocalStore(4, slot)
			}
			reader := &ClassObjectDumper{obj: &ClassObject{ConstantPool: []ConstantInfo{&ConstantUtf8Info{Value: "(I)V"}}}}
			method := &MemberInfo{DescriptorIndex: 1}
			delegatePC := 7
			switch variant {
			case "changed RHS":
				a.JavaValue = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "changed store PC":
				a.OriginPC++
			case "changed call PC":
				call.OriginPC = 0
			case "missing call PC":
				call.HasOriginPC = false
			case "slot read":
				code = append(code[:10], core.OP_ALOAD_2, core.OP_POP, core.OP_RETURN)
			case "wide overlapping read":
				code = append(code[:10], core.OP_DLOAD_1, core.OP_POP2, core.OP_RETURN)
			case "unrelated read":
				code = append(code[:10], core.OP_ILOAD_1, core.OP_POP, core.OP_RETURN)
			case "increment":
				code = append(code[:10], core.OP_IINC, 2, 1, core.OP_RETURN)
			case "legacy RET":
				code = append(code[:10], core.OP_RET, 3)
			case "wrong store category":
				code[4] = core.OP_ISTORE_2
			case "wide store encoding":
				code = append(append(append([]byte{}, code[:4]...), core.OP_WIDE, core.OP_ASTORE, 0, 2), code[5:]...)
				delegatePC += 3
			case "graph budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			decoder := core.NewDecompiler(code, func(int) values.JavaValue { return nil })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			got := reader.constructorDiscardedPrefixEffect(a, decoder, method, delegatePC)
			want := variant == "control" || variant == "original declaration witness" || variant == "unrelated read" || variant == "wide store encoding"
			if (got != nil) != want || got != nil && got.Expression != call {
				t.Fatalf("discarded STORE proof=%v want=%v", got != nil, want)
			}
			if a.LeftValue != local {
				t.Fatal("original store target mutated")
			}
		})
	}
}
