package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A factory's raw result view belongs to its selected consumer parameter, not
// to the enclosing constructor's arity. In particular, this(true, factory(...))
// needs the same inference boundary as this(flag, factory(...)). Preserve the
// factory's original input checks; an input array cannot prove result identity.
//
// This finite domain is an original THIS delegation in a nongeneric owner,
// a complete concrete selected Signature, one proved factory result, and fixed
// other arguments. Every same-arity rival must be excluded at this parameter.
// The selected fixed-arity call then precedes variable-arity applicability.
func (f *FunctionCallExpression) delegationFactoryResultCast(index int, argument JavaValue, ctx *class_context.ClassContext) string {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || ctx.FunctionName != "<init>" ||
		f.FunctionName != "<init>" || f.Kind != InvokeSpecial || !f.IsSpecialInvoke || f.IsStatic || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 ||
		!sameErasureClassName(f.ClassName, ctx.ClassName) || index < 0 || index >= len(f.Arguments) || f.Arguments[index] != argument {
		return ""
	}
	receiver, known := UnpackSoltValue(f.Object).(*JavaRef)
	if !known || receiver == nil || !receiver.IsThis || !sameErasureClassName(witnessRawClassName(receiver.Type()), ctx.ClassName) {
		return ""
	}
	if _, original := receiver.OriginalReceiverSlot(); !original {
		return ""
	}
	parameters, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || result != "V" || len(parameters) != len(f.Arguments) || len(parameters) > 128 || !strings.HasPrefix(parameters[index], "L") || f.FuncType == nil || f.FuncType.ReturnType == nil || bindingType(f.FuncType.ReturnType) != "V" || len(f.FuncType.ParamTypes) != len(parameters) {
		return ""
	}
	for i, parameter := range parameters {
		if bindingType(f.FuncType.ParamTypes[i]) != parameter {
			return ""
		}
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	meta, known := ctx.InvocationMetadata(owner)
	if !known || meta.Name != owner || meta.Signature != ctx.ClassSig || meta.IsInterface || !meta.MembersComplete || !meta.ParentsComplete || len(meta.Methods) > 4096 || len(types.ClassFormalTypeParamNames(meta.Signature)) != 0 {
		return ""
	}
	var selected callbinding.Method
	matches := 0
	for _, method := range meta.Methods {
		if ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return ""
		}
		if method.Name == "<init>" && method.Desc == f.Descriptor {
			selected, matches = method, matches+1
		}
	}
	if matches != 1 || selected.Static || selected.Bridge || selected.Varargs || selected.Signature == "" || len(selected.Signature) > 8192 ||
		ctx.Work != nil && (ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(selected.Signature))*130+1) != nil || ctx.Work.CheckAlloc(int64(len(selected.Signature))*256+128) != nil) {
		return ""
	}
	classSignature, signatures, known := ctx.SiblingClassSig(owner)
	if !known || classSignature != meta.Signature || signatures[class_context.MethodDescKey("<init>", f.Descriptor)] != selected.Signature {
		return ""
	}
	formals, references, valid := types.SignatureTypeVariableReferences(selected.Signature)
	erasure, _, complete := types.EraseLexicalMethodSignatureWithThrows(meta.Signature, selected.Signature)
	_, sourceParameters, produced := types.ParseMethodSignatureFull(selected.Signature, ctx)
	if !valid || len(formals) != 0 || len(references) != 0 || !complete || erasure != f.Descriptor || produced == nil || bindingType(produced) != "V" || len(sourceParameters) != len(parameters) {
		return ""
	}
	target, parameterized := types.AsParameterizedType(sourceParameters[index])
	if !parameterized || len(target.TypeArgs) == 0 || sourceParameters[index].IsArray() || bindingType(sourceParameters[index]) != parameters[index] ||
		!delegationConcreteResultType(sourceParameters[index], ctx, 0) || !ErasedFactoryReturn(ctx, argument, parameters[index]) {
		return ""
	}
	for i, value := range f.Arguments {
		if i == index {
			continue
		}
		if value == nil || value.Type() == nil || sourceParameters[i] == nil || bindingType(sourceParameters[i]) != parameters[i] {
			return ""
		}
		if _, generic := types.AsParameterizedType(sourceParameters[i]); generic {
			return ""
		}
		if parameters[i] == "Z" {
			if !delegationCanonicalBoolean(value, ctx) {
				return ""
			}
			continue // This does not use renderer coercion as a proof.
		}
		if bindingType(value.Type()) != parameters[i] {
			return ""
		}
	}
	for _, rival := range meta.Methods {
		if rival.Name != "<init>" || rival.Desc == f.Descriptor {
			continue
		}
		other, result, err := callbinding.Descriptor(rival.Desc)
		if err != nil || result != "V" || rival.Static || rival.Bridge {
			return ""
		}
		if len(other) != len(parameters) {
			continue
		}
		if strings.HasPrefix(other[index], "[") {
			continue // A typed non-array result is not a fixed-arity array argument.
		}
		if !strings.HasPrefix(other[index], "L") {
			return "" // Unboxing/numeric applicability requires a separate proof.
		}
		related, complete := checkCastAncestorClosure(callbinding.Name(parameters[index]), callbinding.Name(other[index]), ctx.InvocationMetadata)
		if related || !complete {
			return ""
		}
	}
	return sourceParameters[index].String(ctx)
}

func delegationConcreteResultType(typ types.JavaType, ctx *class_context.ClassContext, depth int) bool {
	if typ == nil || depth > 16 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
		return false
	}
	if _, wildcard := typ.(*types.JavaWildcardType); wildcard {
		return false
	}
	if typ.IsArray() {
		return delegationConcreteResultType(typ.ElementType(), ctx, depth+1)
	}
	if parameterized, ok := types.AsParameterizedType(typ); ok {
		owner := strings.ReplaceAll(parameterized.RawClassName, ".", "/")
		meta, known := ctx.InvocationMetadata(owner)
		if !known || meta.Name != owner || !erasedFactoryDeclarationAccessible(owner, meta.Public, ctx) || len(meta.Signature) > 8192 || len(parameterized.TypeArgs) > 64 || len(types.ClassFormalTypeParamNames(meta.Signature)) != len(parameterized.TypeArgs) {
			return false
		}
		for _, argument := range parameterized.TypeArgs {
			if !delegationConcreteResultType(argument, ctx, depth+1) {
				return false
			}
		}
		return true
	}
	if name, named := types.RawClassFQN(typ); named {
		name = strings.ReplaceAll(name, ".", "/")
		meta, known := ctx.InvocationMetadata(name)
		return known && meta.Name == name && erasedFactoryDeclarationAccessible(name, meta.Public, ctx)
	}
	return !callbinding.Reference(bindingType(typ)) && bindingType(typ) != "V" && bindingType(typ) != ""
}

func delegationCanonicalBoolean(value JavaValue, ctx *class_context.ClassContext) bool {
	value = UnpackSoltValue(value)
	if literal, known := value.(*JavaLiteral); known {
		switch v := literal.Data.(type) {
		case bool:
			return bindingType(literal.Type()) == "Z"
		case int:
			typ := bindingType(literal.Type())
			return (typ == "Z" || typ == "I") && (v == 0 || v == 1)
		}
		return false
	}
	return erasedFactoryFixedOperand(value) && bindingType(value.Type()) == "Z" && declaredArrayOperandDescriptor(value, ctx) == "Z"
}
