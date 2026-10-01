package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Descriptor inequality alone does not require a source cast. An already
// assignable argument needs a widening cast only to pin a competing overload;
// widening it speculatively can erase an unavailable ancestor's generic
// formal. Unknown stays unsupported, rather than masquerading as Unique. A
// decoded CHECKCAST is part of arg itself and is never removed here.
func (f *FunctionCallExpression) unprovenWideningArgCast(actual, formal types.JavaType, ctx *class_context.ClassContext) bool {
	if f == nil || ctx == nil || f.IsStatic || f.IsSpecialInvoke || f.FunctionName == "<init>" ||
		(f.Kind != InvokeVirtual && f.Kind != InvokeInterface) || actual == nil || formal == nil ||
		!callbinding.Reference(bindingType(actual)) || !callbinding.Reference(bindingType(formal)) ||
		!provenOverloadWidening(witnessRawClassName(actual), witnessRawClassName(formal), ctx) {
		return false
	}
	switch f.overloadFamilyProof(ctx) {
	case overloadUnique:
		return true
	case overloadUnknown:
		f.noteUnknownOverloadFamily(ctx)
		return true
	}
	return false
}

// Restore a source binding view, not the receiver's semantic type. A recovered
// generic formal may reject an erased payload or let another overload steal
// the call. Widen to the actual generic declaring owner and pin the descriptor
// parameters. In particular, casting a fixed subclass to itself does NOT erase
// the type arguments of its ancestor. All new casts must be proven widenings;
// existing CHECKCAST values remain inside them, at their original positions.
//
// This proof deliberately excludes method formals, generic results, throws and
// poly arguments. Those require separate target/inference/effect evidence. An
// unchanged descriptor result cannot introduce a use-site narrowing check.
func (f *FunctionCallExpression) planErasedInvocation(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil ||
		f.Object == nil || f.IsStatic || f.IsSpecialInvoke || f.FunctionName == "<init>" ||
		(f.Kind != InvokeVirtual && f.Kind != InvokeInterface) {
		return nil, false
	}
	params, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(params) == 0 || len(params) != len(f.Arguments) {
		return nil, false
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	declaring, classSig, sig := erasedInvocationDeclaration(ctx, owner, f.FunctionName, f.Descriptor)
	if declaring == "" || !strings.HasPrefix(sig, "(") || strings.Contains(sig, "^") {
		return nil, false
	}
	end := strings.IndexByte(sig, ')')
	if end < 0 || sig[end+1:] != result { // No class or method variable in the result.
		return nil, false
	}
	bounds := erasedInvocationBounds(classSig)
	if len(bounds) == 0 {
		return nil, false
	}
	_, declared, _ := types.ParseMethodSignatureFull(sig, ctx)
	if len(declared) != len(params) {
		return nil, false
	}
	tokens := make([]string, len(params))
	usesClassFormal := false
	for i, param := range declared {
		if param == nil || param.IsArray() || types.IsWildcardType(param) {
			return nil, false
		}
		if _, nested := types.AsParameterizedType(param); nested {
			return nil, false
		}
		name, _ := types.RawClassFQN(param)
		if bound := bounds[name]; bound != "" {
			if bound != params[i] {
				return nil, false
			}
			tokens[i] = "T" + name + ";"
			usesClassFormal = true
		} else {
			tokens[i] = bindingType(param)
			if tokens[i] != params[i] {
				return nil, false
			}
		}
	}
	// Reconstruct the entire Signature with tagged variables. Substring
	// replacement could confuse TA; with a class whose binary name ends in TA.
	if !usesClassFormal || sig != "("+strings.Join(tokens, "")+")"+result {
		return nil, false
	}
	raw, args := f.receiverParamTypeArgs(ctx)
	if raw == "" {
		// A factory's value type is erased, but javac sees its declared return.
		// Read the exact factory declaration without changing the stored value.
		if inner, ok := UnpackSoltValue(f.Object).(*FunctionCallExpression); ok {
			_, ret, formals := inner.genericMethodSignature(ctx)
			if pt, ok := types.AsParameterizedType(ret); ok && len(formals) == 0 {
				raw, args = pt.RawClassName, pt.TypeArgs
			}
		}
	}
	if raw == "" {
		raw, _ = types.RawClassFQN(f.Object.Type())
	}
	inst, _, formals := types.ResolveInstantiatedSignatureExact(ctx, ctx.SiblingClassSig, raw, args, f.FunctionName, f.Descriptor, len(params))
	if len(inst) != len(params) || len(formals) != 0 {
		return nil, false
	}
	receiver := bindingType(f.Object.Type())
	if !callbinding.Assignable(receiver, "L"+owner+";", ctx.InvocationMetadata) ||
		!callbinding.Assignable(receiver, "L"+declaring+";", ctx.InvocationMetadata) {
		return nil, false
	}
	kind := callbinding.Virtual
	if f.Kind == InvokeInterface {
		kind = callbinding.Interface
	}
	// Both member tables must be complete: original-owner ancestry establishes
	// the inherited entry; declaring-owner ancestry establishes raw lookup.
	original, e := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor, Kind: kind}, ctx.InvocationMetadata)
	family, e2 := callbinding.FamilyOf(callbinding.Witness{Owner: declaring, Name: f.FunctionName, Desc: f.Descriptor, Kind: kind}, ctx.InvocationMetadata)
	metadata, known := ctx.InvocationMetadata(declaring)
	if e != nil || e2 != nil || !original.Complete || !family.Complete || !known || !metadata.Public ||
		family.Target == nil || !family.Target.Public || family.Target.Static || family.Target.Bridge || family.Target.Varargs {
		return nil, false
	}
	for _, m := range family.Methods {
		ps, ret, e := callbinding.Descriptor(m.Desc)
		if e != nil || m.Varargs || (!m.Bridge && strings.Join(ps, "") == strings.Join(params, "") && ret != result) {
			return nil, false
		}
	}
	conflict := false
	for i, arg := range f.Arguments {
		if arg == nil || arg.Type() == nil || isWitnessLambdaArg(UnpackSoltValue(arg)) {
			return nil, false
		}
		actual := bindingType(arg.Type())
		if IsNullLiteral(UnpackSoltValue(arg)) {
			actual = "null"
		} else if _, ok := UnpackSoltValue(arg).(*JavaClassValue); ok {
			actual = "Ljava/lang/Class;"
		}
		if !callbinding.Assignable(actual, params[i], ctx.InvocationMetadata) || inst[i] == nil {
			return nil, false
		}
		if types.IsWildcardType(inst[i]) {
			conflict = true
		} else if source := bindingType(inst[i]); source != params[i] && !erasedInvocationCallerFormal(inst[i], params[i], ctx) &&
			(!callbinding.Assignable(actual, source, ctx.InvocationMetadata) || family.Proof == callbinding.Compete) {
			conflict = true
		}
	}
	if !conflict {
		return nil, false
	}
	out := f.Clone()
	out.Object = &CastExpression{Value: f.Object, TargetType: types.NewJavaClass(strings.ReplaceAll(declaring, "/", ".")), OriginPC: f.OriginPC, Binding: true}
	for i, desc := range params {
		if callbinding.Reference(desc) {
			t, _ := types.ParseDescriptor(desc)
			out.Arguments[i] = &CastExpression{Value: f.Arguments[i], TargetType: t, OriginPC: f.OriginPC, Binding: true}
		}
	}
	out.bindingPlanned = true
	return out, true
}

