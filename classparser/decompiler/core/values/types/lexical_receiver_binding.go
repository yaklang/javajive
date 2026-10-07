package types

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
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
	path, known := ctx.SiblingLexicalTypeOwners(dotToInternal(receiver.RawClassName))
	if !known || len(path) != len(receiver.OwnerSegments) || path[len(path)-1] != dotToInternal(receiver.RawClassName) {
		return nil
	}
	sigma := map[string]JavaType{}
	var signatures []string
	var leaf []JavaType
	for i, segment := range receiver.OwnerSegments {
		if dotToInternal(segment.BinaryName) != path[i] {
			return nil
		}
		sig, _, ok := provider(path[i])
		if !ok {
			return nil
		}
		signatures = append(signatures, sig)
		formals := ClassFormalTypeParamNames(sig)
		if len(formals) != len(segment.TypeArgs) {
			return nil
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
				return nil
			}
			sigma[n] = segment.TypeArgs[j]
		}
	}
	if _, _, valid := EraseLexicalOwnerMethodSignatureWithThrows(signatures, "()V"); !valid {
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
