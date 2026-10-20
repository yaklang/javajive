package core

import (
	"errors"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// ErrLambdaSourceUnsupported distinguishes a source-regeneration limitation
// reported by a qualified original-body callback from malformed metafactory
// arguments. Wrappers must preserve this identity; matching message text is
// never an error-classification proof. This grants no source/body permission.
var ErrLambdaSourceUnsupported = errors.New("original lambda has no proven source representation")

// A qualified callback can prove that no expression may be emitted. Keep the
// valid-bootstrap diagnostic, but publish no placeholder value. The caller
// must fail this method's source construction instead of substituting null.
func unsupportedLambdaSource(req CallSiteRequest, reason string) DispatchResult {
	return DispatchResult{Status: "unsupported", Family: FamilyLambda, Identity: req.Identity, Reason: reason, DiagnosticCode: DiagBootstrapUnknown, Effects: values.EffectOpaque}
}
