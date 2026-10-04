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
	desc := "(Lexample/Input;Lexample/Input;)Lexample/Sequence;"
	sig := "<T:Ljava/lang/Object;>(Lexample/Input<+TT;>;Lexample/Input<+TT;>;)Lexample/Sequence<TT;>;"
	ctx := &class_context.ClassContext{ClassName: "example.Flow", FunctionName: "join", CurrentMethodDesc: desc, IsStatic: true, TypeParams: []string{"T"}, MethodSignaturesByDesc: map[string]string{class_context.MethodDescKey("join", desc): sig}}
	ctx.SiblingClassSig = func(owner string) (string, map[string]string, bool) {
		switch owner {
		case "example/Sequence":
			return "<E:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{
				class_context.MethodDescKey("factory", "([Ljava/lang/Object;)Lexample/Sequence;"): "<V:Ljava/lang/Object;>([TV;)Lexample/Sequence<TV;>;",
				class_context.MethodDescKey("flatten", "(Lexample/Mapper;)Lexample/Sequence;"):    "<R:Ljava/lang/Object;>(Lexample/Mapper<-TE;+Lexample/Input<+TR;>;>;)Lexample/Sequence<TR;>;",
			}, true
		case "example/Functions":
			return "Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("identity", "()Lexample/Mapper;"): "<V:Ljava/lang/Object;>()Lexample/Mapper<TV;TV;>;"}, true
		}
		return "", nil, false
	}
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
	factory := &values.FunctionCallExpression{ClassName: "example.Sequence", FunctionName: "factory", Descriptor: "([Ljava/lang/Object;)Lexample/Sequence;", Kind: values.InvokeStatic, HasOriginPC: true, OriginPC: 10}
	invoke.stackProduced = []values.JavaValue{factory}
	mapper := &values.FunctionCallExpression{ClassName: "example.Functions", FunctionName: "identity", Descriptor: "()Lexample/Mapper;", Kind: values.InvokeStatic, HasOriginPC: true, OriginPC: 11}
	mapperOp := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 11, stackProduced: []values.JavaValue{mapper}}
	projection := &values.FunctionCallExpression{ClassName: "example.Sequence", FunctionName: "flatten", Descriptor: "(Lexample/Mapper;)Lexample/Sequence;", Kind: values.InvokeVirtual, HasOriginPC: true, OriginPC: 12, Object: factory, Arguments: []values.JavaValue{mapper}}
	consumer := &OpCode{Instr: &Instruction{OpCode: OP_INVOKEVIRTUAL}, CurrentOffset: 12, stackConsumed: []values.JavaValue{mapper, factory}, stackProduced: []values.JavaValue{projection}}
	ret := &OpCode{Instr: &Instruction{OpCode: OP_ARETURN}, CurrentOffset: 13, stackConsumed: []values.JavaValue{projection}}
	ops := []*OpCode{alloc, dup1, fill1, dup2, fill2, store, load, invoke, mapperOp, consumer, ret}
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

