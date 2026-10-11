package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"slices"
	"strings"
)

// A concrete anonymous allocation exposes its own source declarations without
// spelling its suppressed binary name. Require the original owned allocation
// and exact declaration binding without method inference; unrelated/inherited lookup stays separate.
func nativeAnonymousDeclaredCall(forest *nativeAnonymousForest, caller *ClassObject, method string, op *core.OpCode, work *workbudget.Budget) bool {
	if forest == nil || caller == nil || forest.objects[caller.GetClassName()] != caller || op == nil || op.Instr == nil || op.Instr.OpCode != core.OP_INVOKEVIRTUAL || len(op.Data) != 2 || !nativeProofWork(work, 1) {
		return false
	}
	index := int(core.Convert2bytesToInt(op.Data))
	if index <= 0 || index > len(caller.ConstantPool) {
		return false
	}
	if ref, ok := caller.ConstantPool[index-1].(*ConstantMethodrefInfo); !ok || ref == nil {
		return false
	}
	symbol := constructorMotionMember(caller, op, core.OP_INVOKEVIRTUAL)
	if symbol == nil || symbol.Member == "<init>" || symbol.Member == "<clinit>" {
		return false
	}
	child := forest.units[symbol.Name]
	group := forest.groups[caller.GetClassName()]
	if child == nil || child.object == nil || forest.objects[symbol.Name] != child.object || group == nil || group.failed || group.forest != forest || group.children[symbol.Name] != child || int(op.CurrentOffset) <= child.invokePC {
		return false
	}
	// EnclosingMethod names the lexical source method; a NEW in its
	// independently witnessed lambda implementation has a different physical
	// method key. Reuse the original allocation-scope certificate for both.
	cut := strings.IndexByte(method, '(')
	if cut <= 0 || !nativeAnonymousAllocationScope(caller, child, method[:cut], method[cut:], work) {
		return false
	}
	params, _, err := callbinding.Descriptor(symbol.Description)
	if err != nil || work != nil && work.CheckAlloc(int64(len(params))*64+128) != nil {
		return false
	}
	var target *MemberInfo
	for _, decl := range child.object.Methods {
		if decl == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, known := sourceBridgeUTF8(child.object, decl.NameIndex)
		if !known {
			return false
		}
		if name != symbol.Member {
			continue
		}
		desc, known := sourceBridgeUTF8(child.object, decl.DescriptorIndex)
		if !known {
			return false
		}
		args, _, err := callbinding.Descriptor(desc)
		if err != nil {
			return false
		}
		if !slices.Equal(params, args) {
			continue
		}
		if target != nil || desc != symbol.Description || decl.AccessFlags&(0x0002|0x0008|0x0040|0x0100|0x0400|0x1000) != 0 {
			return false
		}
		for _, a := range decl.Attributes {
			if _, generic := a.(*SignatureAttribute); generic && !nativeAnonymousLexicalZeroArgCall(forest, child.object, decl, desc, work) {
				return false
			}
		}
		target = decl
	}
	return target != nil
}

// A zero-argument concrete allocation has no parameter inference or argument
// overload conversion. Class/enclosing-method variables in its return type
// are declaration bindings, not generic method binders. Reopen those original
// scopes and require exact descriptor erasure; competing return/bridge targets
// are still refused by the full original declared-family scan above.
func nativeAnonymousLexicalZeroArgCall(forest *nativeAnonymousForest, owner *ClassObject, target *MemberInfo, descriptor string, work *workbudget.Budget) bool {
	args, _, err := callbinding.Descriptor(descriptor)
	if forest == nil || owner == nil || target == nil || err != nil || len(args) != 0 || forest.objects[owner.GetClassName()] != owner {
		return false
	}
	signature, known := nativeMethodLocalOriginalSignature(owner, target.Attributes, work)
	formals, _, valid := types.SignatureTypeVariableReferences(signature)
	if !known || !valid || len(formals) != 0 {
		return false
	}
	reader := NewClassObjectDumper(owner)
	reader.Work = work
	scopes, method, known := reader.flattenedMethodLexicalScopesResolved(func(name string) (*ClassObject, bool) {
		object := forest.objects[name]
		return object, object != nil && object.GetClassName() == name && nativeProofWork(work, 1)
	})
	if !known || !method {
		return false
	}
	signatures := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		signatures = append(signatures, scope.Signature)
	}
	if !nativeMethodLocalBindingBudget(signatures, signature, work) {
		return false
	}
	erased, throws, known := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, signature)
	return known && erased == descriptor && nativeOriginalSignatureThrows(owner, target, throws, work)
}
