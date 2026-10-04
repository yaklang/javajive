package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestBranchArrayInlineRejectsAlternateEntry(t *testing.T) {
	allocation := &OpCode{Instr: &Instruction{OpCode: OP_ANEWARRAY}, CurrentOffset: 1}
	fill := &OpCode{Instr: &Instruction{OpCode: OP_AASTORE}, CurrentOffset: 2}
	invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 3}
	allocation.Target, fill.Source = []*OpCode{fill}, []*OpCode{allocation}
	fill.Target, invoke.Source = []*OpCode{invoke}, []*OpCode{fill}
	d := &Decompiler{}
	if !branchArraySinglePath(d, allocation, invoke) {
		t.Fatal("the array initializer and its invocation form one linear path")
	}
	other := &OpCode{Instr: &Instruction{OpCode: OP_NOP}, CurrentOffset: 4, Target: []*OpCode{invoke}}
	invoke.Source = append(invoke.Source, other)
	if branchArraySinglePath(d, allocation, invoke) {
		t.Fatal("an alternate edge into the invocation must keep the allocation in place")
	}
}

func TestBranchArrayInertElementProof(t *testing.T) {
	literal := values.NewJavaLiteral("x", types.NewJavaClass("java.lang.String"))
	ref := &values.JavaRef{}
	for _, tc := range []struct {
		name  string
		value values.JavaValue
		want  bool
	}{
		{"literal", literal, true}, {"local", ref, true}, {"wrapped-local", values.NewSlotValue(ref, types.NewJavaClass("sample.Item")), true},
		{"nil", nil, false}, {"typed-nil-ref", (*values.JavaRef)(nil), false}, {"typed-nil-literal", (*values.JavaLiteral)(nil), false},
		{"call", &values.FunctionCallExpression{}, false}, {"array-read", &values.JavaArrayMember{}, false},
		{"field-read", &values.RefMember{}, false}, {"new-array", &values.NewExpression{}, false},
		{"opaque-local", &values.JavaRef{CustomValue: &values.CustomValue{}}, false},
		{"captured-local", &values.JavaRef{StackVar: ref}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := branchArrayInertElement(tc.value); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEarlierArrayOperandRequiresOriginalPrivatePrefix(t *testing.T) {
	for _, scenario := range []string{"proved", "pure", "later producer", "branch", "foreign value", "opaque", "handler boundary", "duplicate producer"} {
		t.Run(scenario, func(t *testing.T) {
			call := &values.FunctionCallExpression{IsStatic: true, ClassName: "Probe", FunctionName: "effect", FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("Token")}, Descriptor: "()LToken;"}
			producer, prefix, allocation := op(OP_INVOKESTATIC, 1), op(OP_ICONST_1, 2), op(OP_ANEWARRAY, 3)
			producer.stackProduced = []values.JavaValue{call}
			producer.Target, prefix.Source = []*OpCode{prefix}, []*OpCode{producer}
			prefix.Target, allocation.Source = []*OpCode{allocation}, []*OpCode{prefix}
			d := &Decompiler{opCodes: []*OpCode{producer, prefix, allocation}}
			var value values.JavaValue = call
			want := scenario == "proved" || scenario == "pure"
			switch scenario {
			case "pure":
				value = values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))
			case "later producer":
				producer.CurrentOffset = 4
			case "branch":
				producer.Target = append(producer.Target, op(OP_RETURN, 8))
			case "foreign value":
				copy := *call
				value = &copy
			case "opaque":
				value = &values.CustomValue{}
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 2, EndPc: 5, HandlerPc: 9}}
			case "duplicate producer":
				prefix.stackProduced = []values.JavaValue{call}
			}
			if got := d.branchOperandPrecedesArray(value, allocation); got != want {
				t.Fatalf("proved=%v want=%v", got, want)
			}
		})
	}
}

