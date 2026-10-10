package core

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// NewCondyValue builds a CONSTANT_Dynamic placeholder. The bootstrap is never executed.
func NewCondyValue(name, desc string, bsm uint16, malformed bool, reason string) *values.CustomValue {
	flag := "condy_unsupported"
	if malformed {
		flag = "condy_invalid"
	}
	cv := values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		_ = funcCtx
		if malformed {
			return fmt.Sprintf("/* invalid condy %s:%s bsm=%d %s */ null", name, desc, bsm, reason)
		}
		return fmt.Sprintf("/* unsupported condy %s:%s bsm=%d */ null", name, desc, bsm)
	}, func() types.JavaType {
		if t, err := types.ParseDescriptor(desc); err == nil && t != nil {
			return t
		}
		return types.NewJavaClass("java.lang.Object")
	})
	cv.Flag = flag
	return cv
}

// ClassifyCondy returns invalid vs unsupported for a CONSTANT_Dynamic site.
// bootstrapCount is the BootstrapMethods table length; index is 0-based.
func ClassifyCondy(name, desc string, bootstrapIndex int, bootstrapCount int) DispatchResult {
	req := CallSiteRequest{
		Identity:           BootstrapIdentity{Owner: "constant_dynamic", Name: name, Descriptor: desc, RefKind: RefInvokeStatic},
		CallSiteName:       name,
		CallSiteDescriptor: desc,
	}
	obj := types.NewJavaClass("java.lang.Object")
	if name == "" || desc == "" {
		res := invalidDispatch(req, FamilyCondy, DiagCondyInvalid, "condy missing name or descriptor", obj)
		res.Value = NewCondyValue(name, desc, uint16(max(0, bootstrapIndex)), true, res.Reason)
		return res
	}
	if bootstrapIndex < 0 || bootstrapIndex >= bootstrapCount {
		res := invalidDispatch(req, FamilyCondy, DiagCondyInvalid,
			fmt.Sprintf("condy bootstrap index %d out of range [0,%d)", bootstrapIndex, bootstrapCount),
			obj)
		res.Value = NewCondyValue(name, desc, uint16(max(0, bootstrapIndex)), true, res.Reason)
		return res
	}
	res := unsupportedDispatch(req, FamilyCondy, DiagCondyUnsupported,
		"legal ConstantDynamic has no proven reconstruction; bootstrap not executed",
		obj)
	res.Value = NewCondyValue(name, desc, uint16(bootstrapIndex), false, "")
	res.Capability = EvaluateCapability(FamilyCondy, 0, 0)
	return res
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
