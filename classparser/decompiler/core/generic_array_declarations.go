package core

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
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
func (d *Decompiler) genericArrayUseProven(ref *values.JavaRef, array *values.NewExpression, store *OpCode, element types.JavaType) bool {
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
			case OP_INVOKESTATIC:
				if len(op.stackConsumed) != 1 {
					return false
				}
				uses++
				invocation = op
			default:
				return false
			}
		}
	}
	return stores == len(array.Initializer) && uses == 1 && branchArraySinglePath(d, allocation, invocation) && d.genericArrayFactoryReceiverUse(invocation, element)
}

// An unchecked array can intentionally satisfy a different parameterized
// return or another argument's invariant constraint. Initializer evidence alone
// cannot narrow those uses. Limit recovery to an Object[] factory whose result
// has one direct instance-receiver use; preserve raw returns, stores and escapes.
func (d *Decompiler) genericArrayFactoryReceiverUse(factory *OpCode, element types.JavaType) bool {
	if factory == nil || len(factory.stackProduced) != 1 {
		return false
	}
	result, ok := genericArrayOperand(factory.stackProduced[0]).(*values.FunctionCallExpression)
	if !ok || result == nil || result.Kind != values.InvokeStatic || !result.HasOriginPC || result.OriginPC != int(factory.CurrentOffset) {
		return false
	}
	typ, err := types.ParseMethodDescriptor(result.Descriptor)
	if err != nil || typ.FunctionType() == nil || len(typ.FunctionType().ParamTypes) != 1 {
		return false
	}
	param := typ.FunctionType().ParamTypes[0]
	if !param.IsArray() || param.ArrayDim() != 1 {
		return false
	}
	raw, known := types.RawClassFQN(param.ElementType())
	if !known || raw != "java.lang.Object" {
		return false
	}
	uses := 0
	var consumer *OpCode
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			return false
		}
		for index, value := range op.stackConsumed {
			if genericArrayOperand(value) != result {
				continue
			}
			if (op.Instr.OpCode != OP_INVOKEVIRTUAL && op.Instr.OpCode != OP_INVOKEINTERFACE) || index != len(op.stackConsumed)-1 {
				return false
			}
			uses++
			consumer = op
		}
	}
	if uses != 1 || !branchArraySinglePath(d, factory, consumer) || len(consumer.stackProduced) != 1 {
		return false
	}
	projection, ok := genericArrayOperand(consumer.stackProduced[0]).(*values.FunctionCallExpression)
	kind := values.InvokeVirtual
	if consumer.Instr.OpCode == OP_INVOKEINTERFACE {
		kind = values.InvokeInterface
	}
	if !ok || projection == nil || projection.Kind != kind || !projection.HasOriginPC || projection.OriginPC != int(consumer.CurrentOffset) || genericArrayOperand(projection.Object) != result || len(consumer.stackConsumed) != len(projection.Arguments)+1 || !d.genericArrayProjectionTypes(result, projection, element) {
		return false
	}
	for i, argument := range projection.Arguments {
		if genericArrayOperand(argument) != genericArrayOperand(consumer.stackConsumed[len(projection.Arguments)-i-1]) {
			return false
		}
	}
	uses = 0
	var ret *OpCode
	for _, op := range d.opCodes {
		for _, value := range op.stackConsumed {
			if genericArrayOperand(value) == projection {
				if op.Instr.OpCode != OP_ARETURN {
					return false
				}
				uses++
				ret = op
			}
		}
	}
	return uses == 1 && branchArraySinglePath(d, consumer, ret)
}

func (d *Decompiler) genericArrayCallSignature(call *values.FunctionCallExpression) (string, string) {
	ctx := d.FunctionContext
	if ctx == nil || call == nil || call.Descriptor == "" {
		return "", ""
	}
	key := class_context.MethodDescKey(call.FunctionName, call.Descriptor)
	if call.ClassName == ctx.ClassName {
		sig := ctx.MethodSignaturesByDesc[key]
		if len(sig) > 8192 {
			return "", ""
		}
		return ctx.ClassSig, sig
	}
	if ctx.SiblingClassSig == nil {
		return "", ""
	}
	classSig, methods, known := ctx.SiblingClassSig(strings.ReplaceAll(call.ClassName, ".", "/"))
	if !known || len(methods[key]) > 8192 {
		return "", ""
	}
	return classSig, methods[key]
}

func genericArrayUnboundedFormal(sig string, class bool) string {
	if len(sig) == 0 || len(sig) > 8192 {
		return ""
	}
	names := types.MethodFormalTypeParamNames(sig)
	if class {
		names = types.ClassFormalTypeParamNames(sig)
	}
	if len(names) != 1 || !strings.HasPrefix(sig, "<"+names[0]+":Ljava/lang/Object;>") {
		return ""
	}
	return names[0]
}

