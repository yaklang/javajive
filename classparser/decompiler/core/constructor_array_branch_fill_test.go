package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestPrivateDelegationArrayBranchOwnership(t *testing.T) {
	for _, change := range []string{"proved", "missing allocation PC", "missing store PC", "wrong index", "raw index mismatch", "wrong RHS witness", "foreign element", "unknown hierarchy", "self alias", "wrong parameter", "wrong descriptor", "missing invoke binding", "foreign receiver", "wrong duplicate", "local publication", "field publication", "extra call", "RHS reused", "back edge", "external entry", "handler boundary", "unowned condition", "catch entry", "post delegation reuse", "source-side effect", "effectful earlier argument", "wrong owner", "missing owner context", "this owner", "literal primer string", "wrapped proved", "wrapped wrong opcode", "wrapped missing producer", "wrapped wrong result identity", "wrapped alternate initialization entry", "wrapped effect after producer", "wrapped reused array", "wrapped incompatible array", "wrapped wrong producer owner", "wrapped missing origin", "wrapped wrong operand order", "nested array component", "nested incompatible component", "nested scalar component", "nested wrong rank"} {
		t.Run(change, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			literal := func(n int) *values.JavaLiteral { return values.NewJavaLiteral(n, integer) }
			text := func(s string) *values.JavaLiteral {
				return values.NewJavaLiteral(s, types.NewJavaClass("java.lang.String"))
			}
			arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
			array := values.NewNewArrayExpression(arrayType, literal(2))
			array.HasOriginPC = true
			array.OriginPC = 1
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
			receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Child"))
			receiver.IsThis = true
			condition := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
			condition.IsParam = true
			right := values.NewTernaryExpression(condition, text("yes"), text("no"))
			right.ConditionFromOp = 8
			indexes := []*values.JavaLiteral{literal(0), literal(1)}
			items := []values.JavaValue{text("first"), right}
			statement := statements.NewAssignStatement(ref, array, true)
			entry := NewNode(statement)
			entry.Id = 2
			storeNodes := make([]*Node, 2)
			for i := range storeNodes {
				member := &values.JavaArrayMember{Object: ref, Index: indexes[i]}
				store := statements.NewAssignStatement(member, items[i], false)
				store.ArrayMember = member
				store.HasOriginPC = true
				if i == 0 {
					store.OriginPC = 5
				} else {
					store.OriginPC = 12
				}
				storeNodes[i] = NewNode(store)
				storeNodes[i].Id = store.OriginPC
			}
			condStmt := &statements.ConditionStatement{Condition: condition, TernaryChainArm: true}
			cond := NewNode(condStmt)
			cond.Id = 8
			jumpA := NewNode(statements.NewGOTOStatement())
			jumpB := NewNode(statements.NewGOTOStatement())
			call := &values.FunctionCallExpression{Object: receiver, ClassName: "proof.Base", FunctionName: "<init>", Kind: values.InvokeSpecial, IsSpecialInvoke: true, OriginPC: 13, HasOriginPC: true, Descriptor: "([Ljava/lang/String;)V", Arguments: []values.JavaValue{ref}, FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{arrayType}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}}
			callNode := NewNode(statements.NewExpressionStatement(call))
			callNode.Id = 13
			exit := NewNode(statements.NewReturnStatement(nil))
			link := func(a, b *Node) { a.AddNext(b); b.AddSource(a) }
			link(entry, storeNodes[0])
			link(storeNodes[0], cond)
			link(cond, jumpA)
			link(cond, jumpB)
			link(jumpA, storeNodes[1])
			link(jumpB, storeNodes[1])
			link(storeNodes[1], callNode)
			link(callNode, exit)
			op := func(pc, code int) *OpCode {
				return &OpCode{Id: pc, CurrentOffset: uint16(pc), Instr: &Instruction{OpCode: code}}
			}
			alloc := op(1, OP_ANEWARRAY)
			alloc.stackProduced = []values.JavaValue{array}
			dup := op(2, OP_DUP)
			dup.stackConsumed = []values.JavaValue{array}
			dup.stackProduced = []values.JavaValue{ref, ref}
			idx0, val0, store0 := op(3, OP_ICONST_0), op(4, OP_LDC), op(5, OP_AASTORE)
			store0.stackConsumed = []values.JavaValue{items[0], indexes[0], ref}
			dup2, idx1, branch := op(6, OP_DUP), op(7, OP_ICONST_1), op(8, OP_IFEQ)
			dup2.stackProduced = []values.JavaValue{ref}
			armA, armB, join, store1, invoke := op(9, OP_LDC), op(10, OP_LDC), op(11, OP_NOP), op(12, OP_AASTORE), op(13, OP_INVOKESPECIAL)
			store1.stackConsumed = []values.JavaValue{items[1], indexes[1], ref}
			invoke.stackConsumed = []values.JavaValue{ref, receiver}
			path := []*OpCode{alloc, dup, idx0, val0, store0, dup2, idx1, branch, armA, armB, join, store1, invoke}
			opLink := func(a, b *OpCode) { a.Target = append(a.Target, b); b.Source = append(b.Source, a) }
			for i := 0; i < 7; i++ {
				opLink(path[i], path[i+1])
			}
			opLink(branch, armA)
			opLink(branch, armB)
			opLink(armA, join)
			opLink(armB, join)
			opLink(join, store1)
			opLink(store1, invoke)
			d := &Decompiler{RootNode: entry, FunctionContext: &class_context.ClassContext{FunctionName: "<init>", ClassName: "proof.Child", SupperClassName: "proof.Base"}, opCodes: path, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
			d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{}
			for _, op := range path {
				d.opcodeToSimulateStack[op] = nil
			}
			origins := map[int]*OpCode{2: dup, 5: store0, 8: branch, 12: store1, 13: invoke}
			want := change == "proved" || change == "this owner" || change == "literal primer string" || change == "wrapped proved" || change == "nested array component"
			switch change {
			case "nested array component", "nested incompatible component", "nested scalar component", "nested wrong rank":
				arrayType.RawType().(*types.JavaArrayType).Dimension = 2
				call.Descriptor = "([[Ljava/lang/String;)V"
				component := types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
				switch change {
				case "nested incompatible component":
					component = types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
				case "nested scalar component":
					component = types.NewJavaClass("java.lang.String")
				case "nested wrong rank":
					component = types.NewJavaArrayType(component)
				}
				item := values.NewJavaRef(utils.NewRootVariableId(), nil, component)
				item.IsParam = true
				items[0] = item
				store0.stackConsumed[0] = item
				storeNodes[0].Statement.(*statements.AssignStatement).JavaValue = item
				right.TrueValue = item
				right.FalseValue = item
			case "literal primer string":
				items[0].(*values.JavaLiteral).JavaType = types.NewJavaPrimer(types.JavaString)
			case "wrong owner":
				call.ClassName = "proof.Foreign"
			case "missing owner context":
				d.FunctionContext.ClassName = ""
			case "this owner":
				call.ClassName = "proof.Child"
			case "missing allocation PC":
				array.HasOriginPC = false
			case "missing store PC":
				storeNodes[1].Statement.(*statements.AssignStatement).HasOriginPC = false
			case "wrong index":
				indexes[1].Data = 0
			case "raw index mismatch":
				store1.stackConsumed[1] = literal(0)
			case "wrong RHS witness":
				store1.stackConsumed[0] = text("different")
			case "foreign element":
				items[0] = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
				store0.stackConsumed[0] = items[0]
				storeNodes[0].Statement.(*statements.AssignStatement).JavaValue = items[0]
			case "unknown hierarchy":
				items[0] = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Unknown"))
				store0.stackConsumed[0] = items[0]
				storeNodes[0].Statement.(*statements.AssignStatement).JavaValue = items[0]
			case "self alias":
				right.TrueValue = ref
			case "wrong parameter":
				call.FuncType.ParamTypes[0] = types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
			case "wrong descriptor":
				call.Descriptor = "([Ljava/lang/Object;)V"
			case "missing invoke binding":
				delete(d.invokeFuncCall, invoke)
			case "foreign receiver":
				receiver.IsThis = false
			case "wrong duplicate":
				dup.stackProduced[1] = values.JavaNull
			case "local publication":
				val0.Instr.OpCode = OP_ASTORE_2
				val0.stackConsumed = []values.JavaValue{ref}
			case "field publication":
				val0.Instr.OpCode = OP_PUTSTATIC
			case "extra call":
				val0.Instr.OpCode = OP_INVOKESTATIC
				d.invokeFuncCall[val0] = &values.FunctionCallExpression{FunctionName: "sideEffect", OriginPC: 4, HasOriginPC: true}
			case "RHS reused":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_2}, CurrentOffset: 20, stackConsumed: []values.JavaValue{ref}})
			case "back edge":
				armA.Target = []*OpCode{branch}
			case "external entry":
				store1.Source = append(store1.Source, op(0, OP_GOTO))
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 9, EndPc: 11, HandlerPc: 50}}
			case "unowned condition":
				right.ConditionFromOp = 99
			case "catch entry":
				cond.IsCatchStart = true
			case "post delegation reuse":
				exit.Statement = statements.NewReturnStatement(ref)
			case "source-side effect":
				jumpA.Statement = statements.NewExpressionStatement(&values.FunctionCallExpression{})
			case "effectful earlier argument":
				earlier := &values.FunctionCallExpression{FunctionName: "earlier", OriginPC: 14, HasOriginPC: true}
				call.Arguments = []values.JavaValue{earlier, ref}
				call.FuncType.ParamTypes = []types.JavaType{types.NewJavaClass("java.lang.Object"), arrayType}
				call.Descriptor = "(Ljava/lang/Object;[Ljava/lang/String;)V"
				invoke.stackConsumed = []values.JavaValue{ref, earlier, receiver}
			}
			consumer := call
			if strings.HasPrefix(change, "wrapped ") {
				copy := *call
				consumer = &copy
				consumer.Object = nil
				consumer.ClassName, consumer.FunctionName = "proof.OperandAdapter", "adapt"
				consumer.Kind, consumer.IsStatic, consumer.IsSpecialInvoke = values.InvokeStatic, true, false
				consumer.Descriptor = "([Ljava/lang/String;)Ljava/lang/String;"
				consumer.FuncType = &types.JavaFuncType{ParamTypes: []types.JavaType{arrayType}, ReturnType: types.NewJavaClass("java.lang.String")}
				invoke.Instr.OpCode = OP_INVOKESTATIC
				invoke.stackConsumed, invoke.stackProduced = []values.JavaValue{ref}, []values.JavaValue{consumer}
				initialization := op(14, OP_INVOKESPECIAL)
				initialization.stackConsumed = []values.JavaValue{consumer, receiver}
				opLink(invoke, initialization)
				call.Arguments = []values.JavaValue{consumer}
				call.Descriptor, call.OriginPC = "(Ljava/lang/String;)V", 14
				call.FuncType = &types.JavaFuncType{ParamTypes: []types.JavaType{types.NewJavaClass("java.lang.String")}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}
				origins[callNode.Id] = initialization
				d.opCodes = append(d.opCodes, initialization)
				d.invokeFuncCall[invoke], d.invokeFuncCall[initialization] = consumer, call
				switch change {
				case "wrapped wrong opcode":
					invoke.Instr.OpCode = OP_INVOKEVIRTUAL
				case "wrapped missing producer":
					delete(d.invokeFuncCall, invoke)
				case "wrapped wrong result identity":
					invoke.stackProduced[0] = text("replacement")
				case "wrapped alternate initialization entry":
					initialization.Source = append(initialization.Source, op(15, OP_GOTO))
				case "wrapped effect after producer":
					invoke.Target = []*OpCode{op(15, OP_INVOKESTATIC)}
				case "wrapped reused array":
					consumer.Arguments = []values.JavaValue{ref, ref}
				case "wrapped incompatible array":
					consumer.Descriptor = "([Ljava/lang/Object;)Ljava/lang/String;"
				case "wrapped wrong producer owner":
					original := *consumer
					original.ClassName = "proof.ForeignAdapter"
					d.invokeFuncCall[invoke] = &original
				case "wrapped missing origin":
					consumer.HasOriginPC = false
				case "wrapped wrong operand order":
					consumer.Arguments = []values.JavaValue{text("head"), ref}
					consumer.Descriptor = "(Ljava/lang/String;[Ljava/lang/String;)Ljava/lang/String;"
					consumer.FuncType.ParamTypes = []types.JavaType{types.NewJavaClass("java.lang.String"), arrayType}
					invoke.stackConsumed = []values.JavaValue{consumer.Arguments[0], ref}
				}
			}
			if got := d.inlinePrivateDelegationBranchArray(origins); got != want {
				t.Fatalf("accepted=%v want=%v", got, want)
			}
			if want {
				if d.RootNode != callNode || consumer.Arguments[len(consumer.Arguments)-1] != array || len(array.Initializer) != 2 || array.Initializer[1] != right || array.EvaluationEndPC != 12 {
					t.Fatal("lost operand identity/order or original store endpoint")
				}
			} else if d.RootNode != entry || len(array.Initializer) != 0 || array.HasEvaluationEndPC || consumer.Arguments[len(consumer.Arguments)-1] != ref || len(entry.Next) != 1 || entry.Next[0] != storeNodes[0] {
				t.Fatal("failed plan mutated shared graph or operands")
			}
		})
	}
}

