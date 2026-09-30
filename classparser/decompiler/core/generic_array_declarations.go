package core

import (
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// genericArrayOperand inspects only stack forwarding. A ref's initializer is
// not its current value, and an opaque/cyclic wrapper supplies no type proof.
func genericArrayOperand(value values.JavaValue) values.JavaValue {
	for depth := 0; depth < 8; depth++ {
		if slot, ok := value.(*values.SlotValue); ok && slot != nil {
			value = slot.GetValue()
			continue
		}
		return value
	}
	return nil
}

// The source formals are read from this method's exact descriptor-keyed
// Signature. Parameter refs are matched by identity, not slot/name/erasure.
func (d *Decompiler) genericArrayParameterTypes() map[*values.JavaRef]types.JavaType {
	ctx := d.FunctionContext
	if ctx == nil || d.FunctionType == nil {
		return nil
	}
	sig := ctx.MethodSignatureByDesc(ctx.FunctionName, ctx.CurrentMethodDesc)
	if sig == "" || len(sig) > 8192 {
		return nil
	}
	_, formals, ret := types.ParseMethodSignatureFull(sig, ctx)
	offset := 0
	if !ctx.IsStatic {
		offset = 1
	}
	if ret == nil || len(formals) == 0 || len(formals) > 64 || len(formals)+offset != len(d.Params) || len(d.Params) != len(d.FunctionType.ParamTypes) {
		return nil
	}
	// This scanner excludes the method's own formals. The remaining variables
	// must belong to an instance declaration; a static method cannot use them.
	for _, name := range types.TypeVarRefsInMethodParams(sig) {
		_, erased := ctx.StandaloneEraseTypeVar(name)
		if !ctx.IsTypeParam(name) || ctx.RawEraseTypeVar(name) || erased || ctx.IsStatic {
			return nil
		}
	}
	result := map[*values.JavaRef]types.JavaType{}
	for i, typ := range formals {
		ref, ok := d.Params[i+offset].(*values.JavaRef)
		p, generic := types.AsParameterizedType(typ)
		if !ok || ref == nil || !ref.IsParam || ref.IsThis || ref.StackVar != nil || ref.CustomValue != nil || !generic || len(p.TypeArgs) == 0 {
			continue
		}
		raw, valid := types.RawClassFQN(d.FunctionType.ParamTypes[i+offset])
		if !valid || raw != p.RawClassName {
			continue
		}
		result[ref] = typ
	}
	return result
}

// An array allocation remains raw/reifiable. Only a single-definition local
// declaration can acquire a parameterized element type, and only when every
// initializer item has exactly that source formal type. This preserves factory
// inference without emitting illegal `new Input<T>[]` or changing allocation.
func genericArrayElementType(array *values.NewExpression, params map[*values.JavaRef]types.JavaType) types.JavaType {
	if array == nil || array.JavaType == nil || array.ArrayDim() != 1 || !array.HasOriginPC || !array.HasEvaluationEndPC || array.EvaluationEndPC <= array.OriginPC || len(array.Initializer) < 2 || len(array.Initializer) > 64 {
		return nil
	}
	if _, alreadyGeneric := types.AsParameterizedType(array.ElementType()); alreadyGeneric {
		return nil
	}
	raw, ok := types.RawClassFQN(array.ElementType())
	if !ok {
		return nil
	}
	var element types.JavaType
	for _, item := range array.Initializer {
		ref, ok := genericArrayOperand(item).(*values.JavaRef)
		if !ok || ref == nil || ref.StackVar != nil || ref.CustomValue != nil || !ref.IsParam {
			return nil
		}
		typ := params[ref]
		p, ok := types.AsParameterizedType(typ)
		if !ok || p.RawClassName != raw || len(p.TypeArgs) == 0 {
			return nil
		}
		if element == nil {
			element = typ
		} else if !reflect.DeepEqual(element.RawType(), typ.RawType()) {
			return nil
		}
	}
	return element
}

func genericArrayDenotes(value values.JavaValue, ref *values.JavaRef, array *values.NewExpression) bool {
	value = genericArrayOperand(value)
	if value == ref || value == array {
		return true
	}
	return genericArrayAllocation(value) == array
}

func genericArrayAllocation(value values.JavaValue) *values.NewExpression {
	_, array := genericArrayAliasChain(value)
	return array
}

func genericArrayAliasChain(value values.JavaValue) ([]*values.JavaRef, *values.NewExpression) {
	var refs []*values.JavaRef
	for depth := 0; depth < 8; depth++ {
		value = genericArrayOperand(value)
		if array, ok := value.(*values.NewExpression); ok {
			return refs, array
		}
		ref, ok := value.(*values.JavaRef)
		if !ok || ref == nil || ref.IsParam || ref.IsThis || ref.StackVar != nil || ref.CustomValue != nil {
			return nil, nil
		}
		refs = append(refs, ref)
		value = ref.Val
	}
	return nil, nil
}

// Narrowing a mutable/aliased array declaration would need constraints from
// every write and escape. Accept only the completed straight-line initializer
// followed by one invocation use, with no other store, escape or element read.
func (d *Decompiler) genericArrayUseProven(ref *values.JavaRef, array *values.NewExpression, store *OpCode) bool {
	allocation := d.opcodeAtOffset(array.OriginPC)
	if allocation == nil || allocation.Instr == nil || allocation.Instr.OpCode != OP_ANEWARRAY {
		return false
	}
	stores, uses := 0, 0
	var invocation *OpCode
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			return false
		}
		for index, consumed := range op.stackConsumed {
			if !genericArrayDenotes(consumed, ref, array) || op == store {
				continue
			}
			pc := int(op.CurrentOffset)
			if op.Instr.OpCode == OP_DUP && index == 0 && pc > array.OriginPC && pc <= array.EvaluationEndPC && len(op.stackConsumed) == 1 && len(op.stackProduced) == 2 && genericArrayDenotes(op.stackProduced[0], ref, array) && genericArrayDenotes(op.stackProduced[1], ref, array) {
				continue
			}
			if op.Instr.OpCode == OP_AASTORE && index == 2 && pc > array.OriginPC && pc <= array.EvaluationEndPC {
				stores++
				continue
			}
			if pc <= array.EvaluationEndPC {
				return false
			}
			switch op.Instr.OpCode {
			case OP_INVOKESTATIC, OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE, OP_INVOKESPECIAL:
				uses++
				invocation = op
			default:
				return false
			}
		}
	}
	return stores == len(array.Initializer) && uses == 1 && branchArraySinglePath(d, allocation, invocation)
}

