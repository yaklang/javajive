package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// The instantiated SAM descriptor erases receiver arguments: Optional<String>
// becomes Optional even when its result is String. A materialized method ref
// then needs Function<Optional<String>,String>, not Function<Optional,String>.
// Recover only a declaration-proved receiver type variable returned by a
// zero-argument method. Keep the method reference itself: inserting a result
// CHECKCAST into a replacement lambda changes erased calls that discard it.
func methodRefReceiverType(ctx *class_context.ClassContext, fi types.JavaType, impl *values.JavaClassMember, instantiated values.JavaValue, captures int) types.JavaType {
	if impl == nil || captures != 0 || (impl.RefKind != RefInvokeVirtual && impl.RefKind != RefInvokeInterface) || impl.Description != "()Ljava/lang/Object;" {
		return fi
	}
	pt, ok := types.AsParameterizedType(fi)
	if !ok || len(pt.TypeArgs) == 0 || pt.TypeArgs[0] == nil {
		return fi
	}
	// These SAMs have exactly one receiver input, represented by type arg 0.
	switch pt.RawClassName {
	case "java.util.function.Function", "java.util.function.ToIntFunction", "java.util.function.ToLongFunction", "java.util.function.ToDoubleFunction":
	default:
		return fi
	}
	mt, err := types.ParseMethodDescriptor(t19MethodTypeDesc(instantiated))
	if err != nil || mt.FunctionType() == nil || len(mt.FunctionType().ParamTypes) != 1 {
		return fi
	}
	owner := strings.ReplaceAll(impl.Name, "/", ".")
	if samOwner, ok := types.ClassFQNOf(mt.FunctionType().ParamTypes[0]); !ok || samOwner != owner {
		return fi
	}
	receiver, ok := pt.TypeArgs[0].RawType().(*types.JavaClass)
	if !ok || receiver.Name != owner {
		return fi // keep an already parameterized target or a different receiver
	}
	classSig, methodSig := methodRefReceiverSignature(ctx, impl)
	formals := types.ClassFormalTypeParamNames(classSig)
	if len(formals) == 0 || len(types.MethodFormalTypeParamNames(methodSig)) != 0 {
		return fi
	}
	params, ret := types.ParseMethodSignature(methodSig)
	if ret == nil || len(params) != 0 {
		return fi
	}
	formal, ok := ret.RawType().(*types.JavaClass)
	if !ok {
		return fi
	}
	result := mt.FunctionType().ReturnType
	if primitive, ok := result.RawType().(*types.JavaPrimer); ok {
		wrapper := map[string]string{types.JavaInteger: "java.lang.Integer", types.JavaLong: "java.lang.Long", types.JavaDouble: "java.lang.Double"}[primitive.Name]
		if wrapper == "" {
			return fi
		}
		result = types.NewJavaClass(wrapper)
	}
	args := make([]types.JavaType, len(formals))
	found := false
	for i, name := range formals {
		args[i] = &types.JavaWildcardType{}
		if name == formal.Name {
			args[i] = result
			found = true
		}
	}
	if !found {
		return fi
	}
	updated := append([]types.JavaType(nil), pt.TypeArgs...)
	updated[0] = types.NewParameterizedType(owner, args)
	return types.NewParameterizedType(pt.RawClassName, updated)
}

func methodRefReceiverSignature(ctx *class_context.ClassContext, impl *values.JavaClassMember) (string, string) {
	owner := strings.ReplaceAll(impl.Name, ".", "/")
	if ctx != nil && ctx.SiblingClassSig != nil {
		if classSig, methods, ok := ctx.SiblingClassSig(owner); ok {
			return classSig, methods[class_context.MethodDescKey(impl.Member, impl.Description)]
		}
	}
	// JDK declarations are outside the sibling classpath. This bounded metadata
	// table supplies the same Signature evidence, without guessing from a name
	// such as "get" on an arbitrary application class.
	switch owner {
	case "java/util/Optional", "java/util/function/Supplier":
		if impl.Member == "get" {
			return "<T:Ljava/lang/Object;>Ljava/lang/Object;", "()TT;"
		}
	case "java/util/concurrent/Callable":
		if impl.Member == "call" {
			return "<V:Ljava/lang/Object;>Ljava/lang/Object;", "()TV;"
		}
	case "java/util/Iterator":
		if impl.Member == "next" {
			return "<E:Ljava/lang/Object;>Ljava/lang/Object;", "()TE;"
		}
	case "java/util/Map$Entry":
		sig := "<K:Ljava/lang/Object;V:Ljava/lang/Object;>Ljava/lang/Object;"
		if impl.Member == "getKey" {
			return sig, "()TK;"
		}
		if impl.Member == "getValue" {
			return sig, "()TV;"
		}
	}
	return "", ""
}
