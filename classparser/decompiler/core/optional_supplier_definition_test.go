package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOptionalSupplierViewNeedsUniqueDominatingRecordedFactory(t *testing.T) {
	for _, kind := range []string{"valid", "typed receiver", "reassigned", "shared identity", "parameter", "empty identity", "stale ref definition", "unknown rhs", "missing origin", "wrong origin", "missing factory map", "missing signature", "wrong payload type", "parameterized supplier", "missing graph", "normal bypass", "exception bypass", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			factoryOp := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 0}
			store := &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_1}, CurrentOffset: 3}
			use := &OpCode{Instr: &Instruction{OpCode: OP_INVOKEVIRTUAL}, CurrentOffset: 4}
			mt, _ := types.ParseMethodDescriptor("()Ljava/util/function/Supplier;")
			factory := values.NewFunctionCallExpression(nil, values.NewJavaClassMember("example.Factory", "make", "()Ljava/util/function/Supplier;", mt), mt.FunctionType())
			factory.IsStatic, factory.HasOriginPC, factory.OriginPC = true, true, 0
			factory.SourceReturnType = types.NewParameterizedType("java.util.function.Supplier", []types.JavaType{types.NewJavaClass("java.io.IOException")})
			supplier := values.NewJavaRef(utils.NewRootVariableId(), factory, types.NewJavaClass("java.util.function.Supplier"))
			store.stackConsumed = []values.JavaValue{factory}
			callType, _ := types.ParseMethodDescriptor("(Ljava/util/function/Supplier;)Ljava/lang/Object;")
			receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.Optional"))
			call := values.NewFunctionCallExpression(receiver, values.NewJavaClassMember("java.util.Optional", "orElseThrow", "(Ljava/util/function/Supplier;)Ljava/lang/Object;", callType), callType.FunctionType())
			call.HasOriginPC, call.OriginPC = true, 4
			call.Arguments = []values.JavaValue{supplier}
			g := &SemanticCFG{Nodes: []*OpCode{factoryOp, store, use}, Edges: []SemanticEdge{{From: factoryOp, To: store, Kind: EdgeFallthrough}, {From: store, To: use, Kind: EdgeFallthrough}}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, opCodes: g.Nodes, semanticCFG: g, opcodeIdToRef: map[*OpCode][][2]any{store: {{supplier, true}}}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{factoryOp: factory, use: call}}
			switch kind {
			case "typed receiver":
				receiver.ResetVarType(types.NewParameterizedType("java.util.Optional", []types.JavaType{types.NewJavaClass("java.lang.String")}))
			case "reassigned", "shared identity":
				other := &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_1}, CurrentOffset: 5, stackConsumed: []values.JavaValue{values.JavaNull}}
				alias := supplier
				if kind == "shared identity" {
					alias = values.NewJavaRef(supplier.Id, nil, supplier.Type())
					alias.VarUid = supplier.VarUid
				}
				d.opCodes = append(d.opCodes, other)
				d.opcodeIdToRef[other] = [][2]any{{alias, false}}
			case "parameter":
				supplier.IsParam = true
			case "empty identity":
				supplier.VarUid = ""
			case "stale ref definition", "unknown rhs":
				store.stackConsumed[0] = values.NewJavaRef(utils.NewRootVariableId(), factory, supplier.Type())
			case "missing origin":
				factory.HasOriginPC = false
			case "wrong origin":
				factory.OriginPC = 1
			case "missing factory map":
				delete(d.invokeFuncCall, factoryOp)
			case "missing signature":
				factory.SourceReturnType = nil
			case "wrong payload type":
				factory.SourceReturnType = types.NewParameterizedType("java.util.function.Supplier", []types.JavaType{types.NewJavaClass("java.lang.String")})
			case "parameterized supplier":
				supplier.ResetVarType(factory.SourceReturnType.Copy())
			case "missing graph":
				d.semanticCFG = nil
			case "normal bypass", "exception bypass":
				edgeKind := EdgeTaken
				if kind == "exception bypass" {
					edgeKind = EdgeException
				}
				g.Edges = append(g.Edges, SemanticEdge{From: factoryOp, To: use, Kind: edgeKind})
			case "disabled":
				t.Setenv("JDEC_OPTIONAL_SUPPLIER_DEFINITION_OFF", "1")
			}
			d.restoreOptionalSupplierDefinitionViews()
			cast, changed := call.Arguments[0].(*values.CastExpression)
			want := kind == "valid" || kind == "typed receiver"
			if changed != want {
				t.Fatalf("supplier view=%t, want=%t", changed, want)
			}
			if want && (!cast.Binding || cast.Value != supplier || cast.TargetType.String(d.FunctionContext) != "Supplier<IOException>" || supplier.Type().String(d.FunctionContext) != "Supplier") {
				t.Fatal("view changed supplier identity, declaration or exception target")
			}
		})
	}
}
