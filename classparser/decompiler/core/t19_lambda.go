package core

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

func init() {
	SetFamilyAdapter(FamilyLambda, t19LambdaAdapter)
}

func t19LambdaAdapter(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
	if err := validateLambdaRequest(req); err != nil {
		return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, err.Error(), resultType)
	}
	static := req.StaticArgs
	if req.Identity.Name == "altMetafactory" {
		res, handled := inspectAltMetafactoryFlags(req, resultType)
		if handled {
			return res
		}
		if len(static) < 3 {
			return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, "altMetafactory missing sam/impl/instantiated", resultType)
		}
		static = static[:3]
	}
	if len(static) < 2 {
		return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, "lambda metafactory requires impl method handle", resultType)
	}
	impl, ok := static[1].(*values.JavaClassMember)
	if !ok || impl == nil {
		return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch,
			fmt.Sprintf("lambda metafactory: unexpected impl method handle type %T", static[1]), resultType)
	}

	// Keep the original DynamicArgs (including SlotValues). The builtin metafactory
	// restores capture order itself; unpacking here would freeze pre-rebind identities
	// and swap captures (T19-C01 / T17-C06).
	capturedEval := t19EvalOrder(req.DynamicArgs)

	if d == nil || d.FunctionContext == nil {
		return unsupportedDispatch(req, FamilyLambda, DiagBootstrapUnknown, "lambda reconstruction requires decompiler context", resultType)
	}

	samN := t19SAMParamCount(static)
	if t19ShouldInline(d, impl, len(capturedEval), samN) {
		f := buildinBootstrapMethods["java.lang.invoke.LambdaMetafactory.metafactory"]
		if strings.HasPrefix(impl.Member, "lambda$") {
			if f == nil {
				return unsupportedDispatch(req, FamilyLambda, DiagBootstrapUnknown, "lambda adapter missing", resultType)
			}
			val, err := f(static...)(d, sim, resultType, req.DynamicArgs...)
			if err != nil {
				return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, err.Error(), resultType)
			}
			return okDispatch(req, FamilyLambda, val, values.EffectCall|values.EffectAllocate)
		}
		val, err := t19InlineLambda(req, d, static, impl, capturedEval, resultType)
		if err != nil {
			return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, err.Error(), resultType)
		}
		return okDispatch(req, FamilyLambda, val, values.EffectCall|values.EffectAllocate)
	}

	val := t19MethodRef(req, d, static, impl, req.DynamicArgs, resultType)
	return okDispatch(req, FamilyLambda, val, values.EffectCall|values.EffectAllocate)
}

func t19EvalOrder(pop []values.JavaValue) []values.JavaValue {
	out := make([]values.JavaValue, len(pop))
	for i := range pop {
		out[i] = pop[len(pop)-1-i]
	}
	return out
}

func t19SAMParamCount(static []values.JavaValue) int {
	if len(static) >= 3 {
		if n, ok := t19MethodTypeParamCount(static[2]); ok {
			return n
		}
	}
	if len(static) >= 1 {
		if n, ok := t19MethodTypeParamCount(static[0]); ok {
			return n
		}
	}
	return 0
}

func t19MethodTypeParamCount(v values.JavaValue) (int, bool) {
	desc := t19MethodTypeDesc(v)
	if desc == "" {
		return 0, false
	}
	n, err := ParamCountOfDescriptor(desc)
	if err != nil {
		return 0, false
	}
	return n, true
}

func t19MethodTypeDesc(v values.JavaValue) string {
	if v == nil {
		return ""
	}
	if cv, ok := v.(*values.CustomValue); ok {
		s := cv.String(&class_context.ClassContext{})
		if strings.HasPrefix(s, "(") {
			return s
		}
	}
	if s, ok := LiteralStringData(v); ok && strings.HasPrefix(s, "(") {
		return s
	}
	s := v.String(&class_context.ClassContext{})
	if strings.HasPrefix(s, "(") {
		return s
	}
	return ""
}