func TestPrivateArrayFillMustRemainUnobservableToHandler(t *testing.T) {
	for _, scenario := range []string{"private", "local store", "publish", "handler change", "alternate entry", "extra invocation", "foreign input", "parameter"} {
		t.Run(scenario, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaClass("example.Token")))
			array := values.NewNewExpression(ref.Type())
			array.OriginPC, array.HasOriginPC = 1, true
			allocation, dup, store, invoke := op(OP_ANEWARRAY, 1), op(OP_DUP, 2), op(OP_AASTORE, 3), op(OP_INVOKESTATIC, 4)
			all := []*OpCode{allocation, dup, store, invoke}
			for i := 0; i < len(all)-1; i++ {
				all[i].Target = []*OpCode{all[i+1]}
				all[i+1].Source = []*OpCode{all[i]}
			}
			dup.stackConsumed = []values.JavaValue{ref}
			store.stackConsumed = []values.JavaValue{values.JavaNull, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), ref}
			invoke.stackConsumed = []values.JavaValue{ref}
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{allocation: nil, dup: nil, store: nil, invoke: nil}, opCodes: all, ExceptionTable: []*ExceptionTableEntry{{StartPc: 1, EndPc: 5, HandlerPc: 9}}}
			first := &Node{Statement: statements.NewAssignStatement(ref, array, true)}
			last := &Node{}
			switch scenario {
			case "local store":
				dup.Instr.OpCode = OP_ASTORE
			case "publish":
				dup.Instr.OpCode = OP_PUTSTATIC
			case "handler change":
				d.ExceptionTable[0].StartPc = 2
			case "alternate entry":
				store.Source = append(store.Source, op(OP_NOP, 8))
			case "extra invocation":
				other := op(OP_INVOKESTATIC, 6)
				other.stackConsumed = []values.JavaValue{ref}
				d.opCodes = append(d.opCodes, other)
			case "foreign input":
				store.stackConsumed[0] = ref
			case "parameter":
				ref.IsParam = true
			}
			if got := d.privateArrayFillUnobservable(first, last, dup, store); got != (scenario == "private") {
				t.Fatalf("proved=%v", got)
			}
		})
	}
}

func TestThrownOperandParticipatesInPrivateArrayOwnership(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
	other := values.NewJavaRef(utils.NewRootVariableId(), nil, ref.Type())
	for _, tc := range []struct {
		name  string
		value values.JavaValue
		want  bool
	}{{"reads array", ref, true}, {"other local", other, false}, {"opaque", &values.CustomValue{}, true}} {
		t.Run(tc.name, func(t *testing.T) {
			st := &statements.CustomStatement{ThrownValue: tc.value}
			if got := statementReferencesLocal(st, ref); got != tc.want {
				t.Fatalf("reads=%v", got)
			}
		})
	}
}

func TestBranchArrayReturnWalkPreservesNestedCatchConsumer(t *testing.T) {
	for _, scenario := range []string{"nested catch", "multiple uses", "opaque", "live expression"} {
		t.Run(scenario, func(t *testing.T) {
			call := &values.FunctionCallExpression{ClassName: "example.Probe", FunctionName: "read", Descriptor: "()Ljava/lang/Object;", IsStatic: true, Object: values.NewJavaClassValue(types.NewJavaClass("example.Probe")), FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("java.lang.Object")}}
			returned := &statements.ReturnStatement{JavaValue: call}
			tr := &statements.TryCatchStatement{CatchBodies: [][]statements.Statement{{returned}}}
			body := []statements.Statement{tr}
			switch scenario {
			case "multiple uses":
				tr.TryBody = []statements.Statement{&statements.ReturnStatement{JavaValue: call}}
			case "opaque":
				tr.TryBody = []statements.Statement{&statements.CustomStatement{}}
			case "live expression":
				tr.CatchBodies[0] = []statements.Statement{&statements.ExpressionStatement{Expression: call}}
			}
			location, ok := findBranchArrayReturn(&body, call, &class_context.ClassContext{})
			if ok != (scenario == "nested catch") {
				t.Fatalf("proved=%v", ok)
			}
			if ok && (location == nil || (*location.list)[location.index] != returned) {
				t.Fatal("lost final consumer identity")
			}
		})
	}
}

func TestBranchArrayInlinePreservesExistingStaticArgumentView(t *testing.T) {
	arrayType := types.NewJavaArrayType(types.NewJavaClass("example.Item"))
	wide := types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
	array := values.NewNewExpression(arrayType)
	for _, kind := range []string{"exact", "erased generic parameter", "widened local", "widened declaration", "scalar parameter", "unknown parameter"} {
		t.Run(kind, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
			var parameter types.JavaType = arrayType
			switch kind {
			case "erased generic parameter":
				parameter = wide
			case "widened local":
				ref.ResetVarType(wide)
			case "widened declaration":
				ref.WebDeclType = wide
			case "scalar parameter":
				parameter = types.NewJavaClass("java.lang.Object")
			case "unknown parameter":
				parameter = nil
			}
			if got := branchArrayKeepsArgumentView(array, ref, parameter); got != (kind == "exact" || kind == "erased generic parameter") {
				t.Fatalf("view unchanged=%v", got)
			}
		})
	}
}
