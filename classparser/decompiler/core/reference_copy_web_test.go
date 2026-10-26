package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestHandlerLocalCopyKeepsIndependentTypeFixedPoint(t *testing.T) {
	for _, kind := range []string{"copy", "missing producer", "missing source web", "same slot", "same web", "entry", "missing store", "snapshot owner", "handler source", "copied handler source", "unknown handler"} {
		t.Run(kind, func(t *testing.T) {
			str := types.NewJavaClass("java.lang.String")
			old := values.NewJavaRef(utils.NewRootVariableId(), nil, str)
			source := values.NewJavaRef(utils.NewRootVariableId(), nil, str)
			a, load, b, use := op(OP_ASTORE_1, 1), op(OP_ALOAD_2, 3), op(OP_ASTORE_1, 4), op(OP_ALOAD_1, 5)
			input, output := values.NewSlotValue(source, str), values.NewSlotValue(old, str)
			a.stackConsumed = []values.JavaValue{values.NewJavaLiteral("old", str)}
			load.stackProduced = []values.JavaValue{input}
			b.stackConsumed = []values.JavaValue{input}
			use.stackProduced = []values.JavaValue{output}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, load: 2, b: 3, use: 3}, entryWeb: map[int]int{}}
			sourceStore, handlerStore := op(OP_ASTORE_2, 2), op(OP_ASTORE_3, 6)
			sourceStore.stackConsumed = []values.JavaValue{values.NewJavaLiteral("source", str)}
			handlerStore.stackConsumed = []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, str)}
			webs.webOf[sourceStore], webs.webOf[handlerStore] = 2, 4
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, cachedSlotWebs: webs, opCodes: []*OpCode{a, sourceStore, load, b, use, handlerStore}, opcodeIdToRef: map[*OpCode][][2]any{a: {{old, true}}, b: {{old, false}}, sourceStore: {{source, true}}, handlerStore: {{values.NewJavaRef(utils.NewRootVariableId(), nil, str), true}}}, ExceptionTable: []*ExceptionTableEntry{{StartPc: 0, EndPc: 5, HandlerPc: 6}}}
			switch kind {
			case "handler source":
				d.ExceptionTable[0].HandlerPc = 2
			case "copied handler source":
				handlerLoad := op(OP_ALOAD_3, 7)
				handlerValue := values.NewSlotValue(values.NewJavaRef(utils.NewRootVariableId(), nil, str), str)
				handlerLoad.stackProduced = []values.JavaValue{handlerValue}
				sourceStore.stackConsumed = []values.JavaValue{handlerValue}
				d.opCodes = append(d.opCodes, handlerLoad)
				webs.webOf[handlerLoad] = 4
			case "unknown handler":
				d.ExceptionTable[0].HandlerPc = 99
			case "missing producer":
				load.stackProduced = nil
			case "missing source web":
				delete(webs.webOf, load)
			case "same slot":
				load.Instr = op(OP_ALOAD_1, 3).Instr
			case "same web":
				webs.webOf[load] = 3
			case "entry":
				webs.entryWeb[1] = 3
			case "missing store":
				webs.webOf[op(OP_ASTORE_1, 8)] = 3
			case "snapshot owner":
				d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{op(OP_INVOKEDYNAMIC, 9): {{Value: old, Operand: true}}}
			}
			d.partitionSharedReferenceWebs()
			copied := d.opcodeIdToRef[b][0][0].(*values.JavaRef)
			if (copied != old) != (kind == "copy") {
				t.Fatalf("independent copy=%v", copied != old)
			}
			if kind != "copy" {
				return
			}
			if copied.SolvedWebIdentity == nil || output.GetValue() != copied || input.GetValue() != source || d.opcodeIdToRef[a][0][0] != old {
				t.Fatal("copy partition changed producer/foreign lifetime or lost own load")
			}
			// The source's join is solved later: copying its provisional String
			// type must not pin this independent destination to String forever.
			source.ResetVarType(types.NewJavaClass("java.lang.Object"))
			source.WebDeclType = types.NewJavaClass("java.lang.Object")
			d.unifyReferenceWebs()
			if copied.Type().String(d.FunctionContext) != "Object" || old.Type().String(d.FunctionContext) != "String" || output.Type().String(d.FunctionContext) != "Object" {
				t.Fatal("copy failed to follow its source type without widening another lifetime")
			}
		})
	}
}

