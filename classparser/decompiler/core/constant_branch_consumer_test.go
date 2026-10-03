package core

import (
	"errors"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func constantBranchFixture(t *testing.T, code []byte) (*Decompiler, *SemanticCFG) {
	t.Helper()
	d := auditCFG(t, code)
	d.FunctionType = &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaVoid)}
	d.FunctionContext = &class_context.ClassContext{IsStatic: true}
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	d.semanticCFG = g
	return d, g
}
func TestConstantBranchConsumerUsesOriginalDefinitionsNotLastValue(t *testing.T) {
	for _, scenario := range []string{"one", "zero", "noncanonical", "parameter", "merged expression", "foreign ref", "missing load", "invalid graph", "wrong branch kind", "fake last value", "this", "custom ref", "stack ref", "wrapped", "excessive wrapper", "nil wrapper", "nil ref", "malformed constant"} {
		t.Run(scenario, func(t *testing.T) {
			bytecode := []byte{OP_ICONST_1, OP_ISTORE_0, OP_ILOAD_0, OP_IFEQ, 0, 4, OP_RETURN, OP_RETURN}
			if scenario == "zero" {
				bytecode[0] = OP_ICONST_0
			}
			if scenario == "noncanonical" {
				bytecode[0] = OP_ICONST_2
			}
			d, g := constantBranchFixture(t, bytecode)
			load, branch := g.Nodes[2], g.Nodes[3]
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			load.stackProduced = []values.JavaValue{ref}
			branch.stackConsumed = []values.JavaValue{ref}
			switch scenario {
			case "parameter":
				ref.IsParam = true
			case "merged expression":
				branch.stackConsumed[0] = values.NewBinaryExpression(ref, values.NewJavaLiteral(1, ref.Type()), values.AND, ref.Type())
			case "foreign ref":
				branch.stackConsumed[0] = values.NewJavaRef(utils.NewRootVariableId(), nil, ref.Type())
			case "missing load":
				load.stackProduced = nil
			case "invalid graph":
				g.Err = errors.New("incomplete original CFG")
			case "wrong branch kind":
				copy := *branch.Instr
				branch.Instr = &copy
				branch.Instr.OpCode = OP_IFLT
			case "this":
				ref.IsThis = true
			case "custom ref":
				ref.CustomValue = &values.CustomValue{}
			case "stack ref":
				ref.StackVar = values.NewJavaLiteral(1, ref.Type())
			case "wrapped":
				branch.stackConsumed[0] = values.NewSlotValue(ref, ref.Type())
				load.stackProduced[0] = values.NewSlotValue(ref, ref.Type())
			case "excessive wrapper":
				var slot values.JavaValue = ref
				for i := 0; i < 40; i++ {
					slot = values.NewSlotValue(slot, ref.Type())
				}
				branch.stackConsumed[0] = slot
			case "nil wrapper":
				branch.stackConsumed[0] = (*values.SlotValue)(nil)
			case "nil ref":
				branch.stackConsumed[0] = (*values.JavaRef)(nil)
			case "malformed constant":
				g.Nodes[0].Data = []byte{1}
			case "fake last value":
				ref.Val = values.NewJavaLiteral(0, ref.Type())
			}
			original := branch.stackConsumed[0]
			before, entry := g.ReachingDefinitions(load, 0)
			d.restoreConstantBranchConsumers()
			literal, changed := branch.stackConsumed[0].(*values.JavaLiteral)
			want := scenario == "one" || scenario == "zero" || scenario == "fake last value" || scenario == "wrapped"
			if changed != want {
				t.Fatalf("projected=%v want=%v", changed, want)
			}
			if changed && literal.Data != (scenario != "zero") {
				t.Fatal("wrong original constant", literal.Data)
			}
			if !changed && branch.stackConsumed[0] != original {
				t.Fatal("changed unproved consumer")
			}
			after, e := g.ReachingDefinitions(load, 0)
			if e != entry || len(after) != len(before) {
				t.Fatal("changed original reaching definitions")
			}
			for i := range after {
				if after[i] != before[i] {
					t.Fatal("changed original reaching identity")
				}
			}
			if load.stackProduced != nil && constantBranchOperand(load.stackProduced[0]) != ref {
				t.Fatal("changed shared load/ref")
			}
		})
	}
}
func TestConstantBranchKeepsExceptionalBeforeState(t *testing.T) {
	code := []byte{OP_ICONST_0, OP_ISTORE_0, OP_INVOKESTATIC, 0, 1, OP_ICONST_1, OP_ISTORE_0, OP_ILOAD_0, OP_IFEQ, 0, 4, OP_RETURN, OP_RETURN, OP_ASTORE_1, OP_ILOAD_0, OP_IFNE, 0, 4, OP_RETURN, OP_RETURN}
	d := auditCFG(t, code)
	d.FunctionType = &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaVoid)}
	d.FunctionContext = &class_context.ClassContext{IsStatic: true}
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 2, EndPc: 7, HandlerPc: 13, CatchType: 1}}
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	d.semanticCFG = g
	at := func(pc uint16) *OpCode { return d.opCodes[d.offsetToOpcodeIndex[pc]] }
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
	for _, pcs := range [][2]uint16{{7, 8}, {14, 15}} {
		at(pcs[0]).stackProduced = []values.JavaValue{ref}
		at(pcs[1]).stackConsumed = []values.JavaValue{ref}
	}
	d.restoreConstantBranchConsumers()
	if v, ok := at(8).stackConsumed[0].(*values.JavaLiteral); !ok || v.Data != true {
		t.Fatal("normal state lost")
	}
	if v, ok := at(15).stackConsumed[0].(*values.JavaLiteral); !ok || v.Data != false {
		t.Fatal("throwing producer committed later store")
	}
}

