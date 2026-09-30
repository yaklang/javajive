package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A raw List can be applicable to both List<? extends String> and
// Collection<? extends Number>; neither parameterized formal is more specific.
// Keep the bytecode-selected declaration by recovering its exact formal, only
// when its erasure is a proven supertype of the argument. This adds no runtime check.
// Callee method variables are a separate namespace, even when their spelling
// matches a caller variable. They need inference, never a copied name.
func (f *FunctionCallExpression) parameterizedOverloadArgCast(i int, ctx *class_context.ClassContext) string {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || f.Descriptor == "" ||
		f.FunctionName == "<init>" || i < 0 || i >= len(f.Arguments) ||
		(f.IsSpecialInvoke && !f.isCurrentClass(ctx)) {
		return ""
	}
	arg := f.Arguments[i]
	if arg == nil || arg.Type() == nil || isWitnessLambdaArg(UnpackSoltValue(arg)) {
		return ""
	}
	actual, raw := arg.Type().RawType().(*types.JavaClass)
	selected := witnessRawClassName(f.witnessDescriptorParamType(i))
	if !raw || actual == nil || selected == "" || !provenOverloadWidening(actual.Name, selected, ctx) {
		return ""
	}
	params, _, methodFormals := f.genericMethodSignature(ctx)
	if len(params) == 0 && ctx.SiblingClassSig != nil {
		// A raw generic receiver cannot instantiate its owner's variables, but
		// a concrete formal such as List<? extends Type> is independent of them.
		ownerSig, methods, known := ctx.SiblingClassSig(strings.ReplaceAll(f.ClassName, ".", "/"))
		sig := methods[class_context.MethodDescKey(f.FunctionName, f.Descriptor)]
		if known && sig != "" {
			_, declared, _ := types.ParseMethodSignatureFull(sig, ctx)
			if i < len(declared) && !javaTypeMentionsNames(declared[i], types.ClassFormalTypeParamNames(ownerSig)) {
				params, methodFormals = declared, types.MethodFormalTypeParamNames(sig)
			}
		}
	}
	if len(methodFormals) != 0 || i >= len(params) || !sourceDenotableJavaType(params[i], ctx) {
		return ""
	}
	// A class variable hidden by the caller's own method variable cannot be
	// spelled unambiguously at this call site. Equal names are not identities.
	for _, local := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
		for _, class := range ctx.ClassTypeParams {
			if local == class && javaTypeMentionsNames(params[i], []string{class}) {
				return ""
			}
		}
	}
	formal, parameterized := types.AsParameterizedType(params[i])
	if !parameterized || !sameErasureClassName(formal.RawClassName, selected) {
		return ""
	}
	if !f.isCurrentClass(ctx) {
		// The erased class is already the selected parameter's source type.
		// Every newly named bound also needs an accessibility proof; a public
		// external method may legally mention its private implementation type.
		for _, bound := range formal.TypeArgs {
			if !accessibleOverloadBound(bound, ctx) {
				return ""
			}
		}
	}
	kind := callbinding.Virtual
	if f.IsStatic {
		kind = callbinding.Static
	} else if f.Kind == InvokeInterface {
		kind = callbinding.Interface
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	declaration, known := ctx.InvocationMetadata(owner)
	if !known || declaration.Name != owner || !declaration.MembersComplete {
		return ""
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{
		Owner: owner, Name: f.FunctionName,
		Desc: f.Descriptor, Kind: kind,
	}, ctx.InvocationMetadata)
	// Positive evidence of a competing declaration does not require guessing
	// what an unavailable ancestor declares. We restore the selected formal;
	// this is not a claim that the entire overload family has been enumerated.
	if err != nil || family.Proof != callbinding.Compete || family.Target == nil || family.Target.Varargs || family.Target.Bridge {
		return ""
	}
	if !f.isCurrentClass(ctx) && (!declaration.Public || !family.Target.Public) {
		return ""
	}
	selectedDescriptor := "L" + strings.ReplaceAll(selected, ".", "/") + ";"
	for _, candidate := range family.Methods {
		if candidate.Static != f.IsStatic || candidate.Desc == f.Descriptor {
			continue
		}
		other, _, err := callbinding.Descriptor(candidate.Desc)
		if err != nil || i >= len(other) || other[i] == selectedDescriptor || !callbinding.Reference(other[i]) {
			continue
		}
		if strings.HasPrefix(other[i], "L") {
			otherClass := strings.ReplaceAll(other[i][1:len(other[i])-1], "/", ".")
			if provenOverloadWidening(actual.Name, otherClass, ctx) {
				return params[i].String(ctx)
			}
		}
	}
	return ""
}

func provenOverloadWidening(actual, formal string, ctx *class_context.ClassContext) bool {
	if sameErasureClassName(actual, formal) {
		return true
	}
	a, b := "L"+strings.ReplaceAll(actual, ".", "/")+";", "L"+strings.ReplaceAll(formal, ".", "/")+";"
	// A missing platform method table does not invalidate known hierarchy
	// edges, and using those edges must not invent member-table completeness.
	return callbinding.Assignable(a, b, ctx.InvocationMetadata) || types.IsReferenceSubtypeBridged(actual, formal, ctx.SiblingSuperTypes)
}

func accessibleOverloadBound(typ types.JavaType, ctx *class_context.ClassContext) bool {
	if typ == nil || ctx == nil {
		return false
	}
	if w, ok := typ.(*types.JavaWildcardType); ok {
		return w.Bound == nil || accessibleOverloadBound(w.Bound, ctx)
	}
	if typ.IsArray() {
		return accessibleOverloadBound(typ.ElementType(), ctx)
	}
	name, ok := types.RawClassFQN(typ)
	if !ok {
		return false
	}
	if ctx.IsTypeParam(name) {
		return true
	}
	internal := strings.ReplaceAll(name, ".", "/")
	accessible, known := false, false
	if ctx.SiblingClassAccessible != nil {
		accessible, known = ctx.SiblingClassAccessible(internal)
	}
	if !known {
		if ctx.InvocationMetadata == nil {
			return false
		}
		metadata, found := ctx.InvocationMetadata(internal)
		accessible = found && metadata.Name == internal && metadata.Public
	}
	if !accessible {
		return false
	}
	if p, ok := types.AsParameterizedType(typ); ok {
		for _, arg := range p.TypeArgs {
			if !accessibleOverloadBound(arg, ctx) {
				return false
			}
		}
	}
	return true
}