// An in-scope caller variable with this exact erasure needs no raw view: its
// source cast erases to the same JVM parameter, so the existing generic path
// remains both valid and check-free. Method variables shadow class variables.
func erasedInvocationCallerFormal(param types.JavaType, descriptor string, ctx *class_context.ClassContext) bool {
	name, ok := types.RawClassFQN(param)
	if !ok || !ctx.IsTypeParam(name) {
		return false
	}
	for _, sig := range []string{ctx.CurrentMethodSig, ctx.ClassSig} {
		for _, formal := range types.ClassFormalTypeParamNames(sig) {
			if formal == name {
				return erasedInvocationBounds(sig)[name] == descriptor
			}
		}
	}
	return false
}

// Find a real declaration, never an arity-only inherited signature. Multiple
// equally near declarations, incomplete metadata and cycles fail closed.
func erasedInvocationDeclaration(ctx *class_context.ClassContext, owner, name, desc string) (string, string, string) {
	seen := map[string]bool{}
	level := []string{owner}
	for len(level) > 0 && len(seen) < 64 {
		next := []string{}
		found, classSig, signature := "", "", ""
		for _, node := range level {
			if seen[node] {
				continue
			}
			seen[node] = true
			meta, known := ctx.InvocationMetadata(node)
			if !known || meta.Name != node || !meta.MembersComplete || !meta.ParentsComplete {
				return "", "", ""
			}
			for _, m := range meta.Methods {
				if m.Name != name || m.Desc != desc {
					continue
				}
				cs, methods, known := ctx.SiblingClassSig(node)
				if !known || found != "" {
					return "", "", ""
				}
				found, classSig, signature = node, cs, methods[class_context.MethodDescKey(name, desc)]
			}
			next = append(next, meta.Parents...)
		}
		if found != "" {
			return found, classSig, signature
		}
		level = next
	}
	return "", "", ""
}

// Prove the FIRST bound's erasure explicitly. A dependent first bound must not
// fall back to Object, or be replaced by a later interface bound. Parameterized
// class bounds are valid: erasure discards their arguments, not their raw head.
func erasedInvocationBounds(sig string) map[string]string {
	if !strings.HasPrefix(sig, "<") {
		return nil
	}
	result := map[string]string{}
	rest := sig[1:]
	for len(rest) > 0 && rest[0] != '>' {
		colon := strings.IndexByte(rest, ':')
		if colon <= 0 || result[rest[:colon]] != "" {
			return nil
		}
		name := rest[:colon]
		rest = rest[colon:]
		for strings.HasPrefix(rest, ":") {
			rest = rest[1:]
			if strings.HasPrefix(rest, ":") { // Empty class bound; first interface is the erasure.
				continue
			}
			if !strings.HasPrefix(rest, "L") {
				return nil
			}
			depth, end := 0, -1
			for i, ch := range rest {
				switch ch {
				case '<':
					depth++
				case '>':
					depth--
				case ';':
					if depth == 0 {
						end = i
					}
				}
				if end >= 0 || depth < 0 {
					break
				}
			}
			if end < 0 {
				return nil
			}
			bound := types.ParseSignature(rest[:end+1])
			raw, ok := types.RawClassFQN(bound)
			if !ok || raw == "" {
				return nil
			}
			if result[name] == "" {
				result[name] = "L" + strings.ReplaceAll(raw, ".", "/") + ";"
			}
			rest = rest[end+1:]
		}
	}
	if !strings.HasPrefix(rest, ">") || len(result) != len(types.ClassFormalTypeParamNames(sig)) {
		return nil
	}
	return result
}
