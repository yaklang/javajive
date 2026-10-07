package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// Match the operand back to the original GETFIELD of this lexical instance.
// A materialized qualifier requires its unique unchanged declaration and source
// dominance. JavaRef.Val is mutable simulator state and is never evidence here.
func nativeMemberLexicalEnclosingOperand(value any, plan *nativeMemberAllocation, family *nativeMemberFamily, current string, body []statements.Statement, work *workbudget.Budget) bool {
	if plan == nil || plan.child == nil || plan.enclosingReadPC < 0 || family == nil || !nativeProofWork(work, 4) {
		return false
	}
	child := family.children[current]
	anonymousRead := plan.anonymousEnclosingRead
	if anonymousRead != nil {
		forest := family.anonymousForest
		if forest == nil || forest.units[current] == nil || nativeMemberAnonymousAllocationEnclosingRead(family, forest.units[current].object, plan.anonymousEnclosingMethod, plan.enclosingReadPC, plan.child.owner, work) != anonymousRead {
			return false
		}
	} else if child == nil || child.object == nil || child.object.GetClassName() != current || child.static || child.owner != plan.child.owner {
		return false
	}
	operand, ok := value.(values.JavaValue)
	if !ok || sourceProofNil(operand) {
		return false
	}
	operand, ok = nativeMemberEnclosingUnpack(operand, work)
	if !ok {
		return false
	}
	if ref, ok := operand.(*values.JavaRef); ok {
		if ref == nil || ref.IsThis || ref.IsParam || ref.CustomValue != nil || ref.StackVar != nil {
			return false
		}
		if !nativeMemberDeclarationVisibleAtCall(body, nil, plan.invokePC, work) {
			return false
		}
		declaration, known := nativeCaptureDeclaration(body, ref, false, work)
		if !known || declaration == nil || !nativeMemberDeclarationVisibleAtCall(body, declaration, plan.invokePC, work) {
			return false
		}
		operand, ok = nativeMemberEnclosingUnpack(declaration.JavaValue, work)
		if !ok {
			return false
		}
	}
	if anonymousRead != nil {
		return nativeMemberLexicalReadOperand(operand, anonymousRead, work)
	}
	field, ok := operand.(*values.RefMember)
	if !ok || field == nil || !field.HasOriginPC || field.OriginPC != plan.enclosingReadPC || field.Member != child.field {
		return false
	}
	object, known := nativeMemberEnclosingUnpack(field.Object, work)
	if !known {
		return false
	}
	receiver, ok := object.(*values.JavaRef)
	return ok && receiver != nil && receiver.IsThis && receiver.CustomValue == nil && receiver.StackVar == nil
}

// The complete anonymous forest already certifies original receiver stability,
// control-entry boundaries and every capture field in the dereference chain.
// Keep the exact object, method and read occurrence when composing that proof
// with a named member allocation. Descriptor equality alone grants no scope.
func nativeMemberAnonymousAllocationEnclosingRead(p *nativeMemberFamily, object *ClassObject, method string, pc int, owner string, work *workbudget.Budget) *nativeMemberLexicalRead {
	if p == nil || p.failed || object == nil || !nativeProofWork(work, 4) {
		return nil
	}
	forest := p.anonymousForest
	current := object.GetClassName()
	group := p.anonymousUnits[current]
	if forest == nil || forest.members != p || group == nil || group.failed || group.forest != forest || forest.groups[group.owner] != group || forest.objects[current] != object || group.children[current] == nil || group.children[current] != forest.units[current] || group.children[current].object != object {
		return nil
	}
	read := forest.reads[current][method][pc]
	if read == nil || read.pc != pc || read.descriptor != "L"+owner+";" || !forest.lexicalThis[current][method][pc] {
		return nil
	}
	seen := map[*nativeMemberLexicalRead]bool{}
	for node := read; node != nil; node = node.prior {
		if len(seen) >= 64 || seen[node] || node.parameterOwner != "" || !nativeProofWork(work, 1) {
			return nil
		}
		seen[node] = true
	}
	return read
}

