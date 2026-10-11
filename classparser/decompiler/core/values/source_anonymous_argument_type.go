package values

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func sourceAnonymousArgumentType(ctx *class_context.ClassContext, allocation *NewExpression) types.JavaType {
	if ctx == nil || ctx.SourceAnonymousParentSignature == nil || allocation == nil || allocation.JavaType == nil || reflect.ValueOf(allocation.JavaType).Kind() == reflect.Pointer && reflect.ValueOf(allocation.JavaType).IsNil() {
		return nil
	}
	if allocation.IsArray() || !allocation.HasOriginPC || allocation.ConstructorCall == nil {
		return nil
	}
	call := allocation.ConstructorCall
	owner, known := types.RawClassFQN(allocation.JavaType)
	if !known || strings.ReplaceAll(owner, ".", "/") != strings.ReplaceAll(call.ClassName, ".", "/") || call.Object != allocation || call.FunctionName != "<init>" ||
		call.IsStatic || call.Kind != InvokeSpecial || !call.IsSpecialInvoke || !call.HasOriginPC {
		return nil
	}
	signature, known := ctx.SourceAnonymousParentSignature(call.ClassName, call.Descriptor, ctx.ClassName, ctx.FunctionName, ctx.CurrentMethodDesc, allocation.OriginPC, call.OriginPC)
	if !known || len(signature) > 8192 {
		return nil
	}
	if ctx.Work != nil && (ctx.Work.Charge(workbudget.CounterGraphEdges, int64(len(signature)+1)) != nil || ctx.Work.CheckAlloc(int64(len(signature))*32) != nil) {
		return nil
	}
	depth := 0
	for _, ch := range signature {
		if ch == '<' {
			depth++
			if depth > 32 {
				return nil
			}
		}
		if ch == '>' {
			depth--
			if depth < 0 {
				return nil
			}
		}
	}
	if depth != 0 {
		return nil
	}
	formals, _, valid := types.SignatureTypeVariableReferences(signature)
	if !valid || len(formals) != 0 {
		return nil
	}
	parent, interfaces := types.ParseClassSignatureSupers(signature)
	if len(interfaces) == 1 {
		raw, known := types.RawClassFQN(parent)
		if !known || raw != "java.lang.Object" {
			return nil
		}
		parent = interfaces[0]
	} else if len(interfaces) != 0 {
		return nil
	}
	if _, known := SourceTypeErasure(parent, ctx); !known || !sourceDenotableJavaType(parent, ctx) {
		return nil
	}
	return parent.Copy()
}
