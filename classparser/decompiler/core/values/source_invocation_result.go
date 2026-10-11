package values

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// SourceInvocationResultType observes the selected declaration and receiver
// substitution without changing the JVM computational result or evaluating a
// receiver. It is a source view, not permission to narrow an arbitrary local.
func (f *FunctionCallExpression) SourceInvocationResultType(ctx *class_context.ClassContext) types.JavaType {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || !f.HasOriginPC || f.FuncType == nil || f.IsStatic && f.Kind != InvokeStatic || !f.IsStatic && f.Kind != InvokeVirtual && f.Kind != InvokeInterface {
		return nil
	}
	ps, result, err := callbinding.Descriptor(f.Descriptor)
	if ctx.Work != nil && ctx.Work.CheckAlloc(64<<10) != nil {
		return nil
	}
	if err != nil || len(ps) != len(f.Arguments) || !callbinding.Reference(result) {
		return nil
	}
	bounded := *ctx
	queries := 0
	contradictory := false
	bounded.InvocationMetadata = func(n string) (callbinding.Class, bool) {
		queries++
		if queries > 256 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return callbinding.Class{}, false
		}
		v, ok := ctx.InvocationMetadata(n)
		if !ok || v.Name != n || len(v.Signature) > 4096 || len(v.Methods) > 4096 {
			return callbinding.Class{}, false
		}
		return v, true
	}
	if ctx.SiblingClassSig != nil {
		bounded.SiblingClassSig = func(n string) (string, map[string]string, bool) {
			queries++
			if queries > 256 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
				return "", nil, false
			}
			cs, ms, ok := ctx.SiblingClassSig(n)
			if !ok || len(cs) > 4096 || len(ms) > 4096 {
				return "", nil, false
			}
			meta, complete := bounded.InvocationMetadata(n)
			if !complete || !meta.MembersComplete || !meta.ParentsComplete || !sourceInvocationClassSignatureAgrees(cs, meta) {
				contradictory = true
				return "", nil, false
			}
			for _, method := range meta.Methods {
				if method.Name == f.FunctionName && method.Desc == f.Descriptor {
					if declared, ok := ms[class_context.MethodDescKey(method.Name, method.Desc)]; ok && declared != method.Signature {
						contradictory = true
						return "", nil, false
					}
				}
			}
			return cs, ms, true
		}
	}
	owner, cs, sig := erasedInvocationDeclaration(&bounded, strings.ReplaceAll(f.ClassName, ".", "/"), f.FunctionName, f.Descriptor)
	if contradictory || owner == "" || sig == "" || len(cs)+len(sig) > 8192 {
		return nil
	}
	declaration, available := bounded.InvocationMetadata(owner)
	if !available {
		return nil
	}
	selected := 0
	for _, method := range declaration.Methods {
		if method.Name == f.FunctionName && method.Desc == f.Descriptor {
			if method.Static != f.IsStatic || method.Bridge {
				return nil
			}
			selected++
		}
	}
	if selected != 1 {
		return nil
	}
	physicalResult, validResult := SourceTypeErasure(f.FuncType.ReturnType, &bounded)
	if !validResult || physicalResult != result {
		return nil
	}
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{cs}, sig)
	if !valid || erased != f.Descriptor {
		return nil
	}
	if strings.ReplaceAll(ctx.ClassName, ".", "/") == owner {
		bounded.MethodSignaturesByDesc = map[string]string{class_context.MethodDescKey(f.FunctionName, f.Descriptor): sig}
	}
	typesSeen := map[types.JavaType]bool{}
	typeNodes := 0
	var shape func(types.JavaType, int) bool
	shape = func(t types.JavaType, d int) bool {
		typeNodes++
		if t == nil || reflect.ValueOf(t).Kind() == reflect.Pointer && reflect.ValueOf(t).IsNil() || typesSeen[t] || d > 32 || typeNodes > 1024 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
			return false
		}
		typesSeen[t] = true
		defer delete(typesSeen, t)
		if t.IsArray() {
			return shape(t.ElementType(), d+1)
		}
		if p, ok := types.AsParameterizedType(t); ok {
			if len(p.TypeArgs) > 255 || len(p.OwnerSegments) > 1 {
				return false
			}
			for _, arg := range p.TypeArgs {
				if !shape(arg, d+1) {
					return false
				}
			}
		}
		if w, ok := t.(*types.JavaWildcardType); ok && w.Bound != nil {
			return shape(w.Bound, d+1)
		}
		return true
	}
	// Bound the graph before the existing structural query traverses it. Opaque
	// aliases, cycles and unusually deep receiver chains are not type evidence.
	active := map[JavaValue]bool{}
	nodes := 0
	var graph func(JavaValue, int) bool
	graph = func(v JavaValue, depth int) bool {
		nodes++
		if isNilJavaValue(v) || active[v] || depth > 32 || nodes > 512 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
			return false
		}
		active[v] = true
		defer delete(active, v)
		if r, ok := v.(*JavaRef); ok && (r.CustomValue != nil || r.StackVar != nil) {
			return false
		}
		if ref, ok := v.(*JavaRef); ok && !shape(ref.Type(), 0) {
			return false
		}
		if call, ok := v.(*FunctionCallExpression); ok {
			if call.FuncType == nil || !shape(call.FuncType.ReturnType, 0) {
				return false
			}
			if !call.IsStatic && !graph(call.Object, depth+1) {
				return false
			}
			for _, a := range call.Arguments {
				if !graph(a, depth+1) {
					return false
				}
			}
			return true
		}
		children, ok := Children(v)
		if !ok {
			return false
		}
		for _, child := range children {
			if !graph(child, depth+1) {
				return false
			}
		}
		return true
	}
	if !graph(f, 0) {
		return nil
	}
	parameters, ret, formals := f.genericMethodSignature(&bounded)
	if len(formals) != 0 {
		// Argument-witness inference observes source types rather than the
		// intentionally erased computational types of nested invocations. The
		// temporary refs belong only to this proof; original operands stay put.
		proof := f.Clone()
		for i, arg := range f.Arguments {
			var view types.JavaType
			if inner, ok := UnpackSoltValue(arg).(*FunctionCallExpression); ok {
				view = inner.SourceInvocationResultType(&bounded)
			} else {
				view = SourceFieldType(&bounded, arg)
			}
			if view != nil {
				if erased, known := SourceTypeErasure(view, &bounded); !known || erased != bindingType(arg.Type()) {
					return nil
				}
				proof.Arguments[i] = NewJavaRef(utils.NewRootVariableId(), nil, view)
			}
		}
		bindings := map[string]types.JavaType{}
		variables := methodVariableSet(formals)
		if len(parameters) != len(proof.Arguments) {
			return nil
		}
		for i, param := range parameters {
			if javaTypeMentionsNames(param, formals) && !bindMethodTypeArguments(param, proof.Arguments[i].Type(), variables, bindings) {
				return nil
			}
		}
		constraints, _, valid := types.FormalBoundConstraints(sig)
		if !valid || len(constraints) != len(formals) {
			return nil
		}
		for _, formal := range formals {
			argument := bindings[formal]
			if argument == nil {
				return nil
			}
			actual, known := SourceTypeErasure(argument, &bounded)
			if !known || !callbinding.Reference(actual) {
				return nil
			}
			for _, bound := range constraints[formal] {
				// Parameterized bounds and class-owned bound variables need a
				// separate full bound-substitution proof; erasure cannot prove them.
				if _, parameterized := types.AsParameterizedType(bound); parameterized {
					return nil
				}
				raw, ok := types.RawClassFQN(bound)
				if !ok || !strings.Contains(raw, ".") {
					return nil
				}
				expected, known := SourceTypeErasure(bound, &bounded)
				if !known || !callbinding.Assignable(actual, expected, bounded.InvocationMetadata) {
					return nil
				}
			}
		}
		ret = proof.inferredGenericMethodReturn(&bounded)
	}
	if contradictory || ret == nil || !shape(ret, 0) || !sourceDenotableJavaType(ret, &bounded) {
		return nil
	}
	physical, known := SourceTypeErasure(ret, &bounded)
	if !known || physical != result {
		return nil
	}
	return ret.Copy()
}

