package core

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
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
func methodRefErasedReceiver(ctx *class_context.ClassContext, impl *values.JavaClassMember, actual values.JavaValue, captured []values.JavaValue, captureDescriptor ...string) []values.JavaValue {
	if ctx == nil || ctx.SiblingClassSig == nil || impl == nil || len(captured) != 1 || captured[0] == nil || (impl.RefKind != RefInvokeVirtual && impl.RefKind != RefInvokeInterface) {
		return captured
	}
	receiver, ok := types.AsParameterizedType(captured[0].Type())
	owner := strings.ReplaceAll(impl.Name, "/", ".")
	classSig, methods, known := ctx.SiblingClassSig(strings.ReplaceAll(owner, ".", "/"))
	if !known && ctx.InvocationMetadata != nil {
		meta, ok := ctx.InvocationMetadata(strings.ReplaceAll(owner, ".", "/"))
		if ok && meta.Name == strings.ReplaceAll(owner, ".", "/") && meta.MembersComplete && meta.ParentsComplete {
			classSig, known = meta.Signature, true
			methods = map[string]string{}
			for _, method := range meta.Methods {
				methods[class_context.MethodDescKey(method.Name, method.Desc)] = method.Signature
			}
		}
	}
	formals := types.ClassFormalTypeParamNames(classSig)
	// A raw descriptor for `this` still denotes the class's in-scope type
	// variables. Reconstruct those bindings only for the exact current owner.
	if !ok && strings.ReplaceAll(ctx.ClassName, "/", ".") == owner {
		if ref, yes := values.UnpackSoltValue(captured[0]).(*values.JavaRef); yes && methodRefCopiesThis(ref) {
			args := make([]types.JavaType, len(formals))
			for i, name := range formals {
				args[i] = types.NewJavaClass(name)
			}
			receiver, _ = types.AsParameterizedType(types.NewParameterizedType(owner, args))
			ok = true
		}
	}
	if result := methodRefResultBinding(ctx, impl, actual, captured, classSig, methods, known, captureDescriptor...); result != nil {
		return result
	}
	implTokens, implOK := directSamTokens(impl.Description, false)
	actualTokens, actualOK := directSamTokens(t19MethodTypeDesc(actual), false)
	if !ok || receiver.RawClassName != owner || !implOK || !actualOK || len(implTokens) != len(actualTokens) {
		return captured
	}
	for i, token := range implTokens {
		// A void SAM intentionally discards the implementation's result.
		// Keep the method reference intact: no result CHECKCAST is introduced.
		if token != actualTokens[i] && !(i == len(implTokens)-1 && actualTokens[i] == "V") {
			return captured
		}
	}
	if !known || len(formals) == 0 || len(formals) != len(receiver.TypeArgs) {
		return captured
	}
	signature := methods[class_context.MethodDescKey(impl.Member, impl.Description)]
	body, _, valid := directSamThrows(signature)
	declared, dok := methodRefDeclarationTokens(body, ctx)
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
			return captured
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

// Nested parameterizations erase to their class head. They constrain Java
// source binding but do not change a descriptor-exact method reference's
// argument checks. Keep variable tags for bare class formals; method formals
// and arrays of variables remain outside this receiver-only proof.
func methodRefDeclarationTokens(body string, ctx *class_context.ClassContext) ([]string, bool) {
	if tokens, ok := directSamTokens(body, true); ok {
		return tokens, true
	}
	if !strings.HasPrefix(body, "(") {
		return nil, false
	}
	_, params, result := types.ParseMethodSignatureFull(body, ctx)
	if result == nil {
		return nil, false
	}
	all := append(append([]types.JavaType(nil), params...), result)
	tokens := make([]string, len(all))
	for i, typ := range all {
		if p, ok := types.AsParameterizedType(typ); ok && !typ.IsArray() {
			tokens[i] = "L" + strings.ReplaceAll(p.RawClassName, ".", "/") + ";"
			continue
		}
		// A non-parameterized token may be primitive, concrete or a variable;
		// reconstruct only identities recoverable without a scope guess.
		if typ == nil || typ.IsArray() {
			return nil, false
		}
		if raw, ok := types.RawClassFQN(typ); ok {
			if strings.Contains(raw, ".") || strings.Contains(raw, "/") {
				tokens[i] = "L" + strings.ReplaceAll(raw, ".", "/") + ";"
			} else {
				tokens[i] = "T" + raw + ";"
			}
		} else {
			primitive := map[string]string{"void": "V", "boolean": "Z", "byte": "B", "char": "C", "short": "S", "int": "I", "long": "J", "float": "F", "double": "D"}
			tokens[i] = primitive[typ.String(ctx)]
			if tokens[i] == "" {
				return nil, false
			}
		}
	}
	return tokens, true
}

func methodRefCopiesThis(ref *values.JavaRef) bool {
	if ref == nil {
		return false
	}
	if ref.IsThis {
		return true
	}
	// Invokedynamic operands are immutable snapshots. Only `this` has an
	// immutable language identity that can be recovered from a direct copy.
	source, ok := values.UnpackSoltValue(ref.Val).(*values.JavaRef)
	return ok && source != nil && source.IsThis
}

// A bare class-formal result can recover its source view from the bootstrap's
// instantiated result. This is an unchecked SAME-owner view: the method
// reference itself retains its eager receiver check and original result cast.
// Inputs must remain descriptor-exact and must not depend on that formal.
func methodRefResultBinding(ctx *class_context.ClassContext, impl *values.JavaClassMember, actual values.JavaValue, captured []values.JavaValue, cs string, methods map[string]string, known bool, captureDescriptor ...string) []values.JavaValue {
	if !known || ctx.InvocationMetadata == nil {
		return nil
	}
	owner := strings.ReplaceAll(impl.Name, "/", ".")
	raw, ok := types.RawClassFQN(captured[0].Type())
	if !ok || strings.ReplaceAll(raw, "/", ".") != owner {
		// The invokedynamic descriptor is the typed capture ABI. Stack-local
		// widening can lose that receiver type; it cannot change this ABI.
		if len(captureDescriptor) != 1 {
			return nil
		}
		inputs, _, err := callbinding.Descriptor(captureDescriptor[0])
		if err != nil || len(inputs) != 1 || inputs[0] != "L"+strings.ReplaceAll(owner, ".", "/")+";" {
			return nil
		}
	}
	formals := types.ClassFormalTypeParamNames(cs)
	if len(formals) != 1 {
		return nil
	}
	body, _, valid := directSamThrows(methods[class_context.MethodDescKey(impl.Member, impl.Description)])
	declared, dok := methodRefDeclarationTokens(body, ctx)
	erased, eok := directSamTokens(impl.Description, false)
	target, tok := directSamTokens(t19MethodTypeDesc(actual), false)
	if !valid || !dok || !eok || !tok || len(declared) != len(erased) || len(target) != len(erased) {
		return nil
	}
	last := len(erased) - 1
	if last < 0 || declared[last] != "T"+formals[0]+";" || !callbinding.Reference(target[last]) || target[last] == erased[last] {
		return nil
	}
	bound := types.ClassFormalTypeParamErasures(cs)[formals[0]]
	if bound == "" || erased[last] != "L"+strings.ReplaceAll(bound, ".", "/")+";" || !callbinding.Assignable(target[last], erased[last], ctx.InvocationMetadata) {
		return nil
	}
	for i := 0; i < last; i++ {
		if declared[i] != erased[i] || target[i] != erased[i] {
			return nil
		}
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: strings.ReplaceAll(owner, ".", "/"), Name: impl.Member, Desc: impl.Description}, ctx.InvocationMetadata)
	if err != nil || !family.Complete || family.Target == nil || family.Target.Bridge || family.Target.Static || family.Target.Varargs {
		return nil
	}
	inferred, err := types.ParseDescriptor(target[last])
	if err != nil {
		return nil
	}
	result := append([]values.JavaValue(nil), captured...)
	result[0] = &values.CastExpression{Value: captured[0], TargetType: types.NewParameterizedType(owner, []types.JavaType{inferred}), Binding: true}
	return result
}
