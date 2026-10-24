package types

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Compare source inputs to the physical invoke erasures without formatting
// names or adding an operand cast. Hidden caller binders cannot be rebound by
// their spelling; unknown, dependent and shadowing scopes need original evidence.
func sourceBridgeParameterErasure(ctx *class_context.ClassContext, t JavaType, depth int) (string, bool) {
	if t == nil || depth > 64 || ctx != nil && ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
		return "", false
	}
	if t.IsArray() {
		element, known := sourceBridgeParameterErasure(ctx, t.ElementType(), depth+1)
		return "[" + element, known
	}
	if p, primitive := t.RawType().(*JavaPrimer); primitive {
		return "primitive:" + p.Name, true
	}
	raw, known := RawClassFQN(t)
	if !known {
		return "", false
	}
	if ctx == nil || !ctx.IsTypeParam(raw) {
		return "L" + strings.ReplaceAll(raw, ".", "/") + ";", true
	}
	if len(ctx.LexicalTypeParamSignatures) > 128 || ctx.Work != nil && ctx.Work.CheckAlloc(int64(len(ctx.LexicalTypeParamSignatures)+2)*128) != nil {
		return "", false
	}
	scopes := make([]LexicalTypeScope, 0, len(ctx.LexicalTypeParamSignatures)+2)
	retained := 0
	add := func(sig string, method bool, descriptor string) bool {
		if sig == "" {
			return true
		}
		retained += len(sig)
		if len(sig) > 65535 || retained > 1<<20 {
			return false
		}
		scopes = append(scopes, LexicalTypeScope{Signature: sig, Method: method, Descriptor: descriptor})
		return true
	}
	for i := len(ctx.LexicalTypeParamSignatures) - 1; i >= 0; i-- {
		sig := ctx.LexicalTypeParamSignatures[i]
		if !add(sig, strings.Contains(sig, "("), "") {
			return "", false
		}
	}
	if !add(ctx.ClassSig, false, "") || !add(ctx.CurrentMethodSig, true, ctx.CurrentMethodDesc) {
		return "", false
	}
	if ctx.Work != nil && (ctx.Work.CheckAlloc(int64(retained)*256+int64(len(scopes))*128) != nil || ctx.Work.Charge(workbudget.CounterGraphScans, int64(retained)*130+1) != nil) {
		return "", false
	}
	erasures, known := LexicalTypeParameterErasures(scopes, []string{raw})
	return erasures[raw], known && erasures[raw] != ""
}
