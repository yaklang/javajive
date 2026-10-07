package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
)

// A nested join still contains its first DFS arm until expression planning
// finishes. Its temporary Type is therefore not evidence for an enclosing
// statement phi. Read every original incoming edge instead, without changing
// instruction descriptor types or expression/slot caches. Shared joins are
// memoized; incomplete or cyclic evidence fails before any edge is published.
func (d *Decompiler) closedStackPhiValueType(value values.JavaValue, provider types.SuperTypeProvider) (types.JavaType, bool) {
	if d.Work.CheckAlloc(512*128) != nil {
		return nil, false
	}
	memo := map[values.JavaValue]types.JavaType{}
	active := map[values.JavaValue]bool{}
	var solve func(values.JavaValue, int) (types.JavaType, bool)
	join := func(a, b types.JavaType) types.JavaType {
		if a == nil {
			return b
		}
		if b == nil {
			return a
		}
		if reflect.DeepEqual(a.RawType(), b.RawType()) {
			return a.Copy()
		}
		return types.BridgedCommonSuperType(a, b, provider)
	}
	solve = func(v values.JavaValue, depth int) (types.JavaType, bool) {
		if v == nil || reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil() || depth > 128 || active[v] || d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return nil, false
		}
		if typ, found := memo[v]; found {
			return typ, true
		}
		if len(memo)+len(active) >= 512 {
			return nil, false
		}
		active[v] = true
		defer delete(active, v)
		var typ types.JavaType
		if slot, ok := v.(*values.SlotValue); ok {
			if merge := d.stackPhiSources[slot]; merge != nil {
				if len(merge.Source) < 2 || len(merge.Source) > 512 || d.Work.Charge(workbudget.CounterGraphEdges, int64(len(merge.Source))) != nil {
					return nil, false
				}
				for _, pred := range merge.Source {
					if pred == nil || pred.StackEntry == nil || !containsPhiSuccessor(pred, merge) {
						return nil, false
					}
					incoming, valid := solve(pred.StackEntry.value, depth+1)
					if !valid {
						return nil, false
					}
					previous := typ
					typ = join(typ, incoming)
					if previous != nil && incoming != nil && typ == nil {
						return nil, false
					}
				}
			} else {
				var valid bool
				typ, valid = solve(slot.GetValue(), depth+1)
				if !valid {
					return nil, false
				}
			}
		} else if ref, ok := v.(*values.JavaRef); ok && d.singleStackPhiDefinition(ref) != nil {
			var valid bool
			typ, valid = solve(ref.Val, depth+1)
			if !valid {
				return nil, false
			}
		} else if tern, ok := v.(*values.TernaryExpression); ok {
			if d.Work.Charge(workbudget.CounterGraphEdges, 2) != nil {
				return nil, false
			}
			left, valid := solve(tern.TrueValue, depth+1)
			if !valid {
				return nil, false
			}
			right, valid := solve(tern.FalseValue, depth+1)
			if !valid {
				return nil, false
			}
			typ = join(left, right)
			if left != nil && right != nil && typ == nil {
				return nil, false
			}
		} else if v != values.JavaNull && !values.IsNullLiteral(v) {
			typ = values.TernaryArmRValueType(v)
			if typ == nil {
				return nil, false
			}
			typ = typ.Copy()
			if raw, ok := typ.RawType().(*types.JavaPrimer); ok && raw.Name == types.JavaString {
				typ = types.NewJavaClass("java.lang.String")
			}
		}
		memo[v] = typ
		return typ, true
	}
	return solve(value, 0)
}

func containsPhiSuccessor(pred, merge *OpCode) bool {
	for _, next := range pred.Target {
		if next == merge {
			return true
		}
	}
	return false
}

