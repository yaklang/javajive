package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestPrivateArrayReceiverReadRequiresCompleteOriginalOwnership(t *testing.T) {
	for _, variant := range []string{"original", "routing", "parameter", "THIS", "opaque", "stack ref", "missing allocation PC", "missing end PC", "missing read PC", "wrong read PC", "missing type", "wrong component", "parameterized component", "wrong CP", "missing CP", "wrong length", "changed original length", "wrong index", "wrong element", "wrong store index", "wrong end", "wrong load opcode", "wrong load result", "named local store", "published array", "extra original use", "alternate entry", "handler change", "source escape", "source duplicate", "source side effect", "unowned source condition", "source cycle", "missing source witness", "budget", "allocation budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			literal := func(n int) *values.JavaLiteral { return values.NewJavaLiteral(n, integer) }
			component := types.NewJavaClass("java.lang.String")
			length, slot, index := literal(1), literal(0), literal(0)
			element := values.NewJavaLiteral("element", component)
			array := values.NewNewArrayExpression(types.NewJavaArrayType(component), length)
			array.OriginPC, array.HasOriginPC = 1, true
			array.EvaluationEndPC, array.HasEvaluationEndPC = 5, true
			array.Initializer = []values.JavaValue{element}
			ref := values.NewJavaRef(utils.NewRootVariableId(), array, array.Type())
			read := &values.JavaArrayMember{Object: ref, Index: index, OriginPC: 7, HasOriginPC: true}
			call := &values.FunctionCallExpression{Object: read, ClassName: "java.lang.String", FunctionName: "length", Descriptor: "()I", Kind: values.InvokeVirtual, OriginPC: 8, HasOriginPC: true, FuncType: &types.JavaFuncType{ReturnType: integer}}
			definition := NewNode(statements.NewAssignStatement(ref, array, true))
			definition.Id = 2
			consumer := NewNode(statements.NewReturnStatement(call))
			consumer.Id = 9
			link := func(a, b *Node) { a.AddNext(b); b.AddSource(a) }
			link(definition, consumer)
			op := func(pc, code int) *OpCode { return &OpCode{Id: pc, CurrentOffset: uint16(pc), Instr: InstrInfos[code]} }
			alloc, dup, idx, value, store, readIndex, load, invoke := op(1, OP_ANEWARRAY), op(2, OP_DUP), op(3, OP_ICONST_0), op(4, OP_LDC), op(5, OP_AASTORE), op(6, OP_ICONST_0), op(7, OP_AALOAD), op(8, OP_INVOKEVIRTUAL)
			alloc.Data = []byte{0, 1}
			alloc.stackConsumed, alloc.stackProduced = []values.JavaValue{length}, []values.JavaValue{array}
			dup.stackConsumed, dup.stackProduced = []values.JavaValue{array}, []values.JavaValue{ref, ref}
			idx.stackProduced, value.stackProduced = []values.JavaValue{slot}, []values.JavaValue{element}
			store.stackConsumed = []values.JavaValue{element, slot, ref}
			readIndex.stackProduced = []values.JavaValue{index}
			load.stackConsumed, load.stackProduced = []values.JavaValue{index, ref}, []values.JavaValue{read}
			invoke.stackConsumed, invoke.stackProduced = []values.JavaValue{read}, []values.JavaValue{call}
			ops := []*OpCode{alloc, dup, idx, value, store, readIndex, load, invoke}
			for i := 0; i < len(ops)-1; i++ {
				ops[i].Target = []*OpCode{ops[i+1]}
				ops[i+1].Source = []*OpCode{ops[i]}
			}
			d := &Decompiler{RootNode: definition, FunctionContext: &class_context.ClassContext{}, opCodes: ops, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
			d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{}
			for _, instruction := range ops {
				d.opcodeToSimulateStack[instruction] = nil
			}
			d.constantPoolGetter = func(int) values.JavaValue { return &values.JavaClassValue{JavaType: component} }
			origins := map[int]*OpCode{2: dup}
			switch variant {
			case "routing", "source side effect", "unowned source condition", "source cycle":
				middle := NewNode(statements.NewGOTOStatement())
				definition.ReplaceNext(consumer, middle)
				link(middle, consumer)
				switch variant {
				case "source side effect":
					middle.Statement = statements.NewExpressionStatement(&values.FunctionCallExpression{FunctionName: "discarded"})
				case "unowned source condition":
					middle.Statement = &statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), TernaryChainArm: true}
				case "source cycle":
					middle.ReplaceNext(consumer, middle)
				}
			case "parameter":
				ref.IsParam = true
			case "THIS":
				ref.IsThis = true
			case "opaque":
				ref.CustomValue = &values.CustomValue{}
			case "stack ref":
				ref.StackVar = values.JavaNull
			case "missing allocation PC":
				array.HasOriginPC = false
			case "missing end PC":
				array.HasEvaluationEndPC = false
			case "missing read PC":
				read.HasOriginPC = false
			case "wrong read PC":
				read.OriginPC++
			case "missing type":
				array.JavaType = nil
			case "wrong component":
				array.JavaType = types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
			case "parameterized component":
				array.JavaType = types.NewJavaArrayType(types.NewParameterizedType("java.lang.String", []types.JavaType{types.NewJavaClass("T")}))
			case "wrong CP":
				d.constantPoolGetter = func(int) values.JavaValue {
					return &values.JavaClassValue{JavaType: types.NewJavaClass("java.lang.Object")}
				}
			case "missing CP":
				d.constantPoolGetter = nil
			case "wrong length":
				array.Length = []values.JavaValue{literal(2)}
			case "changed original length":
				alloc.stackConsumed = []values.JavaValue{literal(1)}
			case "wrong index":
				read.Index = literal(0)
			case "wrong element":
				array.Initializer = []values.JavaValue{values.NewJavaLiteral("element", component)}
			case "wrong store index":
				store.stackConsumed[1] = literal(1)
			case "wrong end":
				array.EvaluationEndPC--
			case "wrong load opcode":
				load.Instr = InstrInfos[OP_ARRAYLENGTH]
			case "wrong load result":
				load.stackProduced = []values.JavaValue{ref}
			case "named local store", "published array", "extra original use":
				extra := op(10, OP_ASTORE_1)
				if variant == "published array" {
					extra.Instr = InstrInfos[OP_PUTSTATIC]
				} else if variant == "extra original use" {
					extra.Instr = InstrInfos[OP_IFNULL]
				}
				extra.stackConsumed = []values.JavaValue{ref}
				d.opCodes = append(d.opCodes, extra)
			case "alternate entry":
				load.Source = append(load.Source, op(11, OP_GOTO))
			case "handler change":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 6, HandlerPc: 20}}
			case "source escape":
				escape := NewNode(statements.NewReturnStatement(ref))
				link(consumer, escape)
			case "source duplicate":
				consumer.Statement = statements.NewReturnStatement(values.NewBinaryExpression(call, call, "+", integer))
			case "missing source witness":
				delete(origins, 2)
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.inlinePrivateArrayReceiverReads(origins)
			want := variant == "original" || variant == "routing"
			if (read.Object == array) != want {
				t.Fatalf("accepted=%v", read.Object == array)
			}
			if !want && (read.Object != ref || d.RootNode != definition || len(definition.Next) != 1) {
				t.Fatal("refused transaction changed source ownership")
			}
		})
	}
}
