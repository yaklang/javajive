package core

import (
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func genericArrayProofFixture() (*Decompiler, *values.NewExpression, *values.JavaRef, *values.JavaRef, map[int]*OpCode) {
	raw := types.NewJavaClass("example.Input")
	desc := "(Lexample/Input;Lexample/Input;)Ljava/lang/Object;"
	sig := "<T:Ljava/lang/Object;>(Lexample/Input<+TT;>;Lexample/Input<+TT;>;)Ljava/lang/Object;"
	ctx := &class_context.ClassContext{ClassName: "example.Flow", FunctionName: "join", CurrentMethodDesc: desc, IsStatic: true, TypeParams: []string{"T"}, MethodSignaturesByDesc: map[string]string{class_context.MethodDescKey("join", desc): sig}}
	a := values.NewJavaRef(utils.NewRootVariableId(), nil, raw)
	b := values.NewJavaRef(utils.NewRootVariableId(), nil, raw)
	a.IsParam, b.IsParam = true, true
	arrayType := types.NewJavaArrayType(raw)
	array := values.NewNewArrayExpression(arrayType, values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger)))
	array.Initializer = []values.JavaValue{a, b}
	array.OriginPC, array.HasOriginPC = 1, true
	array.EvaluationEndPC, array.HasEvaluationEndPC = 7, true
	temp := values.NewJavaRef(utils.NewRootVariableId(), array, arrayType)
	local := values.NewJavaRef(utils.NewRootVariableId(), values.NewSlotValue(temp, nil), arrayType)
	alloc := &OpCode{Instr: &Instruction{OpCode: OP_ANEWARRAY}, CurrentOffset: 1}
	dup1 := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 2, stackConsumed: []values.JavaValue{temp}, stackProduced: []values.JavaValue{temp, temp}}
	fill1 := &OpCode{Instr: &Instruction{OpCode: OP_AASTORE}, CurrentOffset: 4, stackConsumed: []values.JavaValue{a, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), temp}}
	dup2 := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 5, stackConsumed: []values.JavaValue{temp}, stackProduced: []values.JavaValue{temp, temp}}
	fill2 := &OpCode{Instr: &Instruction{OpCode: OP_AASTORE}, CurrentOffset: 7, stackConsumed: []values.JavaValue{b, values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), temp}}
	store := &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_2}, CurrentOffset: 8, stackConsumed: []values.JavaValue{temp}}
	load := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_2}, CurrentOffset: 9}
	invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 10, stackConsumed: []values.JavaValue{local}}
	ops := []*OpCode{alloc, dup1, fill1, dup2, fill2, store, load, invoke}
	d := &Decompiler{FunctionContext: ctx, FunctionType: types.NewJavaFuncType(desc, []types.JavaType{raw, raw}, types.NewJavaClass("java.lang.Object")), Params: []values.JavaValue{a, b}, opCodes: ops, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}, opcodeIdToRef: map[*OpCode][][2]any{store: {{local, nil}}}, cachedSlotWebs: &slotWeb{webOf: map[*OpCode]int{store: 1, load: 1}, entryWeb: map[int]int{0: 2, 1: 3}}}
	for i, op := range ops {
		d.opcodeToSimulateStack[op] = nil
		if i > 0 {
			op.Source = []*OpCode{ops[i-1]}
			ops[i-1].Target = []*OpCode{op}
		}
	}
	allocNode := &Node{Id: 2, Statement: &statements.AssignStatement{LeftValue: temp, JavaValue: array}}
	storeNode := &Node{Id: 8, Statement: &statements.AssignStatement{LeftValue: local, JavaValue: local.Val}}
	allocNode.Next, storeNode.Source = []*Node{storeNode}, []*Node{allocNode}
	d.RootNode = allocNode
	return d, array, local, temp, map[int]*OpCode{2: dup1, 8: store}
}