// A unique declaration in another branch, a later declaration, or a declaration
// outside the call's source scope is not a dominating captured qualifier.
func nativeMemberDeclarationVisibleAtCall(body []statements.Statement, declaration *statements.AssignStatement, pc int, work *workbudget.Budget) bool {
	remaining, sites := 8192, 0
	valid := true
	activeStatements := map[statements.Statement]bool{}
	activeValues := map[values.JavaValue]bool{}
	step := func() bool { remaining--; return remaining >= 0 && nativeProofWork(work, 1) }
	var value func(values.JavaValue, bool)
	value = func(v values.JavaValue, visible bool) {
		if work != nil {
			if work.Enter(workbudget.CounterASTDepth) != nil {
				valid = false
				return
			}
			defer work.Leave(workbudget.CounterASTDepth)
		}
		if !step() || sourceProofNil(v) || activeValues[v] {
			valid = false
			return
		}
		activeValues[v] = true
		defer delete(activeValues, v)
		if allocation, ok := v.(*values.NewExpression); ok && allocation.ConstructorCall != nil {
			call := allocation.ConstructorCall
			if call.HasOriginPC && call.OriginPC == pc && call.FunctionName == "<init>" {
				sites++
				if !visible {
					valid = false
				}
			}
		}
		if call, ok := v.(*values.FunctionCallExpression); ok {
			if call.HasOriginPC && call.OriginPC == pc && call.FunctionName == "<init>" {
				sites++
				if !visible {
					valid = false
				}
			}
			if !call.IsStatic {
				value(call.Object, visible)
			}
			for _, arg := range call.Arguments {
				value(arg, visible)
			}
			return
		}
		children, known := values.Children(v)
		if !known {
			valid = false
			return
		}
		for _, child := range children {
			value(child, visible)
		}
	}
	var walk func([]statements.Statement, bool)
	walk = func(body []statements.Statement, visible bool) {
		if work != nil {
			if work.Enter(workbudget.CounterASTDepth) != nil {
				valid = false
				return
			}
			defer work.Leave(workbudget.CounterASTDepth)
		}
		for _, s := range body {
			if !step() || sourceProofNil(s) || activeStatements[s] {
				valid = false
				return
			}
			activeStatements[s] = true
			roots, children, known := catchSourceChildren(s)
			if !known {
				valid = false
				return
			}
			for _, root := range roots {
				value(root, visible)
			}
			for _, child := range children {
				walk(child, visible)
			}
			if s == declaration {
				visible = true
			}
			delete(activeStatements, s)
		}
	}
	walk(body, declaration == nil)
	return valid && sites == 1
}

func nativeMemberEnclosingTypeDenotable(c *ClassObjectDumper, view types.JavaType) bool {
	if c == nil || c.obj == nil || c.FuncCtx == nil || c.nativeOuterContext == nil || c.nativeMemberCurrent == nil || !nativeProofWork(c.Work, 1) {
		return false
	}
	typeArguments, ok := types.AsParameterizedType(view)
	if !ok || strings.ReplaceAll(typeArguments.RawClassName, ".", "/") != c.nativeMemberCurrent.owner || len(typeArguments.TypeArgs) != len(c.nativeOuterContext.ClassTypeParams) {
		return false
	}
	shadowed := map[string]bool{}
	for _, attribute := range c.obj.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if sig, ok := attribute.(*SignatureAttribute); ok {
			if sig == nil {
				return false
			}
			text, known := sourceBridgeUTF8(c.obj, sig.SignatureIndex)
			if !known || !nativeProofWork(c.Work, int64(len(text))) {
				return false
			}
			for _, name := range types.ClassFormalTypeParamNames(text) {
				shadowed[name] = true
			}
		}
	}
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		name, nok := sourceBridgeUTF8(c.obj, method.NameIndex)
		desc, dok := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
		if !nok || !dok {
			return false
		}
		if name+desc != c.FuncCtx.FunctionName+c.FuncCtx.CurrentMethodDesc {
			continue
		}
		for _, attribute := range method.Attributes {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			if sig, ok := attribute.(*SignatureAttribute); ok {
				if sig == nil {
					return false
				}
				text, known := sourceBridgeUTF8(c.obj, sig.SignatureIndex)
				if !known || !nativeProofWork(c.Work, int64(len(text))) {
					return false
				}
				for _, name := range types.MethodFormalTypeParamNames(text) {
					shadowed[name] = true
				}
			}
		}
	}
	for index, argument := range typeArguments.TypeArgs {
		if sourceProofNil(argument) || !nativeProofWork(c.Work, 1) {
			return false
		}
		formal, ok := argument.RawType().(*types.JavaClass)
		if !ok || formal.Name != c.nativeOuterContext.ClassTypeParams[index] || shadowed[formal.Name] {
			return false
		}
	}
	return true
}

// Slot wrappers do not confer a new identity. Bound malformed/cyclic wrapper
// chains locally, including when a standalone caller supplies no work budget.
func nativeMemberEnclosingUnpack(value values.JavaValue, work *workbudget.Budget) (values.JavaValue, bool) {
	for depth := 0; depth < 32; depth++ {
		if sourceProofNil(value) || !nativeProofWork(work, 1) {
			return nil, false
		}
		if slot, ok := value.(*values.SlotValue); ok {
			value = slot.GetValue()
			continue
		}
		return value, true
	}
	return nil, false
}
