package core

import (
	"reflect"
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// constructorDelegationOffset returns the PC of the first super()/this()
// invokespecial in an ordinary constructor, or -1 when the method is not a
// constructor or no direct delegation exists. Object construction inside the
// delegation arguments is deliberately not treated as the delegation.
func (d *Decompiler) constructorDelegationOffset() int {
	if d == nil || d.FunctionContext == nil || d.FunctionContext.FunctionName != "<init>" {
		return -1
	}
	if d.constructorDelegationKnown {
		return d.constructorDelegationPC
	}
	d.constructorDelegationKnown = true
	d.constructorDelegationPC = -1
	self := strings.ReplaceAll(d.FunctionContext.ClassName, ".", "/")
	super := strings.ReplaceAll(d.CodeLimits.DirectSuperClass, ".", "/")
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKESPECIAL || len(op.Data) != 2 {
			continue
		}
		member := d.GetMethodFromPool(int(Convert2bytesToInt(op.Data)))
		if member == nil || member.Member != "<init>" {
			continue
		}
		owner := strings.ReplaceAll(member.Name, ".", "/")
		if owner == self || (super != "" && owner == super) {
			d.constructorDelegationPC = int(op.CurrentOffset)
			break
		}
	}
	return d.constructorDelegationPC
}

// A switch expression leaves its selected value on the operand stack at the
// join shared by every arm. The same shape appears in ordinary conditional
// expression joins that are not ternary diamonds (for example a switch whose
// cases all push a value and fall through to one `return`). The first-source
// stack snapshot used for ordinary statement merges is not a value merge: it
// would silently return only one arm's value. Materialize the joined word in a
// synthetic local and write the incoming edge value before the transfer, so the
// consumer after the join reads the selected arm.
//
// This is deliberately narrower than general JVM stack-map reconstruction:
// only a forward, acyclic join whose sources have exactly one successor and a
// provably unchanged stack prefix is admitted. Loop backedges and
// exception-handler edges stay on the existing conservative path. A protected
// try body is allowed because each new store is emitted at its original
// predecessor PC, inside the same handler coverage as the value it carries.
func (d *Decompiler) prepareAcyclicStackMergeSlot(merge *OpCode, sources []*OpCode, sim *StackSimulationImpl) *values.SlotValue {
	if d == nil || merge == nil || sim == nil || len(sources) == 0 || len(merge.Source) < 2 ||
		merge.Instr == nil || sim.Size() == 0 || merge.IsCatch || merge.IsTryCatchParent || IsSwitchOpcode(merge.Instr.OpCode) {
		return nil
	}
	// Constructor prefixes (the super()/this() call and its arguments) cannot
	// declare or assign a synthetic local before the delegation. A non-ternary
	// stack join there (for example a switch expression used as a delegation
	// argument) has no representable statement form, so fail the method closed
	// instead of emitting cases that silently lose their value. Joins in the
	// constructor body are ordinary locals and use the normal phi path.
	if d.FunctionContext != nil && d.FunctionContext.FunctionName == "<init>" {
		if delegation := d.constructorDelegationOffset(); delegation < 0 || int(merge.CurrentOffset) <= delegation {
			d.unsupportedConstructorPrefix = "constructor prefix stack join cannot be expressed before super()/this()"
			return nil
		}
	}
	if d.acyclicPhiHasProtectedJoin(merge) && !d.commonSwitchAncestor(merge) {
		// A protected normal/catch merge belongs to the established protected
		// stack-store lowering. Only a switch expression join (every edge from
		// one switch) is safe to materialize here.
		return nil
	}
	if acyclicPhiHasCatchAncestor(merge) {
		// One incoming edge comes from a catch handler. The try structurer
		// owns that merge; do not synthesize edge stores.
		return nil
	}
	baseOffset := merge.CurrentOffset
	for _, source := range merge.Source {
		if source == nil || source.Instr == nil ||
			source.CurrentOffset >= baseOffset || len(source.Target) != 1 || source.Target[0] != merge ||
			source.IsCatch || source.IsTryCatchParent {
			return nil
		}
	}
	top := sim.Peek()
	if top == nil || top.Type() == nil {
		return nil
	}
	return values.NewSlotValue(top, top.Type())
}

