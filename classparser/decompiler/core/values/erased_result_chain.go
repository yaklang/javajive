package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

// A JVM CHECKCAST consumes its producer at the producer's descriptor erasure,
// even when the check narrows that result (Map -> LinkedHashMap). Keep the
// original cast outside the planned call. Caller formals and parameterized
// targets carry extra source constraints and cannot use this raw proof.
func (f *FunctionCallExpression) PlanErasedCheckedResultChain(ctx *class_context.ClassContext, target types.JavaType) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || target == nil || target.IsArray() {
		return nil, false
	}
	if _, raw := target.RawType().(*types.JavaClass); !raw {
		return nil, false
	}
	name, reference := types.RawClassFQN(target)
	if !reference || ctx.IsTypeParam(name) {
		return nil, false
	}
	for _, sig := range []string{ctx.CurrentMethodSig, ctx.ClassSig} {
		for _, formal := range types.ClassFormalTypeParamNames(sig) {
			if formal == name {
				return nil, false
			}
		}
	}
	_, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil {
		return nil, false
	}
	return f.PlanErasedResultChain(ctx, result)
}

// The parsed method's FunctionType may still contain only the descriptor's
// raw return while its Signature declares Container<E>. That exact declaration
// supplies the same erased result-use proof as an already-instantiated type.
func (f *FunctionCallExpression) PlanErasedDeclaredReturnChain(ctx *class_context.ClassContext) (*FunctionCallExpression, types.JavaType, bool) {
	if f == nil || ctx == nil {
		return nil, nil, false
	}
	_, _, target := types.ParseMethodSignatureFull(ctx.CurrentMethodSig, ctx)
	if _, parameterized := types.AsParameterizedType(target); !parameterized || target.IsArray() {
		return nil, nil, false
	}
	_, result, err := callbinding.Descriptor(ctx.CurrentMethodDesc)
	if err != nil || bindingType(target) != result {
		return nil, nil, false
	}
	planned, ok := f.PlanErasedResultChain(ctx, result)
	return planned, target, ok
}