// t19ShouldInline decides lambda-body inlining from handle identity (kind, owner, name, desc)
// plus capture/SAM arity. `lambda$` is only a hint for same-class synthetic bodies.
func t19ShouldInline(d *Decompiler, impl *values.JavaClassMember, captured, samN int) bool {
	if d == nil || d.FunctionContext == nil || impl == nil {
		return false
	}
	if impl.Member == "<init>" || impl.RefKind == RefNewInvokeSpecial {
		return false
	}
	owner := NormalizeBootstrapOwner(impl.Name)
	current := NormalizeBootstrapOwner(d.FunctionContext.ClassName)
	if owner != current {
		return false
	}
	// Same-class synthetic bodies (javac `lambda$...`) must inline even when the
	// impl handle is invokevirtual/interface. Skipping those kinds first turned
	// `y -> y + field + x` into `var1::lambda$instanceCapture$5` (int receiver).
	if strings.HasPrefix(impl.Member, "lambda$") {
		return true
	}
	// Kotlin synthetics (`set$lambda-0`) are named methods that must stay as
	// method references with SafeIdentifier, not inlined (and dropped) bodies.
	if strings.Contains(impl.Member, "$lambda-") {
		return false
	}
	switch impl.RefKind {
	case RefInvokeVirtual, RefInvokeInterface:
		return false
	}
	implN, err := ParamCountOfDescriptor(impl.Description)
	if err != nil {
		return false
	}
	switch impl.RefKind {
	case RefInvokeStatic, 0:
		if captured == 0 {
			return false
		}
		return implN == captured+samN
	case RefInvokeSpecial:
		if captured == 0 {
			return false
		}
		if captured == 1 && implN == samN {
			return false
		}
		return implN == captured+samN || implN == captured-1+samN
	default:
		return false
	}
}

func t19InlineLambda(req CallSiteRequest, d *Decompiler, static []values.JavaValue, impl *values.JavaClassMember, captured []values.JavaValue, resultType types.JavaType) (values.JavaValue, error) {
	if d.DumpClassLambdaMethod == nil {
		return nil, fmt.Errorf("DumpClassLambdaMethod is nil")
	}
	ctx := d.FunctionContext
	savedName := ctx.FunctionName
	savedType := ctx.FunctionType
	savedStatic := ctx.IsStatic
	savedSig := ctx.CurrentMethodSig
	savedDesc := ctx.CurrentMethodDesc
	defer func() {
		ctx.FunctionName = savedName
		ctx.FunctionType = savedType
		ctx.IsStatic = savedStatic
		ctx.CurrentMethodSig = savedSig
		ctx.CurrentMethodDesc = savedDesc
	}()

	var lambdaReplace func(oldId *utils.VariableId, newId *utils.VariableId)
	if d.getenv("JDEC_LAMBDA_CAPTURE_REBIND_OFF") == "" {
		lambdaReplace = func(oldId *utils.VariableId, newId *utils.VariableId) {
			for _, ca := range captured {
				if ca != nil {
					ca.ReplaceVar(oldId, newId)
				}
			}
		}
	}
	methodStr, err := d.DumpClassLambdaMethod(impl.Member, impl.Description, utils.NewRootVariableId(), len(captured))
	if err != nil {
		return nil, fmt.Errorf("dump lambda method `%s.%s` error: %w", impl.Name, impl.Member, err)
	}
	var retTypevarCast string
	if d.getenv("JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF") == "" {
		var instantiatedMT values.JavaValue
		if len(static) >= 3 {
			instantiatedMT = static[2]
		}
		retTypevarCast = lambdaReturnPositionTypevar(resultType, instantiatedMT)
	}
	typ := resultType
	stringFn := func(funcCtx *class_context.ClassContext) string {
		if funcCtx != nil && funcCtx.Work != nil && funcCtx.Work.RenderGuarded() {
			if err := funcCtx.Work.Check(); err != nil {
				return ""
			}
		}
		s := methodStr
		for i, ca := range captured {
			name := ""
			if ca != nil {
				name = ca.String(funcCtx)
			}
			if funcCtx != nil && funcCtx.Work != nil && funcCtx.Work.Err() != nil {
				return ""
			}
			s = strings.ReplaceAll(s, fmt.Sprintf("\x00LCAP%d\x00", i), name)
		}
		if retTypevarCast != "" {
			castTarget := resolveLambdaReturnTypevar(funcCtx, retTypevarCast)
			if castTarget != "" {
				s = injectLambdaReturnCast(s, castTarget)
			}
		}
		if funcCtx != nil && funcCtx.Work != nil && funcCtx.Work.RenderGuarded() {
			w := workbudget.NewWriter(funcCtx.Work)
			w.SetBase(funcCtx.OutputHeld)
			if err := w.WriteString(s); err != nil {
				return ""
			}
			return w.String()
		}
		return s
	}
	cv := values.NewCustomValue(stringFn, func() types.JavaType { return typ }, lambdaReplace)
	cv.Flag = "lambda"
	cv.NoOuterCapture = len(captured) == 0
	if len(static) >= 3 {
		if upgradedType := inferLambdaTypeFromInstantiated(typ, static[2]); upgradedType != nil {
			lambdaType := upgradedType
			cv = values.NewCustomValue(cv.StringFunc, func() types.JavaType { return lambdaType }, lambdaReplace)
			cv.Flag = "lambda"
			cv.NoOuterCapture = len(captured) == 0
		}
	}
	cv.CapturesKnown = true
	cv.Captures = captured
	return cv, nil
}

