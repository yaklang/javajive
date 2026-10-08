package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

func (f *FunctionCallExpression) lexicalParameterizedReceiver(ctx *class_context.ClassContext) *types.JavaParameterizedType {
	if f == nil || f.Object == nil || ctx == nil {
		return nil
	}
	if ref, ok := UnpackSoltValue(f.Object).(*JavaRef); ok && ref != nil && ref.IsThis {
		return f.lexicalThisReceiver(ctx)
	}
	if _, field := UnpackSoltValue(f.Object).(*RefMember); !field {
		if pt, ok := types.AsParameterizedType(f.Object.Type()); ok && len(pt.OwnerSegments) > 1 {
			return pt
		}
	}
	if ctx.Getenv("JDEC_GENERIC_PARAM_FIELD_OFF") != "" {
		return nil
	}
	typ := RecoverThisFieldInstantiatedType(ctx, f.Object)
	if typ == nil {
		typ = recoverParameterizedFieldReceiver(ctx, f.Object)
	}
	pt, ok := types.AsParameterizedType(typ)
	if !ok || len(pt.OwnerSegments) < 2 {
		return nil
	}
	// A field Signature is class-scoped even inside a method declaring an equal
	// name. It cannot be rendered as that method's independent type variable.
	for _, segment := range pt.OwnerSegments {
		for _, arg := range segment.TypeArgs {
			for _, formal := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
				if raw, known := types.RawClassFQN(arg); known && raw == formal {
					return nil
				}
			}
		}
	}
	return pt
}

// Recover THIS's implicit enclosing arguments from the same original nonstatic
// ownership proof used for source layout. Inheritance and lexical ownership
// are separate relations. Inner/method binders never stand for an outer binder.
func (f *FunctionCallExpression) lexicalThisReceiver(ctx *class_context.ClassContext) *types.JavaParameterizedType {
	if ctx.SiblingClassSig == nil || ctx.SiblingLexicalTypeOwners == nil || ctx.LexicalClassName == "" {
		return nil
	}
	raw, known := types.RawClassFQN(f.Object.Type())
	if !known || !sameErasureClassName(raw, ctx.ClassName) {
		return nil
	}
	path, known := ctx.SiblingLexicalTypeOwners(strings.ReplaceAll(ctx.ClassName, ".", "/"))
	if !known || len(path) < 2 || len(path) > 64 || !sameErasureClassName(path[len(path)-1], ctx.ClassName) {
		return nil
	}
	scopes := map[string]*class_context.ClassContext{}
	seen := map[*class_context.ClassContext]bool{}
	for scope := ctx; scope != nil; scope = scope.SourceLexicalParent {
		if seen[scope] || len(seen) >= 64 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return nil
		}
		seen[scope] = true
		scopes[strings.ReplaceAll(scope.ClassName, ".", "/")] = scope
	}
	shadowed := map[string]bool{}
	for _, name := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
		shadowed[name] = true
	}
	segments := make([]types.NestedTypeSegment, len(path))
	for i := len(path) - 1; i >= 0; i-- {
		scope := scopes[path[i]]
		if scope == nil {
			return nil
		}
		sig, _, ok := ctx.SiblingClassSig(path[i])
		if !ok || len(sig) > 65535 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(sig))+1) != nil {
			return nil
		}
		names := types.ClassFormalTypeParamNames(sig)
		local := types.ClassFormalTypeParamNames(scope.ClassSig)
		if strings.Join(names, "\x00") != strings.Join(local, "\x00") || len(names) > 0 && scope.ClassSig != sig {
			return nil
		}
		segments[i].BinaryName = strings.ReplaceAll(path[i], "/", ".")
		for _, name := range names {
			if shadowed[name] || !ctx.IsTypeParam(name) {
				return nil
			}
			segments[i].TypeArgs = append(segments[i].TypeArgs, types.NewJavaClass(name))
			shadowed[name] = true
		}
	}
	return &types.JavaParameterizedType{RawClassName: raw, TypeArgs: segments[len(segments)-1].TypeArgs, OwnerSegments: segments}
}
