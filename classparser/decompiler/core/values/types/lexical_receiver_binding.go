package types

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A member's own argument list does not contain its enclosing arguments.
// Bind each Signature segment to that exact original source declaration before
// resolving the method; raw owners and incomplete/static paths grant no scope.
func ResolveLexicalReceiverParamType(ctx *class_context.ClassContext, provider ClassSigProvider, receiver *JavaParameterizedType, method, descriptor string, argc, index int) JavaType {
	if ctx == nil || provider == nil || receiver == nil || argc <= 0 || index < 0 || index >= argc || method == "" || ctx.SiblingLexicalTypeOwners == nil || len(receiver.OwnerSegments) < 2 || len(receiver.OwnerSegments) > 64 {
		return nil
	}
	if erased, _, valid := EraseLexicalOwnerMethodSignatureWithThrows(nil, descriptor); !valid || erased != descriptor {
		return nil
	}
	_, parameters, _ := ParseMethodSignatureFull(descriptor, ctx)
	if len(parameters) != argc {
		return nil
	}
	leaf, sigma, signatures, valid := lexicalReceiverBindings(ctx, provider, receiver)
	if !valid {
		return nil
	}

	_, methods, available := provider(dotToInternal(receiver.RawClassName))
	if !available {
		return nil
	}
	if signature, declared := methods[class_context.MethodDescKey(method, descriptor)]; declared && signature != "" {
		erased, _, valid := EraseLexicalOwnerMethodSignatureWithThrows(signatures, signature)
		if !valid || erased != descriptor {
			return nil
		}
	}
	visited := map[string]bool{}
	return resolveParamWalk(ctx, provider, dotToInternal(receiver.RawClassName), leaf, method, descriptor, argc, index, visited, sigma)
}

// Recheck each parent's original lexical ownership before carrying enclosing
// substitutions across an extends edge. Leaf arguments do not include owners.
func lexicalReceiverBindings(ctx *class_context.ClassContext, provider ClassSigProvider, receiver *JavaParameterizedType) ([]JavaType, map[string]JavaType, []string, bool) {
	if ctx == nil || provider == nil || receiver == nil || ctx.SiblingLexicalTypeOwners == nil || len(receiver.OwnerSegments) < 2 || len(receiver.OwnerSegments) > 64 {
		return nil, nil, nil, false
	}
	path, known := ctx.SiblingLexicalTypeOwners(dotToInternal(receiver.RawClassName))
	if !known || len(path) != len(receiver.OwnerSegments) || path[len(path)-1] != dotToInternal(receiver.RawClassName) {
		return nil, nil, nil, false
	}
	sigma := map[string]JavaType{}
	var signatures []string
	var leaf []JavaType
	seen := map[string]bool{}
	for i, segment := range receiver.OwnerSegments {
		if dotToInternal(segment.BinaryName) != path[i] || seen[path[i]] {
			return nil, nil, nil, false
		}
		seen[path[i]] = true
		sig, _, ok := provider(path[i])
		if !ok || len(sig) > 65535 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(sig))+1) != nil {
			return nil, nil, nil, false
		}
		signatures = append(signatures, sig)
		formals := ClassFormalTypeParamNames(sig)
		if len(formals) != len(segment.TypeArgs) {
			return nil, nil, nil, false
		}
		if i == len(path)-1 {
			leaf = segment.TypeArgs
			for _, n := range formals {
				delete(sigma, n)
			}
			break
		}
		for j, n := range formals {
			if segment.TypeArgs[j] == nil {
				return nil, nil, nil, false
			}
			sigma[n] = segment.TypeArgs[j]
		}
	}
	if _, _, valid := EraseLexicalOwnerMethodSignatureWithThrows(signatures, "()V"); !valid {
		return nil, nil, nil, false
	}
	return leaf, sigma, signatures, true
}

// Validate the inherited method in its original declaration scope, before
// substituting caller arguments. Contradictory metadata remains unknown.
func lexicalMethodDescriptorMatches(ctx *class_context.ClassContext, provider ClassSigProvider, owner, signature, descriptor string) bool {
	if ctx == nil || provider == nil || ctx.SiblingLexicalTypeOwners == nil || descriptor == "" {
		return false
	}
	path, known := ctx.SiblingLexicalTypeOwners(owner)
	if !known || len(path) < 2 || len(path) > 64 || path[len(path)-1] != owner {
		return false
	}
	signatures := make([]string, 0, len(path))
	seen := map[string]bool{}
	for _, name := range path {
		if name == "" || seen[name] {
			return false
		}
		seen[name] = true
		sig, _, available := provider(name)
		if !available || len(sig) > 65535 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(sig))+1) != nil {
			return false
		}
		signatures = append(signatures, sig)
	}
	if len(signature) > 65535 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(signature))+1) != nil {
		return false
	}
	erased, _, valid := EraseLexicalOwnerMethodSignatureWithThrows(signatures, signature)
	return valid && erased == descriptor
}
