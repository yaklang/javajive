package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// ProjectOriginalSamTarget projects a generic declaration onto the two original
// metafactory descriptors. It supplies a type only: the caller must separately
// prove the declaration's identity, source member binding, eager operands and
// complete bootstrap protocol. It never evaluates a JavaValue/source callback.
func ProjectOriginalSamTarget(owner, classSignature, samSignature, erased, instantiated string, provider callbinding.Provider) types.JavaType {
	name := strings.ReplaceAll(owner, "/", ".")
	// A monomorphic declaration has an empty substitution environment. Its
	// projection is identity, not a failed generic inference. Do not apply this
	// rule to a nonempty class Signature whose inherited variables are unknown.
	if classSignature == "" {
		declared := erased
		if samSignature != "" {
			body, variables, known := directSamThrows(samSignature)
			if !known || len(variables) != 0 {
				return nil
			}
			declared = body
		}
		if name == "" || declared != erased || erased != instantiated {
			return nil
		}
		if _, known := directSamTokens(declared, false); !known {
			return nil
		}
		return types.NewJavaClass(name)
	}
	return inferDeclaredSamInstantiation(name, classSignature, samSignature, erased, instantiated, provider)
}

func inferDeclaredLambdaTarget(d *Decompiler, raw types.JavaType, erased, instantiated values.JavaValue) types.JavaType {
	if known := inferLambdaTypeFromInstantiated(raw, instantiated); known != nil {
		return known
	}
	if d == nil || d.blockPartialFunctionalTarget || d.FunctionContext == nil || raw == nil || raw.IsArray() {
		return nil
	}
	declarations := d.FunctionContext.LambdaTargetClassSig
	if declarations == nil {
		declarations = d.FunctionContext.SiblingClassSig
	}
	if declarations == nil {
		return nil
	}
	name, known := types.RawClassFQN(raw)
	_, variable := types.TypeVariableName(raw)
	if !known || name == "" || variable {
		return nil
	}
	if parameterized, known := types.AsParameterizedType(raw); known && (len(parameterized.OwnerSegments) > 1 || len(parameterized.TypeArgs) > 255) {
		return nil
	}
	// Earlier call inference may already carry a parameterized view. Its
	// arguments are not the SAM proof: recover only its physical owner here,
	// then solve again from the original declaration and bootstrap descriptors.
	classSig, methods, ok := declarations(strings.ReplaceAll(name, ".", "/"))
	if !ok {
		return nil
	}
	erasedDesc, actualDesc := t19MethodTypeDesc(erased), t19MethodTypeDesc(instantiated)
	sig, declared := methods[class_context.MethodDescKey(d.InvokeDynamicName, erasedDesc)]
	if !declared {
		sig = inheritedDeclaredSamSignature(d.FunctionContext, declarations, strings.ReplaceAll(name, ".", "/"), d.InvokeDynamicName, erasedDesc)
	}
	return inferDeclaredSamInstantiation(name, classSig, sig, erasedDesc, actualDesc, d.FunctionContext.InvocationMetadata)
}

// Bind declaration variables by their Signature identity, not SAM parameter order.
// This conservative proof handles direct occurrences of class
// variables only. A variable appearing only in the throws clause is existential:
// its bootstrap descriptor contains no binding, so retain a wildcard rather than
// inventing Throwable (or a caller's same-spelled variable). Inherited SAMs,
// nested generic substitutions and dependent/intersection bounds still need
// additional evidence and remain unchanged.
func inferDeclaredSamInstantiation(name, classSig, sig, erased, actual string, providers ...callbinding.Provider) types.JavaType {
	var provider callbinding.Provider
	if len(providers) == 1 {
		provider = providers[0]
	}
	formals := types.ClassFormalTypeParamNames(classSig)
	if len(formals) == 0 {
		return nil
	}
	// Keep Signature variable tags separate from class names. Only direct throws
	// variables are eligible for existential arguments; malformed/foreign tokens
	// and dependent/intersection bounds fail the exact prefix check below.
	body, throws, valid := directSamThrows(sig)
	if !valid {
		return nil
	}
	erasures := types.ClassFormalTypeParamErasures(classSig)
	prefix := "<"
	known := map[string]bool{}
	for _, f := range formals {
		if known[f] {
			return nil
		}
		known[f] = true
		bound := "java.lang.Object"
		if erased := erasures[f]; erased != "" && (throws[f] || provider != nil) {
			bound = erased
		}
		separator := ":"
		if provider != nil && strings.HasPrefix(classSig, prefix+f+"::L") {
			separator = "::"
		}
		prefix += f + separator + "L" + strings.ReplaceAll(bound, ".", "/") + ";"
	}
	prefix += ">"
	if !strings.HasPrefix(classSig, prefix) {
		return nil
	}
	for variable := range throws {
		if !known[variable] {
			return nil
		}
	}
	declared, ok := directSamTokens(body, true)
	if !ok {
		return nil
	}
	erasedTokens, ok := directSamTokens(erased, false)
	if !ok {
		return nil
	}
	actualTokens, ok := directSamTokens(actual, false)
	if !ok || len(declared) != len(erasedTokens) || len(declared) != len(actualTokens) {
		return nil
	}
	substitutions := map[string]string{}
	for i, token := range declared {
		if strings.HasPrefix(token, "T") {
			variable := token[1 : len(token)-1]
			if !known[variable] || (throws[variable] && erasures[variable] != "java.lang.Object") || erasedTokens[i] != "L"+strings.ReplaceAll(erasures[variable], ".", "/")+";" || !(strings.HasPrefix(actualTokens[i], "L") || strings.HasPrefix(actualTokens[i], "[")) {
				return nil
			}
			if erasedTokens[i] != "Ljava/lang/Object;" && (provider == nil || !callbinding.Assignable(actualTokens[i], erasedTokens[i], provider)) {
				return nil
			}
			if prior, exists := substitutions[variable]; exists && prior != actualTokens[i] {
				return nil
			}
			substitutions[variable] = actualTokens[i]
		} else if token != erasedTokens[i] || token != actualTokens[i] {
			return nil
		}
	}
	args := make([]types.JavaType, len(formals))
	for i, f := range formals {
		desc, exists := substitutions[f]
		if !exists {
			if !throws[f] {
				return nil
			}
			args[i] = &types.JavaWildcardType{}
			continue
		}
		typ, err := types.ParseDescriptor(desc)
		if err != nil || typ == nil {
			return nil
		}
		args[i] = typ
	}
	return types.NewParameterizedType(name, args)
}