func TestReferenceJoinPublishesStableDeclarationIdentity(t *testing.T) {
	for _, kind := range []string{"join", "missing value", "primitive", "foreign owner"} {
		t.Run(kind, func(t *testing.T) {
			str, obj := types.NewJavaClass("java.lang.String"), types.NewJavaClass("java.lang.Object")
			a, b := op(OP_ASTORE_1, 1), op(OP_ASTORE_1, 2)
			first, second := values.NewJavaRef(utils.NewRootVariableId(), nil, str), values.NewJavaRef(utils.NewRootVariableId(), nil, obj)
			a.stackConsumed = []values.JavaValue{values.NewJavaLiteral("text", str)}
			b.stackConsumed = []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, obj)}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 1}, entryWeb: map[int]int{}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, cachedSlotWebs: webs, opCodes: []*OpCode{a, b}, opcodeIdToRef: map[*OpCode][][2]any{a: {{first, true}}, b: {{second, true}}}}
			switch kind {
			case "missing value":
				b.stackConsumed = nil
			case "primitive":
				b.stackConsumed = []values.JavaValue{values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger))}
			case "foreign owner":
				c := op(OP_ASTORE_1, 3)
				c.stackConsumed = a.stackConsumed
				d.opCodes = append(d.opCodes, c)
				webs.webOf[c] = 2
				d.opcodeIdToRef[c] = [][2]any{{first, false}}
			}
			d.unifyReferenceWebs()
			identity := first.SolvedWebIdentity
			if kind != "join" {
				if identity != nil || second.SolvedWebIdentity != nil {
					t.Fatal("incomplete/foreign definition acquired a solved join identity")
				}
				return
			}
			if identity == nil || second.SolvedWebIdentity != identity || first.Id != second.Id || first.VarUid != second.VarUid {
				t.Fatal("joined aliases lack one immutable naming authority")
			}
			d.unifyReferenceWebs()
			if first.SolvedWebIdentity != identity || second.SolvedWebIdentity != identity || first.Type().String(d.FunctionContext) != "Object" {
				t.Fatal("fixed point changed the identity or lost the joined type")
			}
		})
	}
}

func TestReferenceJoinIdentityDoesNotDependOnSimulatorAliasCount(t *testing.T) {
	for _, kind := range []string{"shared", "distinct", "null first", "three definitions", "foreign owner", "missing operand"} {
		t.Run(kind, func(t *testing.T) {
			str := types.NewJavaClass("java.lang.String")
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, str)
			other := ref
			if kind == "distinct" {
				other = values.NewJavaRef(utils.NewRootVariableId(), nil, str)
			}
			a, b := op(OP_ASTORE_1, 1), op(OP_ASTORE_1, 2)
			a.stackConsumed = []values.JavaValue{values.NewJavaLiteral("chosen", str)}
			b.stackConsumed = []values.JavaValue{values.JavaNull}
			if kind == "null first" {
				a.stackConsumed, b.stackConsumed = b.stackConsumed, a.stackConsumed
			}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 1}, entryWeb: map[int]int{}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, cachedSlotWebs: webs, opCodes: []*OpCode{a, b}, opcodeIdToRef: map[*OpCode][][2]any{a: {{ref, true}}, b: {{other, false}}}}
			if kind == "three definitions" || kind == "foreign owner" {
				c := op(OP_ASTORE_1, 3)
				c.stackConsumed = []values.JavaValue{values.NewJavaLiteral("third", str)}
				d.opCodes = append(d.opCodes, c)
				webs.webOf[c] = 1
				if kind == "foreign owner" {
					webs.webOf[c] = 2
				}
				d.opcodeIdToRef[c] = [][2]any{{ref, false}}
			}
			if kind == "missing operand" {
				b.stackConsumed = nil
			}
			d.unifyReferenceWebs()
			want := kind != "foreign owner" && kind != "missing operand"
			if (ref.SolvedWebIdentity != nil) != want || (other.SolvedWebIdentity != nil) != want {
				t.Fatal("join identity depended on alias count or incomplete definitions")
			}
			if !want {
				return
			}
			identity := ref.SolvedWebIdentity
			for _, store := range d.opCodes {
				bound := d.opcodeIdToRef[store][0][0].(*values.JavaRef)
				declaration := bound.CloneForSourceDeclaration()
				if bound.SolvedWebIdentity != identity || declaration.SolvedWebIdentity != identity || bound.Type().String(d.FunctionContext) != "String" {
					t.Fatal("declaration cloning lost solved ownership or reference type")
				}
			}
			d.unifyReferenceWebs()
			if ref.SolvedWebIdentity != identity {
				t.Fatal("second solver round changed naming authority")
			}
		})
	}
}
