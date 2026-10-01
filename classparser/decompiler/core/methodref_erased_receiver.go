package core

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A bound method reference can capture a parameterized receiver while its SAM
// accepts the selected method's erased parameters. Targeting that same reference
// through a wildcard capture instead imposes a source constraint absent from the
// bootstrap. Restore an erased receiver view only when owner, declaration and
// all instantiated input/result descriptors agree exactly. The reference stays
// intact: eager receiver checks, dispatch, unboxing and discarded results retain
// their original adaptation. The local's type and identity are never changed.
func methodRefErasedReceiver(ctx *class_context.ClassContext, impl *values.JavaClassMember, actual values.JavaValue, captured []values.JavaValue) []values.JavaValue {
	if ctx == nil || ctx.SiblingClassSig == nil || impl == nil || len(captured) != 1 || captured[0] == nil || (impl.RefKind != RefInvokeVirtual && impl.RefKind != RefInvokeInterface) {
		return captured
	}
	receiver, ok := types.AsParameterizedType(captured[0].Type())
	owner := strings.ReplaceAll(impl.Name, "/", ".")
	if !ok || receiver.RawClassName != owner || t19MethodTypeDesc(actual) != impl.Description {
		return captured
	}
	classSig, methods, known := ctx.SiblingClassSig(strings.ReplaceAll(owner, ".", "/"))
	formals := types.ClassFormalTypeParamNames(classSig)
	if !known || len(formals) == 0 || len(formals) != len(receiver.TypeArgs) {
		return captured
	}
	signature := methods[class_context.MethodDescKey(impl.Member, impl.Description)]
	body, _, valid := directSamThrows(signature)
	declared, dok := directSamTokens(body, true)
	erased, eok := directSamTokens(impl.Description, false)
	if !valid || !dok || !eok || len(declared) != len(erased) {
		return captured
	}
	bindings := map[string]types.JavaType{}
	for i, name := range formals {
		bindings[name] = receiver.TypeArgs[i]
	}
	bounds := types.ClassFormalTypeParamErasures(classSig)
	conflict := false
	for i, token := range declared {
		if !strings.HasPrefix(token, "T") {
			if token != erased[i] {
				return captured
			}
			continue
		}
		name := token[1 : len(token)-1]
		arg, known := bindings[name]
		if !known || arg == nil {
			return captured
		}
		bound := bounds[name]
		if bound == "" {
			bound = "java.lang.Object"
		}
		if erased[i] != "L"+strings.ReplaceAll(bound, ".", "/")+";" {
			return captured
		}
		// Only an input mismatch needs this view; erasing an unconstrained return
		// alone is unnecessary. Wildcards are checked before RawType (nil embed).
		if i < len(declared)-1 {
			typ, err := types.ParseDescriptor(erased[i])
			if err != nil {
				return captured
			}
			if types.IsWildcardType(arg) || !reflect.DeepEqual(arg.RawType(), typ.RawType()) {
				conflict = true
			}
		}
	}
	if !conflict {
		return captured
	}
	result := append([]values.JavaValue(nil), captured...)
	result[0] = &values.CastExpression{Binding: true, Value: captured[0], TargetType: types.NewJavaClass(owner)}
	return result
}