// PlanErasedResultChain restores a descriptor-exact argument tuple only where
// the consumer supplies the identical result erasure. It can cross fluent
// receiver edges whose producer result is exactly the next invoke owner.
// This proof does not infer an unavailable API Signature: a raw owner and
// exact descriptor argument types preserve erased lookup. Poly expressions,
// object allocations, narrowed operands and unrelated result contexts fail
// closed. Materialized functional values retain their own SAM entry checks.
func (f *FunctionCallExpression) PlanErasedResultChain(ctx *class_context.ClassContext, result string) (*FunctionCallExpression, bool) {
	return f.planErasedResultChain(ctx, result, 0)
}
func (f *FunctionCallExpression) planErasedResultChain(ctx *class_context.ClassContext, result string, depth int) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || depth > 32 || f.IsSpecialInvoke || f.FunctionName == "<init>" || f.Kind == InvokeDynamic || ctx.Getenv("JDEC_ERASED_RESULT_CHAIN_OFF") != "" {
		return nil, false
	}
	// Prefer the existing Signature-based functional view when its receiver
	// instantiation is known. Erasing that receiver would discard a stronger
	// proof and override the functional-feature diagnostic switches.
	if ctx.Getenv("JDEC_FUNCTIONAL_ERASURE_RESOLVE_OFF") != "" || ctx.Getenv("JDEC_GENERIC_PARAM_RECV_METHOD_OFF") != "" {
		return nil, false
	}
	strongerFunctionalView := false
	for i, arg := range f.Arguments {
		if f.nestedGenericErasureArgCast(i, arg, ctx) != "" {
			strongerFunctionalView = true
		}
	}
	ps, ret, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || ret != result || !callbinding.Reference(ret) || strings.HasPrefix(ret, "[") || len(ps) != len(f.Arguments) {
		return nil, false
	}
	out := f.Clone()
	changed := false
	owner := "L" + strings.ReplaceAll(f.ClassName, ".", "/") + ";"
	valid := true
	hasParameterized := false
	for i, arg := range f.Arguments {
		if arg == nil || arg.Type() == nil {
			valid = false
			break
		}
		actual := UnpackSoltValue(arg)
		switch actual.(type) {
		case *JavaRef:
			ref := actual.(*JavaRef)
			if ref.StackVar != nil || ref.CustomValue != nil {
				valid = false
			}
		case *RefMember, *JavaClassMember, *JavaClassValue, *JavaLiteral, *ArrayLengthExpression:
		case *FunctionCallExpression:
			child := actual.(*FunctionCallExpression)
			_, childResult, err := callbinding.Descriptor(child.Descriptor)
			if err != nil || !callbinding.Assignable(childResult, ps[i], ctx.InvocationMetadata) {
				valid = false
			}
			if ctx.InvocationMetadata != nil {
				_, _, sig := erasedInvocationDeclaration(ctx, strings.ReplaceAll(child.ClassName, ".", "/"), child.FunctionName, child.Descriptor)
				_, _, sourceResult := types.ParseMethodSignatureFull(sig, ctx)
				if _, generic := types.AsParameterizedType(sourceResult); generic {
					// The child's descriptor type can be raw even though Java
					// re-infers a parameterized result from its declaration. Pin
					// this consuming edge as well as already-parameterized locals.
					hasParameterized = true
				}
			}
		case *CastExpression:
			// An explicit parameterized target fixes the SAM context before
			// the containing invocation is erased. Keep this inner cast and
			// its checks; a bare lambda has no such independent target.
			cast := actual.(*CastExpression)
			_, pinned := types.AsParameterizedType(cast.TargetType)
			fixedRawTarget := false
			if isWitnessLambdaArg(cast.Value) && cast.Value.Type() != nil {
				_, genericOperand := types.AsParameterizedType(cast.Value.Type())
				fixedRawTarget = !genericOperand && bindingType(cast.Value.Type()) == bindingType(cast.TargetType)
			}
			if cast.Value == nil || (!pinned && !fixedRawTarget && isWitnessLambdaArg(UnpackSoltValue(cast.Value))) {
				valid = false
			}
		case *NewExpression:
			// Packed arrays have an explicit component type and rank. The
			// exact descriptor comparison below excludes covariance casts;
			// changing the containing receiver cannot retarget an initializer.
			if !actual.(*NewExpression).IsArray() {
				valid = false
			}
		default:
			valid = false
		}
		if isWitnessLambdaArg(actual) || !callbinding.Assignable(erasedInvocationArgumentType(arg, ps[i]), ps[i], ctx.InvocationMetadata) {
			valid = false
		}
		if _, ok := types.AsParameterizedType(arg.Type()); ok {
			hasParameterized = true
		}
	}
	if !f.IsStatic && (f.Object == nil || f.Object.Type() == nil || bindingType(f.Object.Type()) != owner) {
		valid = false
	}
	// A raw producer erases the next receiver's parameter types too. Never
	// cross a use with unpinned poly or narrowed operands merely because the child
	// alone is provable; every traversed call must have an exact fixed tuple.
	if !valid {

		return nil, false
	}
	// A pre-existing cast fixes its operand's result context independently.
	// Traverse it without moving/removing the check or changing its target.
	refine := func(value JavaValue, expected string) (JavaValue, bool) {
		cast, casted := UnpackSoltValue(value).(*CastExpression)
		inner := value
		if casted {
			inner = cast.Value
		}
		child, ok := UnpackSoltValue(inner).(*FunctionCallExpression)
		if !ok {
			return value, false
		}
		if casted {
			_, result, err := callbinding.Descriptor(child.Descriptor)
			if err != nil {
				return value, false
			}
			expected = result
		} else if _, childResult, err := callbinding.Descriptor(child.Descriptor); err == nil && callbinding.Assignable(childResult, expected, ctx.InvocationMetadata) {
			// A known widening does not change the child's own result erasure.
			// The parent pins its descriptor at this edge after refinement.
			expected = childResult
		}
		planned, ok := child.planErasedResultChain(ctx, expected, depth+1)
		if !ok {
			return value, false
		}
		if casted {
			copy := *cast
			copy.Value = planned
			return &copy, true
		}
		return planned, true
	}
	if !f.IsStatic {
		if receiver, ok := refine(f.Object, owner); ok {
			out.Object, changed = receiver, true
		}
	}
	for i, arg := range f.Arguments {
		if operand, ok := refine(arg, ps[i]); ok {
			out.Arguments[i], changed = operand, true
			// Refining an inner generic invocation changes Java inference at
			// this argument edge. Pin the containing erased tuple as well;
			// otherwise the parent can infer the inner call as Function<Object,
			// Object> instead of consuming its original raw Function result.
			hasParameterized = true
		}
	}
	if valid && hasParameterized && !f.bindingPlanned && !strongerFunctionalView {
		sourceOwner := strings.ReplaceAll(f.ClassName, "/", ".")
		if !f.IsStatic && ctx.InvocationMetadata != nil {
			declaring, signature, _ := erasedInvocationDeclaration(ctx, strings.ReplaceAll(f.ClassName, ".", "/"), f.FunctionName, f.Descriptor)
			if declaring != "" && declaring != strings.ReplaceAll(f.ClassName, ".", "/") && len(types.ClassFormalTypeParamNames(signature)) > 0 {
				// A non-generic subclass can fix its parent's type arguments.
				// Its raw self-view still inherits those arguments; widen to
				// the actual generic declaring owner to erase them. Dynamic
				// dispatch, the invocation witness and every operand stay intact.
				if !callbinding.Assignable(bindingType(f.Object.Type()), "L"+declaring+";", ctx.InvocationMetadata) {
					return nil, false
				}
				sourceOwner = strings.ReplaceAll(declaring, "/", ".")
			}
		}
		if !f.IsStatic {
			out.Object = &CastExpression{Value: out.Object, TargetType: types.NewJavaClass(sourceOwner), Binding: true, OriginPC: f.OriginPC}
		}
		for i, p := range ps {
			if callbinding.Reference(p) {
				target, _ := types.ParseDescriptor(p)
				out.Arguments[i] = &CastExpression{Value: out.Arguments[i], TargetType: target, Binding: true, OriginPC: f.OriginPC}
			}
		}
		out.bindingPlanned = true
		changed = true
	}
	if !changed {
		return nil, false
	}
	return out, true
}