func TestConstantBranchJoinsAndIncrementStayConservative(t *testing.T) {
	for _, scenario := range []string{"equal definitions", "different definitions", "entry path", "increment", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			code := []byte{OP_ILOAD_1, OP_IFEQ, 0, 8, OP_ICONST_1, OP_ISTORE_0, OP_GOTO, 0, 5, OP_ICONST_1, OP_ISTORE_0, OP_ILOAD_0, OP_IFEQ, 0, 4, OP_RETURN, OP_RETURN}
			loadPC, branchPC := uint16(11), uint16(12)
			switch scenario {
			case "different definitions":
				code[9] = OP_ICONST_0
			case "entry path":
				code[3] = 10
			case "increment":
				code = []byte{OP_ICONST_1, OP_ISTORE_0, OP_IINC, 0, 1, OP_ILOAD_0, OP_IFEQ, 0, 4, OP_RETURN, OP_RETURN}
				loadPC, branchPC = 5, 6
			}
			d, g := constantBranchFixture(t, code)
			load := d.opCodes[d.offsetToOpcodeIndex[loadPC]]
			branch := d.opCodes[d.offsetToOpcodeIndex[branchPC]]
			ref := values.NewJavaRef(utils.NewRootVariableId(), values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), types.NewJavaPrimer(types.JavaBoolean))
			load.stackProduced = []values.JavaValue{ref}
			branch.stackConsumed = []values.JavaValue{ref}
			if scenario == "budget" {
				g.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			d.restoreConstantBranchConsumers()
			lit, got := branch.stackConsumed[0].(*values.JavaLiteral)
			if want := scenario == "equal definitions"; got != want {
				t.Fatalf("projection=%v want=%v", got, want)
			}
			if got && lit.Data != true {
				t.Fatal("lost join truth")
			}
			if load.stackProduced[0] != ref || ref.Val == nil {
				t.Fatal("changed original value")
			}
			if scenario == "budget" && g.Err == nil {
				t.Fatal("budget failure hidden")
			}
		})
	}
}
