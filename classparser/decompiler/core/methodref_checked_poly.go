package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The source target of a method reference controls its erased entry checks.
// Infer that target from original generic declarations and both SAM descriptors,
// then separately prove which ordinary member source overload resolution picks.
// A consumer's inferred type arguments are not evidence for either decision.
func methodRefCheckedPolyTarget(d *Decompiler, req CallSiteRequest, static []values.JavaValue, impl *values.JavaClassMember, raw types.JavaType) values.JavaValue {
	if d == nil || d.FunctionContext == nil || impl == nil || impl.RefKind != RefInvokeStatic || len(req.DynamicArgs) != 0 || len(static) != 3 || d.blockPartialFunctionalTarget || class_context.SafeIdentifier(impl.Member) != impl.Member {
		return nil
	}
	ctx := d.FunctionContext
	declarations := ctx.LambdaTargetClassSig
	if declarations == nil {
		declarations = ctx.SiblingClassSig
	}
	name, known := types.RawClassFQN(raw)
	if !known || declarations == nil || ctx.InvocationMetadata == nil {
		return nil
	}
	owner := strings.ReplaceAll(name, ".", "/")
	erased, actual := t19MethodTypeDesc(static[0]), t19MethodTypeDesc(static[2])
	signature, _, known := declarations(owner)
	if !known {
		return nil
	}
	selected := inheritedDeclaredSamSignature(ctx, declarations, owner, req.CallSiteName, erased)
	if selected == "" {
		return nil
	}
	target := inferDeclaredSamInstantiation(name, signature, selected, erased, actual, ctx.InvocationMetadata)
	if target == nil || !methodRefPolySelection(ctx, impl, actual) {
		return nil
	}
	implementationOwner := strings.ReplaceAll(impl.Name, "/", ".")
	erasedView := types.NewJavaClass(name)
	value := values.NewStreamingCustomValue(func(ctx *class_context.ClassContext, out *workbudget.Writer) error {
		bound, known := ctx.StaticMethodReferenceOwner(implementationOwner, impl.Member, impl.Description)
		if !known {
			return ctx.StaticMethodImports.Error()
		}
		return writeMethodReference(out, bound, impl.Member)
	}, func() types.JavaType { return erasedView })
	value.Flag, value.NoOuterCapture, value.CapturesKnown, value.IsMethodRef = "lambda", true, true, true
	value.InstantiatedMtdDesc = actual
	// Freeze only the creation target. Later invariant consumers may see its
	// original erasure, but cannot widen the generated SAM's input check.
	return retainErasedFunctionalValue(value, erasedView, target)
}

// Fixed-arity, non-generic static references use the instantiated SAM inputs
// for applicability. The original member must be strictly more specific than
// every other applicable declaration. Unknown ancestors and other adaptation
// families supply no successful proof. The entire same-name family is read,
// including different arities: a hidden varargs/instance method may compete.
func methodRefPolySelection(ctx *class_context.ClassContext, impl *values.JavaClassMember, instantiated string) bool {
	if ctx == nil || ctx.InvocationMetadata == nil || impl == nil || impl.RefKind != RefInvokeStatic {
		return false
	}
	actual, result, err := callbinding.Descriptor(instantiated)
	inputs, implementationResult, implementationErr := callbinding.Descriptor(impl.Description)
	if err != nil || implementationErr != nil || len(actual) > 255 || len(actual) != len(inputs) || result != implementationResult {
		return false
	}
	charge := func(n int) bool {
		return n >= 0 && (ctx.Work == nil || ctx.Work.Charge(workbudget.CounterGraphScans, int64(n)) == nil)
	}
	graph := map[string]callbinding.Class{}
	active := map[string]bool{}
	var collect func(string, int) bool
	collect = func(name string, depth int) bool {
		if name == "" || depth > 32 || active[name] || !charge(1) {
			return false
		}
		if _, known := graph[name]; known {
			return true
		}
		if len(graph) >= 128 {
			return false
		}
		meta, known := ctx.InvocationMetadata(name)
		if !known || meta.Name != name || !meta.MembersComplete || !meta.ParentsComplete || len(meta.Methods) > 2048 || len(meta.Parents) > 64 || !charge(len(meta.Methods)+len(meta.Parents)) {
			return false
		}
		if ctx.Work != nil && ctx.Work.CheckAlloc(int64(len(meta.Methods)*128+len(meta.Parents)*64+256)) != nil {
			return false
		}
		meta.Methods = append([]callbinding.Method(nil), meta.Methods...)
		meta.Parents = append([]string(nil), meta.Parents...)
		graph[name], active[name] = meta, true
		defer delete(active, name)
		seen := map[string]bool{}
		for _, parent := range meta.Parents {
			if seen[parent] || !collect(parent, depth+1) {
				return false
			}
			seen[parent] = true
		}
		return true
	}
	owner := strings.ReplaceAll(impl.Name, ".", "/")
	if !collect(owner, 0) || (!graph[owner].Public && owner != strings.ReplaceAll(ctx.ClassName, ".", "/")) {
		return false
	}
	// Snapshot the complete owner family before adding input hierarchy nodes.
	var methods []callbinding.Method
	for name, meta := range graph {
		for _, method := range meta.Methods {
			if method.Name != impl.Member {
				continue
			}
			if !method.Static || method.Generic || method.Varargs || method.Bridge || (!method.Public && (name != owner || owner != strings.ReplaceAll(ctx.ClassName, ".", "/"))) {
				return false
			}
			methods = append(methods, method)
		}
	}
	if len(methods) == 0 || len(methods) > 256 || !charge(len(methods)) {
		return false
	}
	for _, input := range actual {
		component := strings.TrimLeft(input, "[")
		if strings.HasPrefix(component, "L") && !collect(callbinding.Name(component), 0) {
			return false
		}
	}
	provider := callbinding.Provider(func(name string) (callbinding.Class, bool) {
		if !charge(1) {
			return callbinding.Class{}, false
		}
		meta, known := graph[name]
		return meta, known
	})
	applicable := func(parameters []string) bool {
		if len(parameters) != len(actual) {
			return false
		}
		for i, input := range actual {
			if !charge(1) || !callbinding.Assignable(input, parameters[i], provider) {
				return false
			}
		}
		return true
	}
	if !applicable(inputs) {
		return false
	}
	matches := 0
	for _, method := range methods {
		parameters, _, err := callbinding.Descriptor(method.Desc)
		if err != nil {
			return false
		}
		if method.Desc == impl.Description {
			matches++
			continue
		}
		if !applicable(parameters) {
			continue
		}
		strict := false
		for i, input := range inputs {
			if !charge(1) || !callbinding.Assignable(input, parameters[i], provider) {
				return false
			}
			strict = strict || input != parameters[i]
		}
		if !strict {
			return false
		}
	}
	return matches == 1 && (ctx.Work == nil || ctx.Work.Err() == nil)
}
