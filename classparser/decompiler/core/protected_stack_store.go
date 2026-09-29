package core

import (
	"fmt"
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A stack value that joins a handler and the normal path cannot be represented
// by a conditional expression. Instrumenters often defer the local store until
// after that join. Recognize an edge on which each predecessor is an unconditional
// forward jump: moving the store to those edges preserves every execution path.
// More general stack joins need full stack-phi lowering, not a first-arm fallback.
func (d *Decompiler) isProtectedStackStore(store *OpCode) bool {
	if len(d.ExceptionTable) == 0 || store == nil || store.Instr == nil || !isLocalStoreOpcode(store.Instr.OpCode) ||
		store.IsCatch || store.IsTryCatchParent || len(store.Source) < 2 {
		return false
	}
	handler := false
	for _, pred := range store.Source {
		if pred == nil || pred.Instr == nil || (pred.Instr.OpCode != OP_GOTO && pred.Instr.OpCode != OP_GOTO_W) ||
			len(pred.Target) != 1 || pred.Target[0] != store || pred.CurrentOffset >= store.CurrentOffset || pred.IsTryCatchParent {
			return false
		}
		seen := map[*OpCode]bool{}
		for cur := pred; cur != nil && !seen[cur]; {
			seen[cur] = true
			if cur.IsCatch {
				for _, entry := range d.ExceptionTable {
					if entry.HandlerPc == cur.CurrentOffset {
						handler = true
					}
				}
				break
			}
			if len(cur.Source) != 1 || cur.Source[0].CurrentOffset >= cur.CurrentOffset {
				break
			}
			cur = cur.Source[0]
		}
	}
	return handler
}

// Materialize each incoming value on its own edge, with the same local identity
// used by post-join loads. No RHS is copied into another arm and the merged local
// must not be folded back to the representative value used during simulation.
func (d *Decompiler) lowerProtectedStackStores() (map[*OpCode]bool, map[*OpCode]*statements.AssignStatement, error) {
	stores := map[*OpCode]bool{}
	edges := map[*OpCode]*statements.AssignStatement{}
	for _, store := range d.opCodes {
		if !d.isProtectedStackStore(store) {
			continue
		}
		refs := d.opcodeIdToRef[store]
		if len(refs) != 1 {
			return nil, nil, fmt.Errorf("protected stack store at %d has no unique local", store.CurrentOffset)
		}
		ref, ok := refs[0][0].(*values.JavaRef)
		if !ok || ref == nil || refs[0][1] != true {
			return nil, nil, fmt.Errorf("protected stack store at %d reuses a live local", store.CurrentOffset)
		}
		var typ types.JavaType
		var provider types.SuperTypeProvider
		if d.FunctionContext != nil {
			provider = d.FunctionContext.SiblingSuperTypes
		}
		for _, pred := range store.Source {
			stack := pred.StackEntry
			if stack == nil || stack.depth != 1 || stack.value == nil || stack.value.Type() == nil {
				return nil, nil, fmt.Errorf("protected stack store at %d needs exactly one incoming value", store.CurrentOffset)
			}
			value := stack.value
			if !values.IsNullLiteral(value) {
				if typ == nil {
					typ = value.Type()
				} else if !reflect.DeepEqual(typ.RawType(), value.Type().RawType()) {
					typ = types.BridgedCommonSuperType(typ, value.Type(), provider)
					if typ == nil {
						return nil, nil, fmt.Errorf("protected stack store at %d needs a denotable value join", store.CurrentOffset)
					}
				}
			}
			edges[pred] = statements.NewAssignStatement(ref, value, false)
		}
		if typ != nil {
			ref.ResetVarType(typ)
		}
		stores[store] = true
		d.disFoldRef = append(d.disFoldRef, ref)
	}
	return stores, edges, nil
}