func t19MethodRef(req CallSiteRequest, d *Decompiler, static []values.JavaValue, impl *values.JavaClassMember, capturedPop []values.JavaValue, resultType types.JavaType) values.JavaValue {
	// Bound receivers are the callsite's first (and usually only) captured argument.
	// DynamicArgs arrive in stack-pop order, matching the historical method-ref renderer.
	capturedArgs := append([]values.JavaValue{}, capturedPop...)
	refType := resultType
	if d.getenv("JDEC_METHODREF_INSTANTIATED_TYPE_OFF") == "" && len(static) >= 3 {
		if up := t19UpgradeFI(resultType, static[2]); up != nil {
			refType = up
		}
	}
	var refReplace func(oldId *utils.VariableId, newId *utils.VariableId)
	if d.getenv("JDEC_LAMBDA_CAPTURE_REBIND_OFF") == "" {
		refReplace = func(oldId *utils.VariableId, newId *utils.VariableId) {
			for _, ca := range capturedArgs {
				if ca != nil {
					ca.ReplaceVar(oldId, newId)
				}
			}
		}
	}
	kind := impl.RefKind
	member := impl.Member
	owner := impl.Name
	refVal := values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		return t19RenderMethodRef(funcCtx, kind, owner, member, capturedArgs)
	}, func() types.JavaType {
		return refType
	}, refReplace)
	refVal.Flag = "lambda"
	refVal.CapturesKnown = true
	refVal.Captures = capturedArgs
	refVal.NoOuterCapture = len(capturedArgs) == 0
	refVal.IsMethodRef = true
	if len(static) >= 3 {
		if desc := t19MethodTypeDesc(static[2]); desc != "" {
			refVal.InstantiatedMtdDesc = desc
		}
	}
	return refVal
}

func t19RenderMethodRef(funcCtx *class_context.ClassContext, kind uint8, owner, member string, captured []values.JavaValue) string {
	typeName := t19OwnerSource(owner, funcCtx)
	if member == "<init>" || kind == RefNewInvokeSpecial {
		if jdecenv.Get("JDEC_CTOR_METHODREF_FIX_OFF") == "" {
			return typeName + "::new"
		}
		return typeName + "::" + class_context.SafeIdentifier("new")
	}
	if strings.HasPrefix(owner, "[") && (member == "<init>" || member == "new") {
		return typeName + "::new"
	}
	switch kind {
	case RefInvokeVirtual, RefInvokeSpecial, RefInvokeInterface:
		if len(captured) > 0 && captured[0] != nil {
			return captured[0].String(funcCtx) + "::" + class_context.SafeIdentifier(member)
		}
		return typeName + "::" + class_context.SafeIdentifier(member)
	default:
		if len(captured) > 0 && kind != RefInvokeStatic && captured[0] != nil {
			return captured[0].String(funcCtx) + "::" + class_context.SafeIdentifier(member)
		}
		return typeName + "::" + class_context.SafeIdentifier(member)
	}
}

func t19UpgradeFI(rawType types.JavaType, instantiatedMethodType values.JavaValue) types.JavaType {
	if up := inferLambdaTypeFromInstantiated(rawType, instantiatedMethodType); up != nil {
		return up
	}
	if rawType == nil {
		return nil
	}
	jc, ok := rawType.RawType().(*types.JavaClass)
	if !ok || jc == nil {
		return nil
	}
	desc := t19MethodTypeDesc(instantiatedMethodType)
	if desc == "" {
		return nil
	}
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil || mt == nil || mt.FunctionType() == nil {
		return nil
	}
	mtParams := mt.FunctionType().ParamTypes
	mtRet := mt.FunctionType().ReturnType
	switch jc.Name {
	case "java.util.function.ToIntFunction", "java.util.function.ToLongFunction", "java.util.function.ToDoubleFunction":
		if len(mtParams) >= 1 {
			return types.NewParameterizedType(jc.Name, []types.JavaType{mtParams[0]})
		}
	case "java.util.function.IntFunction", "java.util.function.LongFunction", "java.util.function.DoubleFunction":
		if mtRet != nil {
			return types.NewParameterizedType(jc.Name, []types.JavaType{mtRet})
		}
	case "java.util.function.ToIntBiFunction":
		if len(mtParams) >= 2 {
			return types.NewParameterizedType(jc.Name, []types.JavaType{mtParams[0], mtParams[1]})
		}
	}
	return nil
}

func t19OwnerSource(owner string, funcCtx *class_context.ClassContext) string {
	owner = strings.ReplaceAll(owner, "/", ".")
	if strings.HasPrefix(owner, "[") {
		t, err := types.ParseDescriptor(owner)
		if err == nil && t != nil {
			if funcCtx == nil {
				funcCtx = &class_context.ClassContext{}
			}
			return t.String(funcCtx)
		}
	}
	if funcCtx == nil {
		funcCtx = &class_context.ClassContext{}
	}
	return funcCtx.ShortTypeName(owner)
}