// This bounded inference proof covers array factory -> generic identity mapper
// -> covariant projection. Names are irrelevant; exact declaration signatures
// must connect every variable. A mere receiver use is insufficient: keep(T)
// can introduce a conflicting invariant constraint even after a private array.
func (d *Decompiler) genericArrayProjectionTypes(factory, projection *values.FunctionCallExpression, element types.JavaType) bool {
	input, ok := types.AsParameterizedType(element)
	if !ok || len(input.TypeArgs) != 1 {
		return false
	}
	bound, ok := input.TypeArgs[0].(*types.JavaWildcardType)
	if !ok || bound == nil || bound.Variant != "extends" || bound.Bound == nil {
		return false
	}
	classSig, factorySig := d.genericArrayCallSignature(factory)
	classVar := genericArrayUnboundedFormal(classSig, true)
	factoryVar := genericArrayUnboundedFormal(factorySig, false)
	if classVar == "" || factoryVar == "" {
		return false
	}
	owner := strings.ReplaceAll(factory.ClassName, ".", "/")
	if factorySig != "<"+factoryVar+":Ljava/lang/Object;>([T"+factoryVar+";)L"+owner+"<T"+factoryVar+";>;" || projection.ClassName != factory.ClassName || len(projection.Arguments) < 1 || len(projection.Arguments) > 8 {
		return false
	}
	_, projectionSig := d.genericArrayCallSignature(projection)
	projectionVar := genericArrayUnboundedFormal(projectionSig, false)
	_, params, ret := types.ParseMethodSignatureFull(projectionSig, nil)
	if projectionVar == "" || projectionVar == classVar || ret == nil || len(params) != len(projection.Arguments) {
		return false
	}
	mapper, ok := genericArrayOperand(projection.Arguments[0]).(*values.FunctionCallExpression)
	if !ok || mapper == nil || mapper.Kind != values.InvokeStatic || !mapper.HasOriginPC || len(mapper.Arguments) != 0 {
		return false
	}
	mapperOp := d.opcodeAtOffset(mapper.OriginPC)
	if mapperOp == nil || mapperOp.Instr == nil || mapperOp.Instr.OpCode != OP_INVOKESTATIC || len(mapperOp.stackConsumed) != 0 || len(mapperOp.stackProduced) != 1 || genericArrayOperand(mapperOp.stackProduced[0]) != mapper || mapper.OriginPC <= factory.OriginPC || mapper.OriginPC >= projection.OriginPC {
		return false
	}
	_, mapperSig := d.genericArrayCallSignature(mapper)
	mapperVar := genericArrayUnboundedFormal(mapperSig, false)
	_, mapperParams, mapperReturn := types.ParseMethodSignatureFull(mapperSig, nil)
	mapperType, ok := types.AsParameterizedType(mapperReturn)
	if mapperVar == "" || !ok || len(mapperParams) != 0 || len(mapperType.TypeArgs) != 2 || !reflect.DeepEqual(mapperType.TypeArgs[0].RawType(), types.NewJavaClass(mapperVar).RawType()) || !reflect.DeepEqual(mapperType.TypeArgs[1].RawType(), types.NewJavaClass(mapperVar).RawType()) {
		return false
	}
	pattern := types.ParseSignature("L" + strings.ReplaceAll(mapperType.RawClassName, ".", "/") + "<-T" + classVar + ";+L" + strings.ReplaceAll(input.RawClassName, ".", "/") + "<+T" + projectionVar + ";>;>;")
	if pattern == nil || !reflect.DeepEqual(params[0].RawType(), pattern.RawType()) || !reflect.DeepEqual(ret.RawType(), types.NewParameterizedType(factory.ClassName, []types.JavaType{types.NewJavaClass(projectionVar)}).RawType()) {
		return false
	}
	// Extra primitive arguments (e.g. delay-error/concurrency) cannot add any
	// generic constraints. Reference arguments require a full solver; decline.
	for _, param := range params[1:] {
		if _, primitive := param.RawType().(*types.JavaPrimer); !primitive {
			return false
		}
	}
	ctx := d.FunctionContext
	_, _, callerReturn := types.ParseMethodSignatureFull(ctx.MethodSignatureByDesc(ctx.FunctionName, ctx.CurrentMethodDesc), ctx)
	want := types.NewParameterizedType(factory.ClassName, []types.JavaType{bound.Bound})
	return callerReturn != nil && reflect.DeepEqual(callerReturn.RawType(), want.RawType())
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
	paramUIDs := map[string]bool{}
	for _, value := range d.Params {
		if ref, ok := value.(*values.JavaRef); ok && ref != nil {
			paramUIDs[ref.VarUid] = true
		}
	}
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
	definitions := map[string]int{}
	nodeCount := 0
	WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		nodeCount++
		if assign, ok := node.Statement.(*statements.AssignStatement); ok && !node.IsDel && assign.ArrayMember == nil {
			if ref, ok := assign.LeftValue.(*values.JavaRef); ok && ref != nil {
				definitions[ref.VarUid]++
			}
		}
		return node.Next, nil
	})
	if nodeCount > 4096 {
		return
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
		if candidates > 32 || ref.VarUid == "" || paramUIDs[ref.VarUid] || definitions[ref.VarUid] != 1 || store.Instr == nil {
			return node.Next, nil
		}
		web, valid := webs.webOf[store]
		local := isLocalStoreOpcode(store.Instr.OpCode) && valid && counts[web] == 1 && len(owners[ref]) == 1 && owners[ref][web] && len(uidOwners[ref.VarUid]) == 1 && paramWebs[web] == nil
		// Inline varargs have no ASTORE/local web. The renderer materializes
		// their DUP allocation into a temporary; prove that exact definition,
		// both stack outputs, and the absence of real local owners instead.
		dup := store.Instr.OpCode == OP_DUP && len(owners[ref]) == 0 && len(uidOwners[ref.VarUid]) == 0 && genericArrayOperand(ref.Val) == array && genericArrayOperand(assign.JavaValue) == array && int(store.CurrentOffset) > array.OriginPC && int(store.CurrentOffset) <= array.EvaluationEndPC && len(store.stackConsumed) == 1 && len(store.stackProduced) == 2 && genericArrayDenotes(store.stackConsumed[0], ref, array) && genericArrayDenotes(store.stackProduced[0], ref, array) && genericArrayDenotes(store.stackProduced[1], ref, array)
		if !local && !dup {
			return node.Next, nil
		}
		element := genericArrayElementType(array, params)
		if element != nil && d.genericArrayUseProven(ref, array, store, element) {
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