// lowerAcyclicStackMerges runs after stack simulation, when every opcode has a
// simulator and the outgoing edge assignment can be represented as a statement.
// Deeper merges are processed first so a nested switch expression resolves to a
// real local before an enclosing join consumes it.
func (d *Decompiler) lowerAcyclicStackMerges(candidates map[*OpCode]*values.SlotValue) {
	if len(candidates) == 0 {
		return
	}
	merges := make([]*OpCode, 0, len(candidates))
	for merge := range candidates {
		merges = append(merges, merge)
	}
	sort.Slice(merges, func(i, j int) bool {
		if merges[i].CurrentOffset != merges[j].CurrentOffset {
			return merges[i].CurrentOffset > merges[j].CurrentOffset
		}
		return merges[i].Id > merges[j].Id
	})
	for _, merge := range merges {
		slot := candidates[merge]
		d.lowerAcyclicStackPhi(merge, slot)
	}
}

func (d *Decompiler) lowerAcyclicStackPhi(merge *OpCode, slot *values.SlotValue) bool {
	if d == nil || merge == nil || slot == nil || len(merge.Source) < 2 || len(merge.Source) > 512 ||
		merge.Instr == nil || merge.IsCatch || merge.IsTryCatchParent {
		return false
	}
	sim := d.opcodeToSimulateStack[merge]
	if sim == nil {
		return false
	}
	prefix := merge.Source[0].StackEntry
	if prefix == nil {
		return false
	}
	prefix = prefix.parent
	incoming := make(map[*OpCode]values.JavaValue, len(merge.Source))
	var first values.JavaValue
	allSame := true
	var typ types.JavaType
	provider := types.SuperTypeProvider(nil)
	if d.FunctionContext != nil {
		provider = d.FunctionContext.SiblingSuperTypes
	}
	for _, pred := range merge.Source {
		if pred == nil || pred.Instr == nil || pred.StackEntry == nil || pred.StackEntry.parent == nil ||
			pred.CurrentOffset >= merge.CurrentOffset || len(pred.Target) != 1 || pred.Target[0] != merge ||
			pred.IsCatch || pred.IsTryCatchParent {
			return false
		}
		if !d.sameStackLifetimePrefix(prefix, pred.StackEntry.parent) {
			return false
		}
		value := pred.StackEntry.value
		if value == nil || value.Type() == nil {
			return false
		}
		incoming[pred] = value
		unwrapped := values.OriginalStackLifetimeUse(value)
		if first == nil {
			first = value
		} else if unwrapped == nil || unwrapped != values.OriginalStackLifetimeUse(first) {
			allSame = false
		}
		// Null is assignable to every reference type and contributes no
		// declaration type. A switch expression such as
		// `case 0 -> null; case 1 -> "one"` must use the non-null arm type,
		// not Object (which would make the shared return ill-typed).
		if u := values.UnpackSoltValue(value); u == values.JavaNull || values.IsNullLiteral(u) {
			continue
		}
		incomingType := values.TernaryArmRValueType(value)
		if incomingType == nil {
			return false
		}
		incomingType = incomingType.Copy()
		if primer, ok := incomingType.RawType().(*types.JavaPrimer); ok && primer.Name == types.JavaString {
			incomingType = types.NewJavaClass("java.lang.String")
		}
		if typ == nil {
			typ = incomingType
		} else if !reflect.DeepEqual(typ.RawType(), incomingType.RawType()) {
			typ = types.BridgedCommonSuperType(typ, incomingType, provider)
		}
		if typ == nil {
			return false
		}
	}
	if typ == nil {
		// Every incoming value is null. Any reference declaration can hold it;
		// Object is the least-specific legal sink.
		typ = types.NewJavaClass("java.lang.Object")
	}
	if allSame {
		// Every edge carries the same already-evaluated word; no phi is needed.
		return false
	}
	ref := sim.NewVar(values.NewJavaLiteral(nil, typ))
	ref.ResetVarType(typ)
	ref.WebDeclType = typ.Copy()
	ref.MarkStackMergePhi()
	slot.ResetValue(ref)
	if d.effectfulStackPhiEdges == nil {
		d.effectfulStackPhiEdges = map[*OpCode]*statements.AssignStatement{}
	}
	for pred, value := range incoming {
		assign := statements.NewAssignStatement(ref, value, false)
		assign.OriginPC, assign.HasOriginPC = int(pred.CurrentOffset), true
		d.effectfulStackPhiEdges[pred] = assign
		if incomingRef, ok := values.UnpackSoltValue(value).(*values.JavaRef); ok && incomingRef != nil {
			d.disFoldRef = append(d.disFoldRef, incomingRef)
		}
		if pred.Instr != nil && pred.Instr.OpCode == OP_CHECKCAST {
			for _, produced := range pred.stackProduced {
				if producer, ok := values.UnpackSoltValue(produced).(*values.JavaRef); ok && producer != nil {
					d.disFoldRef = append(d.disFoldRef, producer)
				}
			}
		}
	}
	d.disFoldRef = append(d.disFoldRef, ref)
	return true
}