func TestDelegationArrayElementProofIsBounded(t *testing.T) {
	stringType := types.NewJavaClass("java.lang.String")
	literal := values.NewJavaLiteral("x", stringType)
	if !delegationArrayElementAssignable(literal, "Ljava/lang/String;", nil) {
		t.Fatal("same source type must be assignable")
	}
	cycle := values.NewTernaryExpression(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), literal, literal)
	cycle.TrueValue = cycle
	if delegationArrayElementAssignable(cycle, "Ljava/lang/String;", nil) {
		t.Fatal("cyclic ternary must fail closed")
	}
	for i := 0; i < 260; i++ {
		literalTree := values.NewTernaryExpression(values.JavaNull, cycle, literal)
		cycle = literalTree
	}
	if delegationArrayElementAssignable(cycle, "Ljava/lang/String;", nil) {
		t.Fatal("unbounded tree must fail closed")
	}
}

func TestDelegationArrayEffectWitnessRequiresOnceAndOrderedStore(t *testing.T) {
	for _, scenario := range []string{"proved", "missing PC", "moved across store", "duplicate producer", "hidden call", "wrong binding", "unmatched new", "foreign origin"} {
		t.Run(scenario, func(t *testing.T) {
			call := &values.FunctionCallExpression{ClassName: "proof.Values", FunctionName: "produce", Descriptor: "()Ljava/lang/String;", Kind: values.InvokeStatic, IsStatic: true, OriginPC: 3, HasOriginPC: true}
			alloc := &OpCode{CurrentOffset: 1, Instr: &Instruction{OpCode: OP_ANEWARRAY}}
			invoke := &OpCode{CurrentOffset: 8, Instr: &Instruction{OpCode: OP_INVOKESPECIAL}}
			producer := &OpCode{CurrentOffset: 3, Instr: &Instruction{OpCode: OP_INVOKESTATIC}}
			store := &OpCode{CurrentOffset: 5, Instr: &Instruction{OpCode: OP_AASTORE}}
			sites := map[*OpCode]int{alloc: 0, producer: 0, store: 0, invoke: 1}
			stores := []*OpCode{store}
			items := []values.JavaValue{call}
			decoded := map[*OpCode]*values.FunctionCallExpression{producer: call}
			switch scenario {
			case "missing PC":
				call.HasOriginPC = false
			case "moved across store":
				call.OriginPC = 6
				producer.CurrentOffset = 6
			case "duplicate producer":
				items[0] = values.NewTernaryExpression(values.JavaNull, call, call)
			case "hidden call":
				sites[&OpCode{CurrentOffset: 4, Instr: &Instruction{OpCode: OP_INVOKESTATIC}}] = 0
			case "wrong binding":
				other := *call
				other.Descriptor = "()Ljava/lang/Object;"
				decoded[producer] = &other
			case "unmatched new":
				sites[&OpCode{CurrentOffset: 2, Instr: &Instruction{OpCode: OP_NEW}}] = 0
			case "foreign origin":
				delete(sites, producer)
			}
			if got := delegationArrayEffectSites(items, stores, sites, alloc, invoke, decoded); got != (scenario == "proved") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
