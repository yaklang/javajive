package core

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func init() {
	setDefaultAdapter(FamilyConcat, defaultConcatAdapter)
	setDefaultAdapter(FamilyLambda, defaultLambdaAdapter)
	setDefaultAdapter(FamilyRecord, defaultObjectMethodsAdapter)
	setDefaultAdapter(FamilyTypeSwitch, defaultUnsupportedFamilyAdapter(FamilyTypeSwitch, "typeSwitch reconstruction is T21"))
	setDefaultAdapter(FamilyEnumSwitch, defaultUnsupportedFamilyAdapter(FamilyEnumSwitch, "enumSwitch reconstruction is T21"))
}

func defaultUnsupportedFamilyAdapter(fam FeatureFamily, reason string) BootstrapAdapter {
	return func(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
		return unsupportedDispatch(req, fam, DiagBootstrapUnknown, reason, resultType)
	}
}

func defaultConcatAdapter(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
	if err := validateConcatRequest(req); err != nil {
		return invalidDispatch(req, FamilyConcat, DiagBootstrapArgMismatch, err.Error(), resultType)
	}
	key := "java.lang.invoke.StringConcatFactory." + req.Identity.Name
	f := buildinBootstrapMethods[key]
	if f == nil && req.Identity.Name == "makeConcat" {
		return okDispatch(req, FamilyConcat, renderMakeConcatPlus(req, resultType), values.EffectCall)
	}
	if f == nil {
		return unsupportedDispatch(req, FamilyConcat, DiagBootstrapUnknown, "concat adapter missing for "+req.Identity.Name, resultType)
	}
	val, err := f(req.StaticArgs...)(d, sim, resultType, req.DynamicArgs...)
	if err != nil {
		return invalidDispatch(req, FamilyConcat, DiagBootstrapArgMismatch, err.Error(), resultType)
	}
	return okDispatch(req, FamilyConcat, val, values.EffectCall)
}

func validateConcatRequest(req CallSiteRequest) error {
	switch req.Identity.Name {
	case "makeConcatWithConstants":
		if len(req.StaticArgs) < 1 {
			return fmt.Errorf("makeConcatWithConstants missing recipe string")
		}
		recipe, ok := LiteralStringData(req.StaticArgs[0])
		if !ok {
			return fmt.Errorf("makeConcatWithConstants recipe is not a string constant (tag error)")
		}
		nArg, nConst := CountConcatRecipeTags(recipe)
		params, err := ParamCountOfDescriptor(req.CallSiteDescriptor)
		if err != nil {
			return err
		}
		if nArg != params {
			return fmt.Errorf("recipe TAG_ARG count %d != callsite arity %d", nArg, params)
		}
		if nConst != len(req.StaticArgs)-1 {
			return fmt.Errorf("recipe TAG_CONST count %d != remaining static constants %d", nConst, len(req.StaticArgs)-1)
		}
	case "makeConcat":
		if len(req.StaticArgs) != 0 {
			return fmt.Errorf("makeConcat must not carry static recipe constants, got %d", len(req.StaticArgs))
		}
	default:
		return fmt.Errorf("unknown concat member %s", req.Identity.Name)
	}
	return nil
}

func renderMakeConcatPlus(req CallSiteRequest, resultType types.JavaType) values.JavaValue {
	args := append([]values.JavaValue(nil), req.DynamicArgs...)
	typ := resultType
	return values.NewStreamingCustomValue(func(funcCtx *class_context.ClassContext, out *workbudget.Writer) error {
		if len(args) == 0 {
			return out.WriteString(`""`)
		}
		// DynamicArgs arrive in the same order as the old invokedynamic pop loop
		// (last param first). Restore left-to-right evaluation.
		for i := len(args) - 1; i >= 0; i-- {
			arg := args[i]
			if i != len(args)-1 {
				if err := out.WriteString(" + "); err != nil {
					return err
				}
			}
			paren := concatArgNeedsParens(arg)
			if paren {
				if err := out.WriteString("("); err != nil {
					return err
				}
			}
			s := ""
			if arg != nil {
				s = arg.String(funcCtx)
			}
			if funcCtx != nil && funcCtx.Work != nil && funcCtx.Work.Err() != nil {
				return funcCtx.Work.Err()
			}
			if err := out.WriteString(s); err != nil {
				return err
			}
			if paren {
				if err := out.WriteString(")"); err != nil {
					return err
				}
			}
		}
		return nil
	}, func() types.JavaType {
		if typ == nil {
			return types.NewJavaPrimer(types.JavaString)
		}
		return typ
	})
}

