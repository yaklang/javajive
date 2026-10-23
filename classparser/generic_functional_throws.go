package javaclassparser

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A functional argument supplies a lower bound for a method-owned throws
// variable. Erased Exceptions is the declaration ABI, not the exception type
// of this source invocation. This finite solver accepts a single covariant
// no-argument SAM, concrete ordered upper bounds and an unambiguous family.
// The caller must independently prove the original factory and reaching use,
// and publish the returned parameterized argument view at that exact call.
func genericFunctionalThrows(provider callbinding.Provider, owner, name, descriptor string, static bool, argument int, samName, samDescriptor, actualDescriptor string, work *workbudget.Budget) (types.JavaType, string, bool) {
	view, receiver, exception, known := genericFunctionalThrowsReceiver(provider, owner, name, descriptor, static, argument, samName, samDescriptor, actualDescriptor, work)
	if receiver != nil {
		return nil, "", false
	}
	return view, exception, known
}

func genericFunctionalThrowsReceiver(provider callbinding.Provider, owner, name, descriptor string, static bool, argument int, samName, samDescriptor, actualDescriptor string, work *workbudget.Budget) (types.JavaType, types.JavaType, string, bool) {
	if provider == nil || strings.HasPrefix(name, "<") || len(owner)+len(name)+len(descriptor)+len(samDescriptor)+len(actualDescriptor) > 8192 || !nativeProofWork(work, 1) {
		return nil, nil, "", false
	}
	if work != nil && work.CheckAlloc(128<<10) != nil {
		return nil, nil, "", false
	}
	physical, _, err := callbinding.Descriptor(descriptor)
	actual, result, ae := callbinding.Descriptor(actualDescriptor)
	if err != nil || ae != nil || len(actual) != 0 || len(physical) > 128 || argument < 0 || argument >= len(physical) || !strings.HasPrefix(physical[argument], "L") || !strings.HasPrefix(result, "L") {
		return nil, nil, "", false
	}
	// Metadata lookups are bounded by the proof-context provider. Refuse any
	// same-arity rival, including generic/bridge/return-only collisions.
	var declaring callbinding.Class
	familyProvider := func(owner string) (callbinding.Class, bool) {
		declaration, known := provider(owner)
		if !known || len(declaration.Methods) > 256 {
			return callbinding.Class{}, false
		}
		for _, method := range declaration.Methods {
			if method.Name == name && method.Desc == descriptor {
				declaring = declaration
			}
			if method.Name == name && method.Varargs {
				return callbinding.Class{}, false
			}
		}
		return declaration, true
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: name, Desc: descriptor}, familyProvider)
	if err != nil || !family.Complete || len(family.Methods) != 1 || family.Target == nil {
		return nil, nil, "", false
	}
	target := *family.Target
	if !target.Generic || target.Static != static || target.Bridge || target.Varargs || !target.ExceptionsKnown || len(target.Exceptions) != 1 || len(target.Signature) > 8192 {
		return nil, nil, "", false
	}
	// Class formals are retained by an all-wildcard receiver view. Only a
	// class-owned result is admitted: no class variable may constrain a
	// method input or the method-owned exception variable.
	erased, exceptions, known := types.EraseLexicalMethodSignatureWithThrows(declaring.Signature, target.Signature)
	if !known || erased != descriptor || len(exceptions) != 1 || callbinding.Name(exceptions[0]) != target.Exceptions[0] {
		return nil, nil, "", false
	}
	bounds, bare, known := types.FormalBoundConstraints(target.Signature)
	if !known || len(bounds) != 1 || len(bare) != 0 {
		return nil, nil, "", false
	}
	_, parameters, returned, thrown := types.ParseMethodSignatureFullWithThrows(target.Signature, &class_context.ClassContext{})
	if returned == nil || len(parameters) != len(physical) || len(thrown) != 1 {
		return nil, nil, "", false
	}
	variable, known := types.TypeVariableName(thrown[0])
	if !known || len(bounds[variable]) == 0 {
		return nil, nil, "", false
	}
	parameter, known := types.AsParameterizedType(parameters[argument])
	if !known || len(parameter.OwnerSegments) > 1 || len(parameter.TypeArgs) != 1 || strings.ReplaceAll(parameter.RawClassName, ".", "/") != callbinding.Name(physical[argument]) {
		return nil, nil, "", false
	}
	wildcard, known := parameter.TypeArgs[0].(*types.JavaWildcardType)
	if !known || wildcard.Variant != "extends" {
		return nil, nil, "", false
	}
	bound, known := types.TypeVariableName(wildcard.Bound)
	if !known || bound != variable {
		return nil, nil, "", false
	}
	var receiver types.JavaType
	classFormals := types.ClassFormalTypeParamNames(declaring.Signature)
	resultVariable, variableResult := types.TypeVariableName(returned)
	if variableResult {
		if static || resultVariable == variable || !slices.Contains(classFormals, resultVariable) {
			return nil, nil, "", false
		}
	}
	if len(classFormals) > 0 && !static {
		if declaring.Name != owner || len(classFormals) > 16 {
			return nil, nil, "", false
		}
		classBounds, valid := types.FormalTypeBounds(declaring.Signature)
		if !valid {
			return nil, nil, "", false
		}
		for _, formal := range classFormals {
			// An intersection capture can change a surrounding overload's
			// source applicability although its first-bound erasure is equal.
			if len(classBounds[formal]) != 1 {
				return nil, nil, "", false
			}
			if _, parameterized := types.AsParameterizedType(classBounds[formal][0]); parameterized {
				return nil, nil, "", false
			}
		}
		args := make([]types.JavaType, len(classFormals))
		for i := range args {
			args[i] = &types.JavaWildcardType{}
		}
		receiver = types.NewParameterizedType(strings.ReplaceAll(owner, "/", "."), args)
	}
	for i, p := range parameters {
		if i != argument && !constructorInvocationThrowsSimpleType(p, false) {
			return nil, nil, "", false
		}
	}
	_, references, known := types.SignatureTypeVariableReferences(target.Signature)
	if !known {
		return nil, nil, "", false
	}
	methodUses, classUses := 0, 0
	for _, ref := range references {
		if ref == variable {
			methodUses++
		} else if variableResult && ref == resultVariable {
			classUses++
		} else {
			return nil, nil, "", false
		}
	}
	if methodUses != 2 || variableResult && classUses != 1 || !variableResult && classUses != 0 {
		return nil, nil, "", false
	}
	functional, known := provider(callbinding.Name(physical[argument]))
	if !known || !functional.IsInterface || !functional.MembersComplete || !functional.ParentsComplete || len(functional.Methods) != 1 || len(functional.Signature) > 8192 || len(functional.Parents) != 1 || functional.Parents[0] != "java/lang/Object" {
		return nil, nil, "", false
	}
	formals := types.ClassFormalTypeParamNames(functional.Signature)
	if len(formals) != 1 {
		return nil, nil, "", false
	}
	sam := functional.Methods[0]
	if sam.Name != samName || sam.Desc != samDescriptor || sam.Static || sam.Bridge || sam.Varargs || sam.Signature != "()T"+formals[0]+";" || !sam.ExceptionsKnown || len(sam.Exceptions) != 0 {
		return nil, nil, "", false
	}
	samErased, _, known := types.EraseLexicalMethodSignatureWithThrows(functional.Signature, sam.Signature)
	if !known || samErased != samDescriptor {
		return nil, nil, "", false
	}
	q := newConstructorWideningQuery(provider)
	for _, constraint := range bounds[variable] {
		raw, valid := types.RawClassFQN(constraint)
		if _, parameterized := types.AsParameterizedType(constraint); parameterized || !valid || !q.assignable(result, "L"+strings.ReplaceAll(raw, ".", "/")+";") {
			return nil, nil, "", false
		}
	}
	fiBounds, known := types.FormalTypeBounds(functional.Signature)
	if !known || len(fiBounds) != 1 {
		return nil, nil, "", false
	}
	for _, constraint := range fiBounds[formals[0]] {
		raw, valid := types.RawClassFQN(constraint)
		if _, parameterized := types.AsParameterizedType(constraint); parameterized || !valid || !q.assignable(result, "L"+strings.ReplaceAll(raw, ".", "/")+";") {
			return nil, nil, "", false
		}
	}
	// Initially admit only proven unchecked instantiations. Checked throws
	// need a caller's source Signature/handler coverage proof as well.
	if !originalExceptionUnchecked(callbinding.Name(result), provider) {
		return nil, nil, "", false
	}
	return types.NewParameterizedType(parameter.RawClassName, []types.JavaType{types.NewJavaClass(strings.ReplaceAll(callbinding.Name(result), "/", "."))}), receiver, callbinding.Name(result), true
}
