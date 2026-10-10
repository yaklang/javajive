package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The bytecode enclosing field is raw, but a certified qualified-this operand
// denotes its actual lexical instance. Recover arguments from that instance's
// original declaration, never from a foreign receiver or an equal-spelled
// caller formal. The projection callback reopens the whole physical read path.
func recoverLexicalEnclosingFieldReceiver(ctx *class_context.ClassContext, field *RefMember) types.JavaType {
	if ctx == nil || field == nil || !field.HasOriginPC || ctx.SourceLexicalCapturedField == nil || ctx.SiblingClassSig == nil || ctx.LexicalClassName == "" {
		return nil
	}
	raw, known := types.RawClassFQN(field.Type())
	if !known {
		return nil
	}
	if ctx.Work != nil && ctx.Work.CheckAlloc(64*96) != nil {
		return nil
	}
	if _, proved := ctx.SourceLexicalCapturedField(field, field.OriginPC, field.Member); !proved {
		return nil
	}
	shadowed := map[string]bool{}
	for _, name := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
		shadowed[name] = true
	}
	seen := map[*class_context.ClassContext]bool{}
	classes := map[string]bool{}
	var selected *class_context.ClassContext
	for scope := ctx; scope != nil; scope = scope.SourceLexicalParent {
		if seen[scope] || len(seen) >= 64 || len(scope.ClassSig) > 65535 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(scope.ClassSig))+1) != nil {
			return nil
		}
		seen[scope] = true
		class := strings.ReplaceAll(scope.ClassName, ".", "/")
		if class == "" || classes[class] {
			return nil
		}
		classes[class] = true
		names := types.ClassFormalTypeParamNames(scope.ClassSig)
		if selected == nil && scope != ctx && sameErasureClassName(scope.ClassName, raw) {
			sig, _, available := ctx.SiblingClassSig(strings.ReplaceAll(raw, ".", "/"))
			if !available || sig != scope.ClassSig || len(names) == 0 {
				return nil
			}
			for _, name := range names {
				if shadowed[name] || !ctx.IsTypeParam(name) {
					return nil
				}
			}
			selected = scope
		}
		for _, name := range names {
			shadowed[name] = true
		}
	}
	if selected == nil {
		return nil
	}
	args := []types.JavaType{}
	for _, name := range types.ClassFormalTypeParamNames(selected.ClassSig) {
		args = append(args, types.NewJavaClass(name))
	}
	return types.NewParameterizedType(raw, args)
}
