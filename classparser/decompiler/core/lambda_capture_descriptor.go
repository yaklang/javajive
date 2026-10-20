package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
)

// Synthetic implementation parameters are the original capture contract.
// A provisional enclosing local (notably a null-only local) must not turn a
// ClassLoader capture into an Object expression inside the emitted body. Keep
// the enclosing identity and snapshot intact and give its substitution the
// original descriptor view. A matching parameterized capture keeps its stronger
// source view; Object parameters do not need a cast.
func lambdaCaptureDescriptorViews(impl *values.JavaClassMember, captured []values.JavaValue) []values.JavaValue {
	if impl == nil {
		return captured
	}
	parsed, err := types.ParseMethodDescriptor(impl.Description)
	if err != nil {
		return captured
	}
	params := parsed.FunctionType().ParamTypes
	switch impl.RefKind {
	case RefInvokeStatic:
	case RefInvokeVirtual, RefInvokeSpecial, RefInvokeInterface:
		params = append([]types.JavaType{types.NewJavaClass(impl.Name)}, params...)
	default:
		return captured
	}
	if len(params) < len(captured) {
		return captured
	}
	out := append([]values.JavaValue(nil), captured...)
	for i, v := range captured {
		if v == nil || v.Type() == nil || params[i] == nil {
			continue
		}
		target := params[i]
		if primitive, ok := target.RawType().(*types.JavaPrimer); ok && primitive.Name != types.JavaString {
			continue
		}
		if name, ok := types.RawClassFQN(target); ok && name == "java.lang.Object" {
			continue
		}
		if a, ok := types.RawClassFQN(target); ok && !target.IsArray() && !v.Type().IsArray() {
			if b, ok := types.RawClassFQN(v.Type()); ok && a == b {
				continue
			}
		}
		if reflect.DeepEqual(target.RawType(), v.Type().RawType()) {
			continue
		}
		if primitive, ok := v.Type().RawType().(*types.JavaPrimer); ok && primitive.Name == types.JavaString {
			if name, ok := types.RawClassFQN(target); ok && name == "java.lang.String" {
				continue
			}
		}
		// Preserve stronger source declarations, including lexical type variables
		// and parameterized captures. Only a provisional Object view needs
		// descriptor recovery; erasing a stronger view would break inference.
		if name, ok := types.RawClassFQN(v.Type()); !ok || name != "java.lang.Object" {
			continue
		}
		if snapshot, owned := v.(*values.JavaRef); owned && snapshot != nil {
			if _, ordinal, pending := snapshot.OriginalPendingDynamicOperand(snapshot.Val); pending && ordinal == i {
				// This is the factory's own fresh operand copy, not the
				// enclosing source declaration. Retain its original capture
				// descriptor on that copy so metadata/binder consumers see
				// the same original identity, without a separate cast view.
				snapshot.ResetVarType(target.Copy())
				continue
			}
		}
		out[i] = &values.CastExpression{Value: v, TargetType: target, Binding: true}
	}
	return out
}