// PlanErasedFormalResult uses the consumer's first bound, with lexical method
// shadowing, as its JVM erasure. Only an already widening result may use this
// route: it never introduces a narrower payload check or guesses a callee's
// unavailable generic Signature.
func (f *FunctionCallExpression) PlanErasedFormalResult(ctx *class_context.ClassContext, target types.JavaType) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || target == nil || target.IsArray() {
		return nil, false
	}
	name, ok := types.RawClassFQN(target)
	if !ok || !ctx.IsTypeParam(name) {
		return nil, false
	}
	bound := ""
	for _, sig := range []string{ctx.CurrentMethodSig, ctx.ClassSig} {
		for _, formal := range types.ClassFormalTypeParamNames(sig) {
			if formal == name {
				// The bound's type arguments constrain calls on the recovered
				// formal (N extends Algebra<N>). A descriptor-only result view
				// cannot erase that relationship and retain the same source receiver.
				if types.FormalTypeParamBounds(sig)[name] != nil {
					return nil, false
				}
				bound = erasedInvocationBounds(sig)[name]
				break
			}
		}
		if bound != "" {
			break
		}
		// A malformed/dependent method bound still shadows a class formal.
		for _, formal := range types.ClassFormalTypeParamNames(sig) {
			if formal == name {
				return nil, false
			}
		}
	}
	_, ret, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || bound == "" || !callbinding.Assignable(ret, bound, ctx.InvocationMetadata) {
		return nil, false
	}
	return f.PlanErasedResultChain(ctx, ret)
}

