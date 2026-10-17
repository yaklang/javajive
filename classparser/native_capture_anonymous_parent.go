package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// An anonymous implementation has no source declaration name. Its original
// generic parent, already emitted at the same lexical allocation site, can
// instead declare a captured local. This changes a source view only: the NEW,
// constructor operands, original callback order and runtime class stay intact.
// Closed type arguments and ordinary class/interface intersection bounds are
// proved here. Method variables, wildcards and parameterized bounds require a
// generic hierarchy/binder proof and are deliberately not erased to pass.
func (c *ClassObjectDumper) nativeCapturedAnonymousParentType(declaration *statements.AssignStatement, actual, expected string, target callbinding.Class, definition *ClassObject, provider callbinding.Provider) (types.JavaType, bool) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || declaration == nil || sourceProofNil(declaration.JavaValue) || provider == nil || definition == nil {
		return nil, false
	}
	group := c.nativeAnonymousRoot
	if group == nil || group.failed || group.owner != c.obj.GetClassName() || !nativeProofWork(c.Work, 1) {
		return nil, false
	}
	seed, known := nativeMemberEnclosingUnpack(declaration.JavaValue, c.Work)
	allocation, allocated := seed.(*values.NewExpression)
	producer := group.children[callbinding.Name(actual)]
	if !known || !allocated || allocation == nil || !allocation.HasOriginPC || allocation.ConstructorCall == nil || producer == nil || producer.object == nil || producer.object.GetClassName() != callbinding.Name(actual) {
		return nil, false
	}
	call := allocation.ConstructorCall
	owner, method, original := originalAnonymousOwner(producer.object)
	if !original || owner != group.owner || method != producer.method || method != c.FuncCtx.FunctionName+c.FuncCtx.CurrentMethodDesc ||
		!call.HasOriginPC || allocation.OriginPC != producer.newPC || call.OriginPC != producer.invokePC ||
		strings.ReplaceAll(call.ClassName, ".", "/") != producer.object.GetClassName() || call.FunctionName != "<init>" || call.Descriptor != producer.descriptor ||
		call.IsStatic || call.Kind != values.InvokeSpecial {
		return nil, false
	}
	receiver, known := nativeMemberEnclosingUnpack(call.Object, c.Work)
	if !known || receiver != allocation {
		return nil, false
	}
	signature, known := nativeOriginalClassSignature(producer.object, c.Work)
	own, references, valid := types.SignatureTypeVariableReferences(signature)
	if !known || !valid || len(own) != 0 || len(references) != 0 {
		return nil, false
	}
	parent, interfaces := types.ParseClassSignatureSupers(signature)
	if len(producer.object.Interfaces) == 1 && producer.object.GetSupperClassName() == "java/lang/Object" {
		if len(interfaces) != 1 {
			return nil, false
		}
		parent = interfaces[0]
	} else if len(producer.object.Interfaces) != 0 || len(interfaces) != 0 {
		return nil, false
	}
	instantiation, parameterized := types.AsParameterizedType(parent)
	if !parameterized || strings.ReplaceAll(instantiation.RawClassName, ".", "/") != target.Name {
		return nil, false
	}
	erasure, typed := values.SourceTypeErasure(parent, c.FuncCtx)
	if !typed || erasure != expected {
		return nil, false
	}
	// The provider and actual original declaration must describe the same
	// formal/bound namespace; a forged same-name metadata table is insufficient.
	targetSignature, known := nativeOriginalClassSignature(definition, c.Work)
	formals := types.ClassFormalTypeParamNames(target.Signature)
	if !known || targetSignature != target.Signature || len(formals) == 0 || len(formals) != len(instantiation.TypeArgs) {
		return nil, false
	}
	declared := map[string]bool{}
	for _, formal := range formals {
		declared[formal] = true
	}
	_, targetReferences, valid := types.SignatureTypeVariableReferences(targetSignature)
	if !valid {
		return nil, false
	}
	for _, reference := range targetReferences {
		if !declared[reference] {
			return nil, false
		}
	}
	// Reuse the Signature grammar's ordered-bound parser. The leading formal
	// section is identical for class and method signatures; consume its balanced
	// angle section, then give the parser an empty method tail, never rendered text.
	depth, end := 0, -1
	for i, char := range targetSignature {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if char == '<' {
			depth++
		}
		if char == '>' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if end < 0 || c.Work != nil && c.Work.CheckAlloc(int64(end)+3) != nil {
		return nil, false
	}
	bounds, _, boundGrammar := types.FormalBoundConstraints(targetSignature[:end] + "()V")
	if !boundGrammar || len(bounds) != len(formals) {
		return nil, false
	}
	for i, formal := range formals {
		argument := instantiation.TypeArgs[i]
		if argument == nil {
			return nil, false
		}
		if _, wildcard := argument.RawType().(*types.JavaWildcardType); wildcard {
			return nil, false
		}
		actualArgument, known := values.SourceTypeErasure(argument, c.FuncCtx)
		if !known || !callbinding.Reference(actualArgument) {
			return nil, false
		}
		for _, bound := range bounds[formal] {
			if _, parameterized := types.AsParameterizedType(bound); parameterized {
				return nil, false
			}
			boundDescriptor, known := values.SourceTypeErasure(bound, c.FuncCtx)
			if !known || !callbinding.Reference(boundDescriptor) || !callbinding.Assignable(actualArgument, boundDescriptor, provider) {
				return nil, false
			}
		}
	}
	return parent.Copy(), nativeProofWork(c.Work, 1)
}
