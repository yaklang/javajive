package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// A unary reference argument binds to the Object overload by strict invocation
// before Java considers unboxing for primitive overloads. Widening it to Object
// cannot improve that selection and can destroy a generic source formal whose
// descriptor erased to Object (e.g. Element<T>.multiply(T) vs multiply(double)).
// Require a complete family; another reference overload or missing ancestor
// invalidates this proof. No caller/callee type-variable names are equated.
func (f *FunctionCallExpression) objectFormalWinsBeforeUnboxing(ctx *class_context.ClassContext) bool {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || len(f.Arguments) != 1 ||
		f.FunctionName == "<init>" || f.Arguments[0] == nil || !isWitnessReferenceType(f.Arguments[0].Type()) {
		return false
	}
	params, _, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(params) != 1 || params[0] != "Ljava/lang/Object;" {
		return false
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{
		Owner: strings.ReplaceAll(f.ClassName, ".", "/"), Name: f.FunctionName, Desc: f.Descriptor,
	}, ctx.InvocationMetadata)
	if err != nil || !family.Complete || family.Target == nil {
		return false
	}
	for _, method := range family.Methods {
		other, _, err := callbinding.Descriptor(method.Desc)
		if err != nil || len(other) != 1 || (other[0] != params[0] && callbinding.Reference(other[0])) {
			return false
		}
	}
	return true
}