func TestGenericArrayDUPDeclarationRequiresUniqueAllocationOwner(t *testing.T) {
	for _, change := range []string{"valid", "wrong stack input", "wrong stack output", "incomplete outputs", "other duplication opcode", "second source definition", "parameter UID", "real local owner", "opaque temporary", "allocation alias", "two calls", "escaped allocation"} {
		t.Run(change, func(t *testing.T) {
			d, array, _, temp, nodes := genericArrayProofFixture()
			store, invoke := d.opCodes[5], d.opCodes[7]
			d.opCodes = append(d.opCodes[:5], append([]*OpCode(nil), d.opCodes[7:]...)...)
			d.opCodes[4].Target, invoke.Source = []*OpCode{invoke}, []*OpCode{d.opCodes[4]}
			invoke.stackConsumed = []values.JavaValue{temp}
			delete(d.opcodeIdToRef, store)
			delete(d.cachedSlotWebs.webOf, store)
			d.RootNode.Next = nil
			switch change {
			case "wrong stack input":
				d.opCodes[1].stackConsumed[0] = values.JavaNull
			case "wrong stack output":
				d.opCodes[1].stackProduced[1] = values.JavaNull
			case "incomplete outputs":
				d.opCodes[1].stackProduced = d.opCodes[1].stackProduced[:1]
			case "other duplication opcode":
				d.opCodes[1].Instr.OpCode = OP_DUP_X1
			case "second source definition":
				copy := *temp
				d.RootNode.Next = []*Node{{Id: 20, Statement: &statements.AssignStatement{LeftValue: &copy, JavaValue: array}}}
			case "parameter UID":
				temp.VarUid = d.Params[0].(*values.JavaRef).VarUid
			case "real local owner":
				d.opCodes = append(d.opCodes, store)
				d.cachedSlotWebs.webOf[store] = 1
				d.opcodeIdToRef[store] = [][2]any{{temp, nil}}
			case "opaque temporary":
				temp.CustomValue = values.NewCustomValue(nil, nil)
			case "allocation alias":
				copy := values.NewJavaRef(utils.NewRootVariableId(), array, array.Type())
				temp.Val = copy
			case "two calls":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 11, stackConsumed: []values.JavaValue{temp}})
			case "escaped allocation":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_PUTSTATIC}, CurrentOffset: 11, stackConsumed: []values.JavaValue{temp}})
			}
			d.recoverGenericArrayDeclarations(nodes)
			if got := temp.WebDeclType != nil; got != (change == "valid") {
				t.Fatalf("DUP declaration recovered=%v", got)
			}
			if _, changed := types.AsParameterizedType(array.ElementType()); changed {
				t.Fatal("DUP declaration changed the reifiable allocation type")
			}
		})
	}
}

