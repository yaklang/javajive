package values

import (
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A raw allocation's constructor parameters are erased. If another overload
// competes, preserve the actual invokespecial descriptor before any generic
// formal suppresses that cast. In particular, new Box((Object) "x") and
// new Box((Object) null) must not switch from Box(T) to Box(String).
// The enclosing method's descriptor and its class type variables say nothing
// about this new object's instantiation.
func (f *FunctionCallExpression) rawConstructorBindingCast(i int, arg JavaValue, ctx *class_context.ClassContext) string {
	if f == nil || f.FunctionName != "<init>" || arg == nil || f.Object == nil ||
		isWitnessLambdaArg(arg) || f.overloadFamilyProof(ctx) != overloadCompete {
		return ""
	}
	object := UnpackSoltValue(f.Object)
	if ref, ok := object.(*JavaRef); ok {
		if ref == nil || ref.IsThis || ref.Val == nil {
			return ""
		}
		object = UnpackSoltValue(ref.Val)
	}
	allocation, ok := object.(*NewExpression)
	if !ok || allocation == nil || allocation.Type() == nil {
		return ""
	}
	if !sameErasureClassName(witnessRawClassName(allocation.Type()), f.ClassName) {
		return ""
	}
	if _, parameterized := types.AsParameterizedType(allocation.Type()); parameterized || allocation.genericCtorDiamond(ctx) != "" {
		return ""
	}
	param := f.witnessDescriptorParamType(i)
	if param == nil || !isWitnessReferenceType(param) {
		return ""
	}
	if IsNullLiteral(UnpackSoltValue(arg)) {
		return renderWitnessParamType(param, ctx)
	}
	actual := arg.Type()
	if actual == nil || !isWitnessReferenceType(actual) || witnessSameRawClass(actual, param) {
		return ""
	}
	// An exact array already selects the descriptor's formal. Adding a cast
	// supplies no binding evidence and hides array initializer/temporary
	// structure from constructor and enum reconstruction. Covariant arrays
	// remain distinct: String[] may still need an Object[] overload pin.
	if actual.IsArray() && param.IsArray() && reflect.DeepEqual(actual.RawType(), param.RawType()) {
		return ""
	}
	return renderWitnessParamType(param, ctx)
}