// The sibling signature provider materializes a raw superclass header when a
// non-generic original class has no Signature attribute. Accept that derived
// header only if every raw edge agrees with the physical class metadata. It
// cannot introduce type variables, generic arguments or new superclass edges.
func sourceInvocationClassSignatureAgrees(signature string, meta callbinding.Class) bool {
	if signature == meta.Signature {
		return true
	}
	if meta.Signature != "" || signature == "" {
		return false
	}
	own, refs, valid := types.SignatureTypeVariableReferences(signature)
	if !valid || len(own) != 0 || len(refs) != 0 {
		return false
	}
	super, interfaces := types.ParseClassSignatureSupers(signature)
	edges := append([]types.JavaType{super}, interfaces...)
	if meta.IsInterface {
		raw, ok := types.RawClassFQN(super)
		if !ok || strings.ReplaceAll(raw, ".", "/") != "java/lang/Object" {
			return false
		}
		edges = interfaces
	}
	if len(edges) != len(meta.Parents) {
		return false
	}
	physical := map[string]bool{}
	for _, n := range meta.Parents {
		if physical[n] {
			return false
		}
		physical[n] = true
	}
	for _, edge := range edges {
		if _, generic := types.AsParameterizedType(edge); generic {
			return false
		}
		n, ok := types.RawClassFQN(edge)
		if !ok || !physical[strings.ReplaceAll(n, ".", "/")] {
			return false
		}
		delete(physical, strings.ReplaceAll(n, ".", "/"))
	}
	return len(physical) == 0
}