// A zero-input generic factory has no argument constraints from which Java
// can recover nested result arguments. A descriptor-exact result cast supplies
// an erased assignment view without adding a check, changing dispatch or
// retargeting poly inputs. Every receiver edge must be a known widening.
func ErasedFactoryAssignmentView(value JavaValue, target types.JavaType, ctx *class_context.ClassContext) JavaValue {
	return erasedFactoryAssignmentView(value, target, ctx, 32)
}
func erasedFactoryAssignmentView(value JavaValue, target types.JavaType, ctx *class_context.ClassContext, budget int) JavaValue {
	if budget <= 0 || ctx == nil || ctx.InvocationMetadata == nil || target == nil || target.IsArray() {
		return value
	}
	if _, ok := types.AsParameterizedType(target); !ok {
		return value
	}
	if cast, ok := UnpackSoltValue(value).(*CastExpression); ok {
		if cast.TargetType == nil || bindingType(cast.TargetType) != bindingType(target) {
			return value
		}
		inner := erasedFactoryAssignmentView(cast.Value, target, ctx, budget-1)
		if inner == cast.Value {
			return value
		}
		copy := *cast
		copy.Value = inner
		return &copy
	}
	root, ok := UnpackSoltValue(value).(*FunctionCallExpression)
	if !ok {
		return value
	}
	_, result, err := callbinding.Descriptor(root.Descriptor)
	if err != nil || result != bindingType(target) {
		return value
	}
	call := root
	for depth := 0; depth < budget; depth++ {
		if call == nil || !call.HasOriginPC || call.Kind >= InvokeDynamic || call.IsSpecialInvoke || len(call.Arguments) != 0 {
			return value
		}
		ps, ret, err := callbinding.Descriptor(call.Descriptor)
		if err != nil || len(ps) != 0 || !callbinding.Reference(ret) || strings.HasPrefix(ret, "[") {
			return value
		}
		if call.IsStatic {
			_, _, sig := erasedInvocationDeclaration(ctx, strings.ReplaceAll(call.ClassName, ".", "/"), call.FunctionName, call.Descriptor)
			if len(types.MethodFormalTypeParamNames(sig)) == 0 {
				return value
			}
			_, _, declared := types.ParseMethodSignatureFull(sig, ctx)
			if _, ok := types.AsParameterizedType(declared); !ok || bindingType(declared) != ret {
				return value
			}
			raw, _ := types.ParseDescriptor(result)
			return &CastExpression{Value: value, TargetType: raw, Binding: true, OriginPC: root.OriginPC}
		}
		child, ok := UnpackSoltValue(call.Object).(*FunctionCallExpression)
		if !ok {
			return value
		}
		_, childResult, err := callbinding.Descriptor(child.Descriptor)
		owner := "L" + strings.ReplaceAll(call.ClassName, ".", "/") + ";"
		if err != nil || !callbinding.Assignable(childResult, owner, ctx.InvocationMetadata) {
			return value
		}
		call = child
	}
	return value
}

// ErasedZeroInputFactoryResult proves a closed invocation chain whose only
// source inference starts at a generic static factory. There are no arguments
// whose target typing could change an overload, lambda entry check or evaluation
// order. A caller can suppress final return-target inference with an unchecked
// view at this same descriptor erasure; every original invocation stays intact.
func ErasedZeroInputFactoryResult(ctx *class_context.ClassContext, value JavaValue, result string) bool {
	if ctx == nil || ctx.InvocationMetadata == nil || !strings.HasPrefix(result, "L") {
		return false
	}
	call, ok := UnpackSoltValue(value).(*FunctionCallExpression)
	if !ok {
		return false
	}
	_, rootResult, err := callbinding.Descriptor(call.Descriptor)
	if err != nil || rootResult != result {
		return false
	}
	for depth := 0; depth < 32; depth++ {
		if call == nil || !call.HasOriginPC || call.IsSpecialInvoke || strings.HasPrefix(call.FunctionName, "<") || (call.Kind == InvokeSpecial || call.Kind >= InvokeDynamic) || len(call.Arguments) != 0 || call.IsStatic != (call.Kind == InvokeStatic) {
			return false
		}
		ps, ret, err := callbinding.Descriptor(call.Descriptor)
		if err != nil || len(ps) != 0 || !strings.HasPrefix(ret, "L") {
			return false
		}
		owner := strings.ReplaceAll(call.ClassName, ".", "/")
		ownerMeta, known := ctx.InvocationMetadata(owner)
		if !known || ownerMeta.Name != owner || (call.Kind == InvokeInterface && !ownerMeta.IsInterface) {
			return false
		}
		kind := callbinding.Virtual
		if call.IsStatic {
			kind = callbinding.Static
		} else if call.Kind == InvokeInterface {
			kind = callbinding.Interface
		}
		family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: call.FunctionName, Desc: call.Descriptor, Kind: kind}, ctx.InvocationMetadata)
		if err != nil || !family.Complete || family.Proof != callbinding.Unique || family.Target == nil || family.Target.Static != call.IsStatic || family.Target.Bridge || family.Target.Varargs {
			return false
		}
		// A covariant bridge or duplicate return-only declaration is not a unique
		// source binding, even though its erased argument tuple is the same.
		if len(family.Methods) != 1 {
			return false
		}
		declaring, classSig, sig := erasedInvocationDeclaration(ctx, owner, call.FunctionName, call.Descriptor)
		if declaring == "" || (family.Target.Generic && sig == "") {
			return false
		}
		if sig != "" {
			scope := map[string]bool{}
			if !call.IsStatic {
				for _, name := range types.ClassFormalTypeParamNames(classSig) {
					scope[name] = true
				}
			}
			for _, name := range types.MethodFormalTypeParamNames(sig) {
				scope[name] = true
			}
			if !closedZeroInputReferenceSignatureInScope(sig, scope) {
				return false
			}
			_, params, declared := types.ParseMethodSignatureFull(sig, ctx)
			if len(params) != 0 || declared == nil || bindingType(declared) != ret {
				return false
			}
		}
		if call.IsStatic {
			names := types.MethodFormalTypeParamNames(sig)
			if len(names) == 0 || len(erasedInvocationBounds(sig)) != len(names) {
				return false
			}
			_, _, declared := types.ParseMethodSignatureFull(sig, ctx)
			if _, generic := types.AsParameterizedType(declared); !generic {
				return false
			}
			return true
		}
		child, ok := UnpackSoltValue(call.Object).(*FunctionCallExpression)
		if !ok {
			return false
		}
		_, produced, err := callbinding.Descriptor(child.Descriptor)
		if err != nil || !callbinding.Assignable(produced, "L"+owner+";", ctx.InvocationMetadata) {
			return false
		}
		call = child
	}
	return false
}

