package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A complete, unique NON-generic declaration needs no widening cast for an
// already assignable value. Generic declarations still need their argument
// views: even with one overload, a raw Iterable cast can control inference
// between Iterable<E> and Consumer<? super E>. Unknown is not Unique.
func (f *FunctionCallExpression) unprovenWideningArgCast(actual, formal types.JavaType, ctx *class_context.ClassContext) bool {
	if f == nil || ctx == nil || f.IsStatic || f.IsSpecialInvoke || f.FunctionName == "<init>" ||
		(f.Kind != InvokeVirtual && f.Kind != InvokeInterface) || actual == nil || formal == nil ||
		!callbinding.Reference(bindingType(actual)) || !callbinding.Reference(bindingType(formal)) ||
		!provenOverloadWidening(witnessRawClassName(actual), witnessRawClassName(formal), ctx) {
		return false
	}
	if ctx.InvocationMetadata == nil {
		return false
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: strings.ReplaceAll(f.ClassName, ".", "/"), Name: f.FunctionName, Desc: f.Descriptor}, ctx.InvocationMetadata)
	return err == nil && family.Complete && family.Proof == callbinding.Unique && family.Target != nil && !family.Target.Generic
}

// A fixed child's Signature can expose a parameterized direct parent whose
// bytes are unavailable. If the child declares no method of this name, a raw
// view of that SAME parent plus the exact descriptor arguments preserves the
// erased lookup. Dropping the argument cast alone is unsafe: the hidden parent
// may also have an overload for the concrete payload type, even when the
// selected method does not use its class variable at all.
//
// This is a partial source binding view, never a completeness proof. Unknown
// access/throws declarations remain unsupported. Void, non-array descriptors
// and widening-only operands avoid new result checks, varargs or poly inference.
func (f *FunctionCallExpression) planIncompleteErasedOwner(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || f.Object == nil ||
		f.IsStatic || f.IsSpecialInvoke || f.FunctionName == "<init>" || (f.Kind != InvokeVirtual && f.Kind != InvokeInterface) {
		return nil, false
	}
	ps, ret, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || ret != "V" || len(ps) == 0 || len(ps) != len(f.Arguments) {
		return nil, false
	}
	raw, ok := types.RawClassFQN(f.Object.Type())
	if !ok || !sameErasureClassName(raw, f.ClassName) {
		return nil, false
	}
	owner := strings.ReplaceAll(raw, ".", "/")
	meta, known := ctx.InvocationMetadata(owner)
	cs, _, sigKnown := ctx.SiblingClassSig(owner)
	if !known || meta.Name != owner || !meta.MembersComplete || !meta.ParentsComplete || !sigKnown ||
		len(meta.Parents) != 1 || len(types.ClassFormalTypeParamNames(cs)) != 0 {
		return nil, false
	}
	for _, m := range meta.Methods {
		if m.Name == f.FunctionName {
			return nil, false
		}
	}
	parent, ok := types.AsParameterizedType(types.ParseSignature(cs))
	if !ok || len(parent.TypeArgs) == 0 || strings.ReplaceAll(parent.RawClassName, ".", "/") != meta.Parents[0] {
		return nil, false
	}
	if _, available := ctx.InvocationMetadata(meta.Parents[0]); available {
		return nil, false
	}
	// There must be an actual source constraint exposed by this fixed binding.
	// Otherwise the existing renderer owns the call and needs no speculative view.
	conflict := false
	for i, arg := range f.Arguments {
		if arg == nil || arg.Type() == nil || strings.HasPrefix(ps[i], "[") || isWitnessLambdaArg(UnpackSoltValue(arg)) {
			return nil, false
		}
		actual := erasedInvocationArgumentType(arg, ps[i])
		if !callbinding.Assignable(actual, ps[i], ctx.InvocationMetadata) {
			return nil, false
		}
		for _, bound := range parent.TypeArgs {
			if bound != nil && !types.IsWildcardType(bound) && actual == bindingType(bound) && actual != ps[i] {
				conflict = true
			}
		}
	}
	if !conflict {
		return nil, false
	}
	out := f.Clone()
	out.Object = &CastExpression{Value: f.Object, TargetType: types.NewJavaClass(parent.RawClassName), Binding: true, OriginPC: f.OriginPC}
	for i, p := range ps {
		if callbinding.Reference(p) {
			typ, _ := types.ParseDescriptor(p)
			out.Arguments[i] = &CastExpression{Value: f.Arguments[i], TargetType: typ, Binding: true, OriginPC: f.OriginPC}
		}
	}
	out.bindingPlanned = true
	f.noteUnknownOverloadFamily(ctx)
	return out, true
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
	return f.planErasedInvocationProof(ctx, false)
}
func (f *FunctionCallExpression) planErasedInvocationProof(ctx *class_context.ClassContext, erasedResultUse bool) (*FunctionCallExpression, bool) {
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
	if end < 0 {
		return nil, false
	}
	resultToken := sig[end+1:]
	if resultToken != result && !erasedResultUse {
		return nil, false
	}
	bounds := erasedInvocationBounds(classSig)
	if len(bounds) == 0 {
		return nil, false
	}
	if resultToken != result {
		if !strings.HasPrefix(resultToken, "T") || !strings.HasSuffix(resultToken, ";") || bounds[resultToken[1:len(resultToken)-1]] != result {
			return nil, false
		}
	}
	_, declared, _ := types.ParseMethodSignatureFull(sig, ctx)
	if len(declared) != len(params) {
		return nil, false
	}
	tokens := make([]string, len(params))
	usesClassFormal := false
	for i, param := range declared {
		if param == nil || (!erasedResultUse && param.IsArray()) || types.IsWildcardType(param) {
			return nil, false
		}
		if _, nested := types.AsParameterizedType(param); nested {
			return nil, false
		}
		base, rank := param, 0
		for base.IsArray() {
			rank++
			base = base.ElementType()
		}
		name, _ := types.RawClassFQN(base)
		if bound := bounds[name]; bound != "" {
			prefix := strings.Repeat("[", rank)
			bound = prefix + bound
			if bound != params[i] {
				return nil, false
			}
			tokens[i] = strings.Repeat("[", rank) + "T" + name + ";"
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
	if !usesClassFormal || sig != "("+strings.Join(tokens, "")+")"+resultToken {
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
	if len(args) == 0 && sameErasureClassName(raw, ctx.ClassName) {
		if receiver, ok := UnpackSoltValue(f.Object).(*JavaRef); ok && receiver.IsThis {
			// `this` is the current generic declaration, even though its stack
			// descriptor is raw. Only its own unshadowed formals can supply the
			// identity substitution; arbitrary raw locals remain unresolved.
			cs, _, known := invocationSignatureEvidence(ctx, strings.ReplaceAll(raw, ".", "/"))
			owned := types.ClassFormalTypeParamNames(cs)
			for _, name := range owned {
				for _, shadow := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
					if name == shadow {
						return nil, false
					}
				}
				if !known || !ctx.IsTypeParam(name) {
					return nil, false
				}
				args = append(args, types.NewJavaClass(name))
			}
		}
	}
	inst, _, formals := types.ResolveInstantiatedSignatureExact(ctx, func(n string) (string, map[string]string, bool) { return invocationSignatureEvidence(ctx, n) }, raw, args, f.FunctionName, f.Descriptor, len(params))
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
		family.Target == nil || !family.Target.Public || family.Target.Static || family.Target.Bridge || (family.Target.Varargs && !erasedResultUse) {
		return nil, false
	}
	for _, m := range family.Methods {
		ps, ret, e := callbinding.Descriptor(m.Desc)
		if e != nil || (m.Varargs && (!erasedResultUse || m.Desc != f.Descriptor)) || (!m.Bridge && strings.Join(ps, "") == strings.Join(params, "") && ret != result) {
			return nil, false
		}
	}
	if family.Target.Varargs {
		last := len(params) - 1
		if last < 0 || !strings.HasPrefix(params[last], "[") || f.Arguments[last] == nil || bindingType(f.Arguments[last].Type()) != params[last] {
			return nil, false
		}
	}
	conflict := false
	for i, arg := range f.Arguments {
		if arg == nil || arg.Type() == nil || isWitnessLambdaArg(UnpackSoltValue(arg)) {
			return nil, false
		}
		actual := erasedInvocationArgumentType(arg, params[i])
		if !callbinding.Assignable(actual, params[i], ctx.InvocationMetadata) || inst[i] == nil {
			return nil, false
		}
		if erasedResultUse {
			// An in-scope formal may also carry a stronger recursive bound
			// (T extends Algebra<T>). Its recovered source declaration owns
			// that relationship; descriptor-only erasure must not replace it
			// with Object arguments on a still-parameterized receiver.
			if name, ok := types.RawClassFQN(inst[i]); ok && !inst[i].IsArray() && ctx.IsTypeParam(name) && !IsNullLiteral(UnpackSoltValue(arg)) {
				return nil, false
			}
		}
		// A caller variable with the selected erasure needs no raw view only
		// when the operand actually has that same source variable. Object is
		// assignable to K's erasure, not to K itself. This distinction matters
		// inside erased SAM helpers: inventing a (K) check would change their
		// payload checks and effects. Keep the exact descriptor input instead.
		callerFormal := erasedInvocationCallerFormal(inst[i], params[i], ctx) && actual == bindingType(inst[i])
		if types.IsWildcardType(inst[i]) {
			conflict = true
		} else if source := bindingType(inst[i]); source != params[i] && !callerFormal &&
			(!callbinding.Assignable(actual, source, ctx.InvocationMetadata) || family.Proof == callbinding.Compete) {
			conflict = true
		}
	}
	if !conflict {
		return nil, false
	}
	out := f.Clone()
	receiverValue := f.Object
	if child, ok := UnpackSoltValue(receiverValue).(*FunctionCallExpression); ok {
		// The proved raw receiver use also consumes the producer at its
		// original result erasure. Propagate that use through fully proved
		// fluent edges before freezing it in a binding cast.
		if planned, ok := child.PlanErasedResultChain(ctx, "L"+owner+";"); ok {
			receiverValue = planned
		}
	}
	out.Object = &CastExpression{Value: receiverValue, TargetType: types.NewJavaClass(strings.ReplaceAll(declaring, "/", ".")), OriginPC: f.OriginPC, Binding: true}
	for i, desc := range params {
		if callbinding.Reference(desc) {
			t, _ := types.ParseDescriptor(desc)
			out.Arguments[i] = &CastExpression{Value: f.Arguments[i], TargetType: t, OriginPC: f.OriginPC, Binding: true}
		}
	}
	out.bindingPlanned = true
	return out, true
}

func erasedInvocationArgumentType(arg JavaValue, descriptor string) string {
	inner := UnpackSoltValue(arg)
	if IsNullLiteral(inner) {
		return "null"
	}
	if _, ok := inner.(*JavaClassValue); ok {
		return "Ljava/lang/Class;"
	}
	if descriptor == "Z" && erasedInvocationBooleanValue(inner, map[*TernaryExpression]uint8{}, 0) {
		return "Z"
	}
	return bindingType(arg.Type())
}

// The verifier's int stack category does not distinguish booleans. Recognize
// only 0/1 literals and ternary trees with boolean conditions and proven 0/1
// arms. Do not use coerceBooleanArgument as a proof: it can convert any int to
// a nonzero test. Planning reads the tree without rewriting or evaluating it.
// Memoization handles shared diamonds; cycles and excessive depth fail closed.
func erasedInvocationBooleanValue(v JavaValue, memo map[*TernaryExpression]uint8, depth int) bool {
	v = UnpackSoltValue(v)
	if v == nil || depth > 64 {
		return false
	}
	if t, ok := v.(*TernaryExpression); ok {
		if state := memo[t]; state != 0 {
			return state == 2
		}
		memo[t] = 1
		valid := t.Condition != nil && isBooleanTyped(t.Condition) &&
			erasedInvocationBooleanValue(t.TrueValue, memo, depth+1) &&
			erasedInvocationBooleanValue(t.FalseValue, memo, depth+1)
		if valid {
			memo[t] = 2
		}
		return valid
	}
	if lit, ok := v.(*JavaLiteral); ok && bindingType(lit.Type()) == "I" {
		value, ok := lit.Data.(int)
		return ok && (value == 0 || value == 1)
	}
	return isBooleanTyped(v)
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
				cs, methods, known := invocationSignatureEvidence(ctx, node)
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

// Discarded results and existing CHECKCASTs carry explicit erased use evidence.
// Generic class results may be viewed raw there without a new payload check.
func (f *FunctionCallExpression) PlanErasedClassResultUse(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	if ctx != nil && ctx.Getenv("JDEC_ERASED_CLASS_RESULT_USE_OFF") != "" {
		return nil, false
	}
	return f.planErasedInvocationProof(ctx, true)
}

// Platform declaration Signatures belong to this proof's evidence scope. Do
// not feed them into unrelated legacy generic inference, where unconstrained
// captures could otherwise become non-denotable source types (e.g. ? super ?).
func invocationSignatureEvidence(ctx *class_context.ClassContext, name string) (string, map[string]string, bool) {
	if ctx == nil {
		return "", nil, false
	}
	if ctx.SiblingClassSig != nil {
		if cs, m, ok := ctx.SiblingClassSig(name); ok {
			return cs, m, true
		}
	}
	if ctx.InvocationMetadata == nil {
		return "", nil, false
	}
	meta, ok := ctx.InvocationMetadata(name)
	if !ok || meta.Name != name || !meta.MembersComplete || !meta.ParentsComplete {
		return "", nil, false
	}
	m := map[string]string{}
	for _, method := range meta.Methods {
		m[class_context.MethodDescKey(method.Name, method.Desc)] = method.Signature
	}
	return meta.Signature, m, true
}
