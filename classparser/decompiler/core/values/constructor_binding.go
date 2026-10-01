package values

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A selected generic argument view may need an unchecked conversion between
// invariant parameterizations. The descriptor establishes the raw head of
// that already-selected cast. Bridging through the same head changes source
// inference without introducing a different runtime CHECKCAST or evaluation.
func (f *FunctionCallExpression) renderProvenArgumentCast(i int, target string, arg JavaValue, ctx *class_context.ClassContext) string {
	expr := arg.String(ctx)
	_, rawAllocation := UnpackSoltValue(arg).(*NewExpression)
	if strings.Contains(target, "<") && !strings.HasSuffix(target, "[]") && !rawAllocation && !isWitnessLambdaArg(UnpackSoltValue(arg)) {
		if param := f.witnessDescriptorParamType(i); param != nil && !param.IsArray() {
			raw := param.String(ctx)
			if raw == erasureNameOf(target) {
				return fmt.Sprintf("(%s)(%s)(%s)", target, raw, expr)
			}
		}
	}
	return fmt.Sprintf("(%s)(%s)", target, expr)
}

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
	if f.constructorMethodFormalCallerInference(i, arg, ctx) {
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

// A non-generic allocated class does not erase its constructor's method
// formals. An already typed caller variable must keep its input inference if
// every competing constructor is proved inapplicable at the source bound.
func (f *FunctionCallExpression) constructorMethodFormalCallerInference(i int, arg JavaValue, ctx *class_context.ClassContext) bool {
	if ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || arg == nil || arg.Type() == nil {
		return false
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	cs, methods, known := ctx.SiblingClassSig(owner)
	if !known || len(types.ClassFormalTypeParamNames(cs)) != 0 {
		return false
	}
	sig := methods[class_context.MethodDescKey("<init>", f.Descriptor)]
	if !strings.HasPrefix(sig, "<") {
		return false
	}
	bounds := erasedInvocationBounds(sig)
	_, params, _ := types.ParseMethodSignatureFull(sig, ctx)
	ps, _, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || i < 0 || i >= len(ps) || len(params) != len(ps) || len(f.Arguments) != len(ps) {
		return false
	}
	for _, value := range f.Arguments {
		if value == nil || value.Type() == nil {
			return false
		}
	}
	formal, bare := types.RawClassFQN(params[i])
	if !bare || bounds[formal] != ps[i] || !erasedInvocationCallerFormal(arg.Type(), ps[i], ctx) {
		return false
	}
	c, known := ctx.InvocationMetadata(owner)
	if !known || !c.MembersComplete {
		return false
	}
	for _, m := range c.Methods {
		if m.Name != "<init>" || m.Desc == f.Descriptor {
			continue
		}
		competing, _, err := callbinding.Descriptor(m.Desc)
		if err != nil || m.Varargs {
			return false
		}
		if len(competing) != len(ps) {
			continue
		}
		excluded := false
		for j, other := range competing {
			if f.Arguments[j] == nil || f.Arguments[j].Type() == nil {
				return false
			}
			actual := erasedInvocationArgumentType(f.Arguments[j], ps[j])
			if erasedInvocationCallerFormal(f.Arguments[j].Type(), ps[j], ctx) {
				actual = ps[j]
			}
			if !callbinding.Assignable(actual, other, ctx.InvocationMetadata) {
				excluded = true
				break
			}
		}
		if !excluded {
			return false
		}
	}
	return true
}