func defaultLambdaAdapter(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
	if err := validateLambdaRequest(req); err != nil {
		return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, err.Error(), resultType)
	}
	if req.Identity.Name == "altMetafactory" {
		if res, handled := inspectAltMetafactoryFlags(req, resultType); handled {
			return res
		}
	}
	f := buildinBootstrapMethods["java.lang.invoke.LambdaMetafactory.metafactory"]
	if f == nil {
		return unsupportedDispatch(req, FamilyLambda, DiagBootstrapUnknown, "lambda adapter missing", resultType)
	}
	val, err := f(req.StaticArgs...)(d, sim, resultType, req.DynamicArgs...)
	if err != nil {
		return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, err.Error(), resultType)
	}
	return okDispatch(req, FamilyLambda, val, values.EffectCall|values.EffectAllocate)
}

const (
	lambdaFlagSerializable = 1
	lambdaFlagMarkers      = 2
	lambdaFlagBridges      = 4
)

func validateLambdaRequest(req CallSiteRequest) error {
	switch req.Identity.Name {
	case "metafactory":
		if len(req.StaticArgs) < 3 {
			return fmt.Errorf("metafactory requires samMethodType, implMethod, instantiatedMethodType, got %d static args", len(req.StaticArgs))
		}
		if len(req.StaticArgs) > 3 {
			return fmt.Errorf("metafactory extra static args: %d", len(req.StaticArgs))
		}
	case "altMetafactory":
		if len(req.StaticArgs) < 4 {
			return fmt.Errorf("altMetafactory requires sam, impl, instantiated, flags, got %d", len(req.StaticArgs))
		}
	default:
		return fmt.Errorf("unknown lambda member %s", req.Identity.Name)
	}
	return nil
}

func inspectAltMetafactoryFlags(req CallSiteRequest, resultType types.JavaType) (DispatchResult, bool) {
	flags, ok := intFromLiteral(req.StaticArgs[3])
	if !ok {
		return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, "altMetafactory flags is not an int", resultType), true
	}
	unknown := flags &^ (lambdaFlagSerializable | lambdaFlagMarkers | lambdaFlagBridges)
	if unknown != 0 {
		return unsupportedDispatch(req, FamilyLambda, DiagBootstrapUnknown,
			fmt.Sprintf("altMetafactory unknown flags 0x%x", flags), resultType), true
	}
	if flags&lambdaFlagSerializable != 0 {
		// T19 may later prove a supported serializable subset. Default: do not drop the flag.
		return unsupportedDispatch(req, FamilyLambda, DiagBootstrapUnknown,
			"altMetafactory FLAG_SERIALIZABLE is not silently ignored", resultType), true
	}
	idx := 4
	if flags&lambdaFlagMarkers != 0 {
		if idx >= len(req.StaticArgs) {
			return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, "altMetafactory FLAG_MARKERS missing marker count", resultType), true
		}
		n, ok := intFromLiteral(req.StaticArgs[idx])
		if !ok || n < 0 {
			return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, "altMetafactory marker count invalid", resultType), true
		}
		idx++
		if idx+n > len(req.StaticArgs) {
			return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, "altMetafactory marker interfaces truncated", resultType), true
		}
		idx += n
	}
	if flags&lambdaFlagBridges != 0 {
		if idx >= len(req.StaticArgs) {
			return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, "altMetafactory FLAG_BRIDGES missing bridge count", resultType), true
		}
		n, ok := intFromLiteral(req.StaticArgs[idx])
		if !ok || n < 0 {
			return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, "altMetafactory bridge count invalid", resultType), true
		}
		idx++
		if idx+n > len(req.StaticArgs) {
			return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch, "altMetafactory bridges truncated", resultType), true
		}
		idx += n
	}
	if idx != len(req.StaticArgs) {
		return invalidDispatch(req, FamilyLambda, DiagBootstrapArgMismatch,
			fmt.Sprintf("altMetafactory extra static args: consumed %d of %d", idx, len(req.StaticArgs)), resultType), true
	}
	return DispatchResult{}, false
}

func intFromLiteral(v values.JavaValue) (int, bool) {
	lit, ok := v.(*values.JavaLiteral)
	if !ok || lit == nil {
		return 0, false
	}
	switch n := lit.Data.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint32:
		return int(n), true
	default:
		return 0, false
	}
}

func defaultObjectMethodsAdapter(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
	if len(req.StaticArgs) < 2 {
		return invalidDispatch(req, FamilyRecord, DiagBootstrapArgMismatch,
			"ObjectMethods.bootstrap requires record class and names", resultType)
	}
	f := buildinBootstrapMethods["java.lang.runtime.ObjectMethods.bootstrap"]
	if f == nil {
		return unsupportedDispatch(req, FamilyRecord, DiagBootstrapUnknown, "ObjectMethods adapter missing", resultType)
	}
	val, err := f(req.StaticArgs...)(d, sim, resultType, req.DynamicArgs...)
	if err != nil {
		return invalidDispatch(req, FamilyRecord, DiagBootstrapArgMismatch, err.Error(), resultType)
	}
	return okDispatch(req, FamilyRecord, val, values.EffectCall|values.EffectReadMemory)
}