func (d *Decompiler) recoverGenericArrayDeclarations(idToOpcode map[int]*OpCode) {
	if d == nil || d.RootNode == nil || len(d.opCodes) > 4096 {
		return
	}
	params := d.genericArrayParameterTypes()
	webs := d.slotWebs()
	if len(params) == 0 || webs == nil {
		return
	}
	counts := map[int]int{}
	owners := map[*values.JavaRef]map[int]bool{}
	uidOwners := map[string]map[int]bool{}
	paramWebs := d.parameterWebRefs(webs)
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || !isLocalStoreOpcode(op.Instr.OpCode) {
			continue
		}
		web, valid := webs.webOf[op]
		if !valid {
			continue
		}
		counts[web]++
		if param := paramWebs[web]; param != nil {
			delete(params, param)
		}
		infos := d.opcodeIdToRef[op]
		if len(infos) != 1 {
			continue
		}
		if ref, ok := infos[0][0].(*values.JavaRef); ok && ref != nil {
			if owners[ref] == nil {
				owners[ref] = map[int]bool{}
			}
			owners[ref][web] = true
			if uidOwners[ref.VarUid] == nil {
				uidOwners[ref.VarUid] = map[int]bool{}
			}
			uidOwners[ref.VarUid][web] = true
		}
	}
	candidates := 0
	WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		assign, ok := node.Statement.(*statements.AssignStatement)
		if !ok || node.IsDel || assign.ArrayMember != nil {
			return node.Next, nil
		}
		ref, ok := assign.LeftValue.(*values.JavaRef)
		array := genericArrayAllocation(assign.JavaValue)
		store := idToOpcode[node.Id]
		if !ok || ref == nil || ref.IsParam || ref.IsThis || ref.StackVar != nil || ref.CustomValue != nil || array == nil || store == nil {
			return node.Next, nil
		}
		candidates++
		if candidates > 32 || ref.VarUid == "" {
			return node.Next, nil
		}
		web, valid := webs.webOf[store]
		if !valid || counts[web] != 1 || len(owners[ref]) != 1 || !owners[ref][web] || len(uidOwners[ref.VarUid]) != 1 || paramWebs[web] != nil {
			return node.Next, nil
		}
		element := genericArrayElementType(array, params)
		if element != nil && d.genericArrayUseProven(ref, array, store) {
			aliases, allocation := genericArrayAliasChain(assign.JavaValue)
			if allocation != array {
				return node.Next, nil
			}
			for _, alias := range aliases {
				// DUP creates an allocation temporary, not another local web.
				// A real stored alias would need its own complete constraints.
				if alias != ref && (len(owners[alias]) != 0 || len(uidOwners[alias.VarUid]) != 0) {
					return node.Next, nil
				}
			}
			declaration := types.NewJavaArrayType(element.Copy())
			ref.ResetVarType(declaration)
			ref.WebDeclType = declaration.Copy()
			for _, alias := range aliases {
				alias.ResetVarType(declaration.Copy())
				alias.WebDeclType = declaration.Copy()
			}
		}
		return node.Next, nil
	})
}
