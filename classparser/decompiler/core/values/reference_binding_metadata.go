package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A nonthrowing reference conversion needs original inheritance, not the
// operand class's entire method table. Keep that capability separate: the
// planner still requires complete original member families and accessibility.
// An ancestry-only class has no public/method/generic/owner-role permissions.
//
// This fallback changes only fixed views of descriptor-seeded parameters or
// retained original CHECKCASTs. Inferred locals and poly producers cannot gain
// a new target context. The original invocation, operands and checks stay put;
// the supplemental provider never becomes the caller's declaration provider.
func (f *FunctionCallExpression) originalReferenceBindingMetadata(ctx *class_context.ClassContext, receiver string, arguments []callbinding.Argument, parameters []string) (callbinding.Provider, bool) {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingSuperTypes == nil ||
		!f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 || f.IsSpecialInvoke || f.FuncType == nil || f.FuncType.ReturnType == nil ||
		len(parameters) > 255 || len(parameters) != len(arguments) || len(arguments) != len(f.Arguments) || len(f.FuncType.ParamTypes) != len(parameters) ||
		f.IsStatic && f.Kind != InvokeStatic || !f.IsStatic && f.Kind != InvokeVirtual && f.Kind != InvokeInterface {
		return nil, false
	}
	physical, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(physical) != len(parameters) || bindingType(f.FuncType.ReturnType) != result {
		return nil, false
	}
	for i, parameter := range parameters {
		if physical[i] != parameter || bindingType(f.FuncType.ParamTypes[i]) != parameter {
			return nil, false
		}
	}
	if ctx.Work != nil && ctx.Work.CheckAlloc(int64(len(arguments))*48+128) != nil {
		return nil, false
	}
	metadata := boundedReferenceBindingMetadata(ctx)
	needed := false
	prove := func(value JavaValue, actual, formal string) bool {
		if actual == formal {
			return true
		}
		if !erasedFactoryFixedOperand(value) || declaredArrayOperandDescriptor(value, ctx) != actual || !referenceBindingViewAccessible(formal, ctx) {
			return false
		}
		if cast, original := UnpackSoltValue(value).(*CastExpression); original && cast.OriginPC >= f.OriginPC {
			return false
		}
		if callbinding.Assignable(actual, formal, ctx.InvocationMetadata) {
			return true
		}
		if !closedReferenceBindingWidening(actual, formal, metadata) {
			return false
		}
		needed = true
		return true
	}
	for i, argument := range arguments {
		if argument.Poly || !prove(f.Arguments[i], argument.Type, parameters[i]) {
			return nil, false
		}
	}
	if !f.IsStatic && !prove(f.Object, receiver, "L"+strings.ReplaceAll(f.ClassName, ".", "/")+";") {
		return nil, false
	}
	if !needed || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterNodeCopies, int64(len(arguments))+2) != nil {
		return nil, false
	}
	return metadata, true
}

func boundedReferenceBindingMetadata(ctx *class_context.ClassContext) callbinding.Provider {
	parents := func(name string) ([]string, bool) {
		ancestry, known := ctx.SiblingSuperTypes(name)
		if !known || len(ancestry) > 1024 {
			return nil, false
		}
		for _, parent := range ancestry {
			if len(parent) == 0 || len(parent) > 8192 || strings.ContainsAny(parent, ".[]();") ||
				ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(parent))+1) != nil {
				return nil, false
			}
		}
		return ancestry, true
	}
	combined := callbinding.WithReferenceAncestry(ctx.InvocationMetadata, parents, ctx.Work)
	remaining := 1024
	return func(name string) (callbinding.Class, bool) {
		remaining--
		if remaining < 0 || len(name) == 0 || len(name) > 8192 {
			return callbinding.Class{}, false
		}
		declaration, known := combined(name)
		if !known || declaration.Name != name || len(declaration.Parents) > 1024 || len(declaration.Methods) > 4096 || declaration.IsInterface && declaration.Final {
			return callbinding.Class{}, false
		}
		text := len(declaration.Signature)
		for _, method := range declaration.Methods {
			text += len(method.Name) + len(method.Desc) + len(method.Signature)
			if len(method.Name) > 8192 || len(method.Desc) > 8192 || len(method.Signature) > 8192 || text > 1<<20 {
				return callbinding.Class{}, false
			}
		}
		if text > 1<<20 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(text+len(declaration.Methods))+1) != nil {
			return callbinding.Class{}, false
		}
		return declaration, true
	}
}

func referenceBindingViewAccessible(descriptor string, ctx *class_context.ClassContext) bool {
	for strings.HasPrefix(descriptor, "[") {
		descriptor = descriptor[1:]
	}
	if !strings.HasPrefix(descriptor, "L") || !strings.HasSuffix(descriptor, ";") {
		return false
	}
	name := descriptor[1 : len(descriptor)-1]
	if !strings.Contains(name, "/") && ctx.IsTypeParam(name) {
		return false
	}
	if ctx.SiblingClassAccessible != nil {
		if accessible, known := ctx.SiblingClassAccessible(name); known && !accessible {
			return false
		}
	}
	// The ordinary planner checks the complete declaration's public flag.
	return true
}

func closedReferenceBindingWidening(actual, formal string, metadata callbinding.Provider) bool {
	// Primitive arrays do not acquire reference-component covariance. The
	// language-defined identity/Object/Cloneable/Serializable conversions were
	// already available to the original planner without supplemental metadata.
	for strings.HasPrefix(actual, "[") && strings.HasPrefix(formal, "[") {
		actual, formal = actual[1:], formal[1:]
	}
	if !strings.HasPrefix(actual, "L") || !strings.HasSuffix(actual, ";") || !strings.HasPrefix(formal, "L") || !strings.HasSuffix(formal, ";") {
		return false
	}
	// Positive paths alone must not hide an unresolved side branch or cycle.
	// This complete closure has fixed node/edge/depth caps, including nil Work.
	related, complete := checkCastAncestorClosure(callbinding.Name(actual), callbinding.Name(formal), metadata)
	return related && complete
}