// The shared Signature parser accepts prefixes. This proof permits one entire
// zero-input reference result, with a balanced formal prefix and no throws
// scope. In particular a generic throws variable cannot be re-inferred here.
func closedZeroInputReferenceSignature(sig string) bool {
	return closedZeroInputReferenceSignatureInScope(sig, nil)
}
func closedZeroInputReferenceSignatureInScope(sig string, scope map[string]bool) bool {
	if len(sig) > 4096 {
		return false
	}
	at := 0
	// A formal has one class bound (possibly empty) and zero or more interface
	// bounds. Dependent type-variable bounds are valid on instance methods.
	if strings.HasPrefix(sig, "<") {
		at++
		count := 0
		for at < len(sig) && sig[at] != '>' {
			begin := at
			for at < len(sig) && sig[at] != ':' {
				if strings.ContainsRune("<>();.[/", rune(sig[at])) {
					return false
				}
				at++
			}
			if at == begin || at >= len(sig) {
				return false
			}
			at++
			bound := false
			if at < len(sig) && sig[at] != ':' {
				next, ok := takeZeroFactoryFieldSignature(sig, at, 0, scope)
				if !ok {
					return false
				}
				at = next
				bound = true
			}
			for at < len(sig) && sig[at] == ':' {
				next, ok := takeZeroFactoryFieldSignature(sig, at+1, 0, scope)
				if !ok {
					return false
				}
				at = next
				bound = true
			}
			if !bound {
				return false
			}
			count++
		}
		if count == 0 || at >= len(sig) || sig[at] != '>' {
			return false
		}
		at++
	}
	if !strings.HasPrefix(sig[at:], "()L") {
		return false
	}
	end, ok := takeZeroFactoryFieldSignature(sig, at+2, 0, scope)
	return ok && end == len(sig)
}

func takeZeroFactoryFieldSignature(sig string, at, depth int, scope map[string]bool) (int, bool) {
	if depth > 32 || at >= len(sig) {
		return at, false
	}
	switch sig[at] {
	case 'T':
		begin := at + 1
		at = begin
		for at < len(sig) && sig[at] != ';' {
			if strings.ContainsRune("<>:().[/", rune(sig[at])) {
				return at, false
			}
			at++
		}
		return at + 1, at > begin && at < len(sig) && (scope == nil || scope[sig[begin:at]])
	case '[':
		at++
		if at < len(sig) && strings.ContainsRune("BCDFIJSZ", rune(sig[at])) {
			return at + 1, true
		}
		return takeZeroFactoryFieldSignature(sig, at, depth+1, scope)
	case 'L':
		at++
		for {
			begin := at
			for at < len(sig) && sig[at] != '<' && sig[at] != '.' && sig[at] != ';' {
				if strings.ContainsRune(":>()[]^", rune(sig[at])) || sig[at] <= ' ' {
					return at, false
				}
				at++
			}
			if at == begin || at >= len(sig) {
				return at, false
			}
			if sig[at] == '<' {
				at++
				count := 0
				for at < len(sig) && sig[at] != '>' {
					if sig[at] == '*' {
						at++
						count++
						continue
					}
					if sig[at] == '+' || sig[at] == '-' {
						at++
					}
					next, ok := takeZeroFactoryFieldSignature(sig, at, depth+1, scope)
					if !ok {
						return at, false
					}
					at = next
					count++
				}
				if count == 0 || at >= len(sig) {
					return at, false
				}
				at++
			}
			if at < len(sig) && sig[at] == ';' {
				return at + 1, true
			}
			if at >= len(sig) || sig[at] != '.' {
				return at, false
			}
			at++
		}
	}
	return at, false
}