// This index is populated from the original decoded definitions, never names.
func (d *Decompiler) singleStackPhiDefinition(ref *values.JavaRef) *values.SlotValue {
	if ref == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.WebDeclType != nil {
		return nil
	}
	seed := d.stackPhiDefinitions[ref]
	if seed == nil || ref.Val != seed {
		return nil
	}
	return seed
}

func (d *Decompiler) indexStackPhiDefinitions() bool {
	type definition struct {
		ref   *values.JavaRef
		op    *OpCode
		first bool
		count int
	}
	definitions := map[string]definition{}
	for op, rows := range d.opcodeIdToRef {
		if d.Work.CheckAlloc(int64(len(definitions)+len(rows))*128) != nil {
			return false
		}
		for _, row := range rows {
			if d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
				return false
			}
			ref, ok := row[0].(*values.JavaRef)
			if !ok || ref == nil || ref.VarUid == "" {
				continue
			}
			record := definitions[ref.VarUid]
			record.count++
			record.ref, record.op = ref, op
			record.first, _ = row[1].(bool)
			definitions[ref.VarUid] = record
		}
	}
	index := map[*values.JavaRef]*values.SlotValue{}
	for _, record := range definitions {
		ref, op := record.ref, record.op
		if record.count != 1 || !record.first || op == nil || op.Instr == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.WebDeclType != nil {
			continue
		}
		seed, ok := ref.Val.(*values.SlotValue)
		if !ok || d.stackPhiSources[seed] == nil {
			continue
		}
		known := d.refToCreatingStore[ref] == op && GetStoreIdx(op) >= 0 && len(op.stackConsumed) == 1 && op.stackConsumed[0] == seed
		if pc, kind, witness := ref.OriginalStackMaterializationWitness(seed); witness && pc == int(op.CurrentOffset) && kind == op.Instr.OpCode {
			switch kind {
			case OP_DUP, OP_DUP_X1, OP_DUP_X2, OP_DUP2, OP_DUP2_X1, OP_DUP2_X2:
				for _, original := range d.dupConvertedRefValue[op] {
					if original == seed {
						known = true
					}
				}
			}
		}
		if known {
			index[ref] = seed
		}
	}
	d.stackPhiDefinitions = index
	return true
}

func (d *Decompiler) refineClosedStackPhiDefinitions() {
	if d.FunctionContext == nil || len(d.stackPhiSources) == 0 || !d.indexStackPhiDefinitions() {
		return
	}
	provider := d.FunctionContext.SiblingSuperTypes
	plans := map[*values.JavaRef]types.JavaType{}
	for ref := range d.stackPhiDefinitions {
		seed := d.singleStackPhiDefinition(ref)
		if seed == nil {
			continue
		}
		typ, valid := d.closedStackPhiValueType(seed, provider)
		oldName, oldClass := types.RawClassFQN(ref.Type())
		newName, newClass := types.RawClassFQN(typ)
		if !valid || !oldClass || !newClass || !types.IsReferenceSubtypeBridged(oldName, newName, provider) {
			continue
		}
		plans[ref] = typ
	}
	if d.Work.Check() != nil {
		return
	}
	// Complete all queries before changing declaration types. Original producer
	// descriptors remain untouched, including calls used as conditional arms.
	d.stackPhiTypes = map[*values.SlotValue]types.JavaType{}
	for ref, typ := range plans {
		seed := ref.Val.(*values.SlotValue)
		ref.ResetVarType(typ.Copy())
		ref.WebDeclType = typ.Copy()
		seed.TmpType = typ.Copy()
		d.stackPhiTypes[seed] = typ.Copy()
	}
}

func (d *Decompiler) resetClosedStackPhiValue(slot *values.SlotValue, tern *values.TernaryExpression) {
	if typ := d.stackPhiTypes[slot]; typ != nil {
		// The conditional owns its cache. Sharing an arm's type wrapper here
		// would overwrite a call/parameter type when ResetValue propagates it.
		tern.SetCachedType(typ.Copy())
	}
	slot.ResetValue(tern)
}