func TestGenericArrayFactoryRequiresCompleteInferencePath(t *testing.T) {
	for _, change := range []string{"valid", "missing metadata", "other factory overload", "bounded factory", "raw factory", "different factory variable", "factory direct return", "factory escape", "factory repeated receiver", "factory as argument", "projection alternate entry", "projection handler gap", "missing factory witness", "wrong factory opcode", "missing projection witness", "wrong projection opcode", "wrong projection operand", "mapper arguments", "missing mapper witness", "wrong mapper opcode", "mapper not identity", "bounded mapper", "invariant projection", "different projection output", "reference extra constraint", "different caller return", "projection escape", "projection repeated return"} {
		t.Run(change, func(t *testing.T) {
			d, array, local, temp, nodes := genericArrayProofFixture()
			factoryOp, mapperOp, projectionOp, retOp := d.opCodes[7], d.opCodes[8], d.opCodes[9], d.opCodes[10]
			factory := factoryOp.stackProduced[0].(*values.FunctionCallExpression)
			mapper := mapperOp.stackProduced[0].(*values.FunctionCallExpression)
			projection := projectionOp.stackProduced[0].(*values.FunctionCallExpression)
			rewriteSig := func(owner, name, desc, sig string) {
				previous := d.FunctionContext.SiblingClassSig
				d.FunctionContext.SiblingClassSig = func(class string) (string, map[string]string, bool) {
					classSig, methods, known := previous(class)
					if class == owner {
						copy := map[string]string{}
						for k, v := range methods {
							copy[k] = v
						}
						copy[class_context.MethodDescKey(name, desc)] = sig
						methods = copy
					}
					return classSig, methods, known
				}
			}
			switch change {
			case "missing metadata":
				d.FunctionContext.SiblingClassSig = nil
			case "other factory overload":
				factory.Descriptor = "([Ljava/lang/Object;I)Lexample/Sequence;"
			case "bounded factory":
				rewriteSig("example/Sequence", "factory", factory.Descriptor, "<V:Ljava/lang/Number;>([TV;)Lexample/Sequence<TV;>;")
			case "raw factory":
				rewriteSig("example/Sequence", "factory", factory.Descriptor, "([Ljava/lang/Object;)Lexample/Sequence;")
			case "different factory variable":
				rewriteSig("example/Sequence", "factory", factory.Descriptor, "<V:Ljava/lang/Object;>([TV;)Lexample/Sequence<Ljava/lang/String;>;")
			case "factory direct return":
				projectionOp.Instr.OpCode = OP_ARETURN
			case "factory escape":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_PUTSTATIC}, CurrentOffset: 14, stackConsumed: []values.JavaValue{factory}})
			case "factory repeated receiver":
				d.opCodes = append(d.opCodes, projectionOp)
			case "factory as argument":
				projectionOp.stackConsumed = []values.JavaValue{factory, mapper}
			case "projection alternate entry":
				projectionOp.Source = append(projectionOp.Source, &OpCode{})
			case "projection handler gap":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 12, HandlerPc: 20}}
			case "missing factory witness":
				factory.HasOriginPC = false
			case "wrong factory opcode":
				factory.Kind = values.InvokeVirtual
			case "missing projection witness":
				projection.HasOriginPC = false
			case "wrong projection opcode":
				projection.Kind = values.InvokeStatic
			case "wrong projection operand":
				projection.Arguments[0] = values.JavaNull
			case "mapper arguments":
				mapper.Arguments = []values.JavaValue{values.JavaNull}
			case "missing mapper witness":
				mapper.HasOriginPC = false
			case "wrong mapper opcode":
				mapperOp.Instr.OpCode = OP_INVOKEVIRTUAL
			case "mapper not identity":
				rewriteSig("example/Functions", "identity", mapper.Descriptor, "<V:Ljava/lang/Object;>()Lexample/Mapper<TV;Ljava/lang/String;>;")
			case "bounded mapper":
				rewriteSig("example/Functions", "identity", mapper.Descriptor, "<V:Ljava/lang/Number;>()Lexample/Mapper<TV;TV;>;")
			case "invariant projection":
				rewriteSig("example/Sequence", "flatten", projection.Descriptor, "<R:Ljava/lang/Object;>(Lexample/Mapper<TE;Lexample/Input<TR;>;>;)Lexample/Sequence<TR;>;")
			case "different projection output":
				rewriteSig("example/Sequence", "flatten", projection.Descriptor, "<R:Ljava/lang/Object;>(Lexample/Mapper<-TE;+Lexample/Input<+TR;>;>;)Lexample/Sequence<TE;>;")
			case "reference extra constraint":
				projection.Arguments = append(projection.Arguments, values.JavaNull)
				projectionOp.stackConsumed = []values.JavaValue{values.JavaNull, mapper, factory}
				rewriteSig("example/Sequence", "flatten", projection.Descriptor, "<R:Ljava/lang/Object;>(Lexample/Mapper<-TE;+Lexample/Input<+TR;>;>;TR;)Lexample/Sequence<TR;>;")
			case "different caller return":
				key := class_context.MethodDescKey(d.FunctionContext.FunctionName, d.FunctionContext.CurrentMethodDesc)
				d.FunctionContext.MethodSignaturesByDesc[key] = "<T:Ljava/lang/Object;>(Lexample/Input<+TT;>;Lexample/Input<+TT;>;)Lexample/Sequence<Ljava/lang/String;>;"
			case "projection escape":
				retOp.Instr.OpCode = OP_ASTORE_3
			case "projection repeated return":
				d.opCodes = append(d.opCodes, retOp)
			}
			d.recoverGenericArrayDeclarations(nodes)
			if got := local.WebDeclType != nil; got != (change == "valid") {
				t.Fatalf("inference path recovered=%v", got)
			}
			if _, changed := types.AsParameterizedType(array.ElementType()); changed || (change != "valid" && temp.WebDeclType != nil) {
				t.Fatal("rejected inference path partly changed an allocation/declaration")
			}
		})
	}
}