func TestGenericArrayDeclarationRequiresExactWebAndFormalProof(t *testing.T) {
	for _, change := range []string{"valid", "missing signature", "other overload", "foreign type variable", "static class variable", "wrong formal erasure", "parameter identity", "mixed formal types", "raw formal", "opaque operand", "cyclic alias", "missing allocation witness", "partial initializer", "empty initializer", "oversized initializer", "wrong component class", "multidimensional", "parameter reassignment", "local reassignment", "shared UID", "stored alias", "escape", "array read", "post-init write", "repeated use", "alternate entry", "handler gap", "bad DUP", "unproven web", "disabled webs", "opcode budget"} {
		t.Run(change, func(t *testing.T) {
			d, array, local, temp, nodes := genericArrayProofFixture()
			before := array.Type().Copy()
			sigKey := class_context.MethodDescKey("join", d.FunctionContext.CurrentMethodDesc)
			sig := d.FunctionContext.MethodSignaturesByDesc[sigKey]
			switch change {
			case "missing signature":
				d.FunctionContext.MethodSignaturesByDesc = nil
			case "other overload":
				d.FunctionContext.CurrentMethodDesc = "()Ljava/lang/Object;"
			case "foreign type variable":
				d.FunctionContext.MethodSignaturesByDesc[sigKey] = "<T:Ljava/lang/Object;>(Lexample/Input<+TU;>;Lexample/Input<+TU;>;)Ljava/lang/Object;"
			case "static class variable":
				d.FunctionContext.MethodSignaturesByDesc[sigKey] = sig[len("<T:Ljava/lang/Object;>"):]
			case "wrong formal erasure":
				d.FunctionType.ParamTypes[0] = types.NewJavaClass("example.Other")
			case "parameter identity":
				copy := *(d.Params[0].(*values.JavaRef))
				array.Initializer[0] = &copy
			case "mixed formal types":
				d.FunctionContext.MethodSignaturesByDesc[sigKey] = "<T:Ljava/lang/Object;>(Lexample/Input<+TT;>;Lexample/Input<Ljava/lang/String;>;)Ljava/lang/Object;"
			case "raw formal":
				d.FunctionContext.MethodSignaturesByDesc[sigKey] = "<T:Ljava/lang/Object;>(Lexample/Input;Lexample/Input<+TT;>;)Ljava/lang/Object;"
			case "opaque operand":
				array.Initializer[0] = values.NewCustomValue(nil, nil)
			case "cyclic alias":
				temp.Val = temp
			case "missing allocation witness":
				array.HasOriginPC = false
			case "partial initializer":
				array.HasEvaluationEndPC = false
			case "empty initializer":
				array.Initializer = nil
			case "oversized initializer":
				for len(array.Initializer) < 65 {
					array.Initializer = append(array.Initializer, d.Params[0])
				}
			case "wrong component class":
				array.JavaType = types.NewJavaArrayType(types.NewJavaClass("example.Other"))
				before = array.Type().Copy()
			case "multidimensional":
				array.JavaType = types.NewJavaArrayType(array.JavaType)
				before = array.Type().Copy()
			case "parameter reassignment":
				store := &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_0}, CurrentOffset: 0}
				d.opCodes = append(d.opCodes, store)
				d.cachedSlotWebs.webOf[store] = 2
				d.opcodeIdToRef[store] = [][2]any{{d.Params[0], nil}}
			case "local reassignment", "shared UID", "stored alias":
				store := &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_2}, CurrentOffset: 11}
				ref := local
				web := 1
				if change == "shared UID" {
					copy := *local
					ref, web = &copy, 4
				}
				if change == "stored alias" {
					ref, web = temp, 4
				}
				d.opCodes = append(d.opCodes, store)
				d.cachedSlotWebs.webOf[store] = web
				d.opcodeIdToRef[store] = [][2]any{{ref, nil}}
			case "escape", "array read", "post-init write":
				opcode := OP_PUTSTATIC
				if change == "array read" {
					opcode = OP_AALOAD
				}
				if change == "post-init write" {
					opcode = OP_AASTORE
				}
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: opcode}, CurrentOffset: 11, stackConsumed: []values.JavaValue{local}})
			case "repeated use":
				d.opCodes[7].stackConsumed = append(d.opCodes[7].stackConsumed, local)
			case "alternate entry":
				d.opCodes[7].Source = append(d.opCodes[7].Source, &OpCode{})
			case "handler gap":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 8, HandlerPc: 20}}
			case "bad DUP":
				d.opCodes[1].stackProduced[1] = values.JavaNull
			case "unproven web":
				delete(d.cachedSlotWebs.webOf, d.opCodes[5])
			case "disabled webs":
				d.Env = func(string) string { return "1" }
			case "opcode budget":
				for len(d.opCodes) <= 4096 {
					d.opCodes = append(d.opCodes, d.opCodes[0])
				}
			}
			d.recoverGenericArrayDeclarations(nodes)
			if got := local.WebDeclType != nil; got != (change == "valid") {
				t.Fatalf("declaration recovered=%v", got)
			}
			if !reflect.DeepEqual(array.Type().RawType(), before.RawType()) {
				t.Fatal("declaration recovery changed the allocated JVM array type")
			}
			if change == "valid" {
				if local.WebDeclType.String(d.FunctionContext) != "Input<? extends T>[]" || temp.WebDeclType == nil || !reflect.DeepEqual(local.WebDeclType.RawType(), temp.WebDeclType.RawType()) {
					t.Fatal("DUP temporary and sole source local lost their declaration proof")
				}
			} else if temp.WebDeclType != nil {
				t.Fatal("a rejected proof partly changed the allocation temporary")
			}
		})
	}
}