// directSamThrows separates a direct SAM signature's optional throws clauses.
// Throws do not occur in a JVM method descriptor, so they must not participate
// in input/result matching. Concrete exception classes need no type binding.
func directSamThrows(sig string) (string, map[string]bool, bool) {
	parts := strings.Split(sig, "^")
	variables := map[string]bool{}
	for _, token := range parts[1:] {
		if len(token) < 3 || token[len(token)-1] != ';' || strings.ContainsAny(token[1:len(token)-1], "<>():.[;") {
			return "", nil, false
		}
		switch token[0] {
		case 'T':
			variables[token[1:len(token)-1]] = true
		case 'L':
		default:
			return "", nil, false
		}
	}
	return parts[0], variables, true
}

// Tokens retain TT; versus LT;: the generic type parser represents both as a
// JavaClass named T, which cannot by itself prove variable identity. Return is
// included after all parameters; void is permitted only in the return position.
func directSamTokens(desc string, variables bool) ([]string, bool) {
	if !strings.HasPrefix(desc, "(") {
		return nil, false
	}
	at := 1
	var result []string
	read := func(allowVoid bool) (string, bool) {
		start := at
		if at >= len(desc) {
			return "", false
		}
		for at < len(desc) && desc[at] == '[' {
			at++
		}
		array := at > start
		if at >= len(desc) {
			return "", false
		}
		switch desc[at] {
		case 'T':
			if !variables || array {
				return "", false
			}
			fallthrough
		case 'L':
			end := strings.IndexByte(desc[at:], ';')
			if end < 2 {
				return "", false
			}
			part := desc[at : at+end+1]
			if strings.ContainsAny(part, "<>():.[") {
				return "", false
			}
			at += end + 1
		case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
			at++
		case 'V':
			if !allowVoid || array {
				return "", false
			}
			at++
		default:
			return "", false
		}
		return desc[start:at], true
	}
	for at < len(desc) && desc[at] != ')' {
		token, ok := read(false)
		if !ok {
			return nil, false
		}
		result = append(result, token)
	}
	if at >= len(desc) || desc[at] != ')' {
		return nil, false
	}
	at++
	token, ok := read(true)
	if !ok || at != len(desc) {
		return nil, false
	}
	return append(result, token), true
}

// Instantiation descriptors erase nested generic arguments. Target the poly
// expression at that proven SAM without claiming those erased arguments are a
// complete invariant local type. A raw local retains legal unchecked conversion
// to a later, declaration-proved Foo<Collection<String>> consumer.
func retainErasedFunctionalValue(v *values.CustomValue, raw, target types.JavaType, markers ...types.JavaType) *values.CustomValue {
	if v == nil || target == nil {
		return v
	}
	if retained, known := v.RetainMethodReferenceTarget(raw, target, markers...); known {
		return retained
	}
	copy := *v
	copy.TypeFunc = func() types.JavaType { return raw }
	if write := v.WriteFunc; write != nil {
		copy.WriteFunc = func(ctx *class_context.ClassContext, out *workbudget.Writer) error {
			creation := target.String(ctx)
			for _, marker := range markers {
				creation += " & " + marker.String(ctx)
			}
			if err := out.WriteString("(" + raw.String(ctx) + ") ((" + creation + ") ("); err != nil {
				return err
			}
			if err := write(ctx, out); err != nil {
				return err
			}
			return out.WriteString("))")
		}
	} else if render := v.StringFunc; render != nil {
		// Preserve the existing bounded-render rejection for string-only callbacks.
		copy.StringFunc = func(ctx *class_context.ClassContext) string {
			return "(" + raw.String(ctx) + ") ((" + target.String(ctx) + ") (" + render(ctx) + "))"
		}
	}
	return &copy
}

// Bootstrap descriptors fix the SAM's entry checks, but erase nested type
// arguments. A known generic class in an input/result descriptor is therefore
// a partial target, not proof of an invariant local Function<Input,RawBox>.
// Keep the checked poly target and an erased local view independently.
func instantiatedSAMErasesGenericArguments(instantiated values.JavaValue, ctx *class_context.ClassContext) bool {
	if ctx == nil || ctx.InvocationMetadata == nil {
		return false
	}
	ps, ret, err := callbinding.Descriptor(t19MethodTypeDesc(instantiated))
	if err != nil {
		return false
	}
	for _, token := range append(ps, ret) {
		token = strings.TrimLeft(token, "[")
		if len(token) < 3 || token[0] != 'L' || token[len(token)-1] != ';' {
			continue
		}
		metadata, ok := ctx.InvocationMetadata(token[1 : len(token)-1])
		if ok && len(types.ClassFormalTypeParamNames(metadata.Signature)) > 0 {
			return true
		}
	}
	return false
}
