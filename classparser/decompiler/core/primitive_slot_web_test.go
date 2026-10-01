package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Boolean reconstruction happens after stores have been simulated with the
// JVM's int computational type. A later length must not share the first ref,
// or recovering the flag's boolean type also turns the array length boolean.
func TestPrimitiveSlotStoresKeepDisjointIdentitiesBeforeTypeRecovery(t *testing.T) {
	for _, joined := range []bool{false, true} {
		first, read, next, last := op(OP_ISTORE_1, 1), op(OP_ILOAD_1, 2), op(OP_ISTORE_1, 3), op(OP_ILOAD_1, 4)
		read.Source = []*OpCode{first}
		next.Source = []*OpCode{read}
		last.Source = []*OpCode{next}
		if joined {
			last.Source = append(last.Source, first)
		}
		first.Target = []*OpCode{read}
		read.Target = []*OpCode{next}
		next.Target = []*OpCode{last}
		if joined {
			first.Target = append(first.Target, last)
		}
		d := NewDecompiler(nil, nil)
		d.opCodes = []*OpCode{first, read, next, last}
		d.FunctionType = types.NewJavaFuncType("()V", nil, types.NewJavaPrimer(types.JavaVoid))
		sim := NewStackSimulation(NewEmptyStackEntry(), map[int]*values.JavaRef{}, utils.NewRootVariableId())
		sim.Push(values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
		if err := d.calcOpcodeStackInfo(sim, first); err != nil {
			t.Fatal(err)
		}
		flag := sim.GetVar(1)
		invoke := op(OP_INVOKESTATIC, 2)
		invoke.stackConsumed = []values.JavaValue{values.NewSlotValue(flag, flag.Type())}
		d.invokeFuncCall[invoke] = &values.FunctionCallExpression{FuncType: types.NewJavaFuncType("(Z)V", []types.JavaType{types.NewJavaPrimer(types.JavaBoolean)}, types.NewJavaPrimer(types.JavaVoid))}
		sim.Push(values.NewJavaLiteral(12, types.NewJavaPrimer(types.JavaInteger)))
		if err := d.calcOpcodeStackInfo(sim, next); err != nil {
			t.Fatal(err)
		}
		length := sim.GetVar(1)
		if values.SameLocal(flag, length) != joined {
			t.Fatalf("stores share identity=%v, joined=%v, first=%s/%s next=%s/%s webs=%v", values.SameLocal(flag, length), joined, flag.VarUid, flag.Type().String(d.FunctionContext), length.VarUid, length.Type().String(d.FunctionContext), d.slotWebs().webOf)
		}
		if !joined {
			flag.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
			if p, ok := length.Type().RawType().(*types.JavaPrimer); !ok || p.Name != types.JavaInteger {
				t.Fatal("late boolean recovery corrupted a disjoint length")
			}
		}
	}
}

func TestPrimitiveStoreDisjointWebProof(t *testing.T) {
	for _, kind := range []string{"disjoint", "same web", "shared identity", "missing target", "missing owner", "unknown identity", "reference", "ambiguous zero", "ambiguous one", "numeric sink", "numeric comparison", "missing operand", "parameter reuse", "parameter phi", "unknown parameter"} {
		t.Run(kind, func(t *testing.T) {
			first, next := op(OP_ISTORE_1, 1), op(OP_ISTORE_1, 3)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
			value := values.NewJavaLiteral(12, types.NewJavaPrimer(types.JavaInteger))
			d := &Decompiler{cachedSlotWebs: &slotWeb{webOf: map[*OpCode]int{first: 1, next: 2}, entryWeb: map[int]int{0: 1}}, opcodeIdToRef: map[*OpCode][][2]any{first: {{ref, true}}}}
			invoke := op(OP_INVOKESTATIC, 2)
			invoke.stackConsumed = []values.JavaValue{values.NewSlotValue(ref, ref.Type())}
			call := &values.FunctionCallExpression{FuncType: types.NewJavaFuncType("(Z)V", []types.JavaType{types.NewJavaPrimer(types.JavaBoolean)}, types.NewJavaPrimer(types.JavaVoid))}
			d.invokeFuncCall = map[*OpCode]*values.FunctionCallExpression{invoke: call}
			switch kind {
			case "same web":
				d.cachedSlotWebs.webOf[next] = 1
			case "shared identity":
				third := op(OP_ISTORE_1, 2)
				d.cachedSlotWebs.webOf[third] = 2
				d.opcodeIdToRef[third] = [][2]any{{ref, false}}
			case "missing target":
				delete(d.cachedSlotWebs.webOf, next)
			case "missing owner":
				delete(d.cachedSlotWebs.webOf, first)
			case "unknown identity":
				d.opcodeIdToRef = nil
			case "reference":
				value = values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object"))
			case "ambiguous zero":
				value = values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
			case "ambiguous one":
				value = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "numeric sink":
				call.FuncType.ParamTypes[0] = types.NewJavaPrimer(types.JavaInteger)
			case "numeric comparison":
				invoke.stackConsumed[0] = values.NewBinaryExpression(ref, value, values.NE, types.NewJavaPrimer(types.JavaBoolean))
			case "missing operand":
				invoke.stackConsumed = nil
			case "parameter reuse", "parameter phi", "unknown parameter":
				ref.IsParam = true
				d.opcodeIdToRef = nil
				d.Params = []values.JavaValue{ref}
				if kind == "parameter phi" {
					d.cachedSlotWebs.entryWeb[0] = 2
				} else if kind == "unknown parameter" {
					delete(d.cachedSlotWebs.entryWeb, 0)
				}
			}
			want := kind == "disjoint" || kind == "parameter reuse"
			if got := d.primitiveStoreStartsDisjointWeb(next, ref, value); got != want {
				t.Fatalf("disjoint proof=%v want=%v", got, want)
			}
		})
	}
}
