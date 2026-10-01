package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

func inferDeclaredLambdaTarget(d *Decompiler, raw types.JavaType, erased, instantiated values.JavaValue) types.JavaType {
	if known := inferLambdaTypeFromInstantiated(raw, instantiated); known != nil {
		return known
	}
	if d == nil || d.blockPartialFunctionalTarget || d.FunctionContext == nil || d.FunctionContext.SiblingClassSig == nil || raw == nil || raw.IsArray() {
		return nil
	}
	jc, ok := raw.RawType().(*types.JavaClass)
	if !ok || jc == nil {
		return nil
	}
	classSig, methods, ok := d.FunctionContext.SiblingClassSig(strings.ReplaceAll(jc.Name, ".", "/"))
	if !ok {
		return nil
	}
	erasedDesc, actualDesc := t19MethodTypeDesc(erased), t19MethodTypeDesc(instantiated)
	sig := methods[class_context.MethodDescKey(d.InvokeDynamicName, erasedDesc)]
	return inferDeclaredSamInstantiation(jc.Name, classSig, sig, erasedDesc, actualDesc, d.FunctionContext.InvocationMetadata)
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
func retainErasedFunctionalValue(v *values.CustomValue, raw, target types.JavaType) *values.CustomValue {
	if v == nil || target == nil {
		return v
	}
	copy := *v
	copy.TypeFunc = func() types.JavaType { return raw }
	if write := v.WriteFunc; write != nil {
		copy.WriteFunc = func(ctx *class_context.ClassContext, out *workbudget.Writer) error {
			if err := out.WriteString("(" + raw.String(ctx) + ") ((" + target.String(ctx) + ") ("); err != nil {
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
