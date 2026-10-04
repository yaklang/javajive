package core

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// JVMS 5.4.3.5 method-handle reference kinds.
const (
	RefGetField         uint8 = 1
	RefGetStatic        uint8 = 2
	RefPutField         uint8 = 3
	RefPutStatic        uint8 = 4
	RefInvokeVirtual    uint8 = 5
	RefInvokeStatic     uint8 = 6
	RefInvokeSpecial    uint8 = 7
	RefNewInvokeSpecial uint8 = 8
	RefInvokeInterface  uint8 = 9
)

// FeatureFamily is a whitelist family, not a string suffix.
type FeatureFamily string

const (
	FamilyUnknown    FeatureFamily = "unknown"
	FamilyConcat     FeatureFamily = "string_concat"
	FamilyLambda     FeatureFamily = "lambda_metafactory"
	FamilyRecord     FeatureFamily = "object_methods"
	FamilyTypeSwitch FeatureFamily = "type_switch"
	FamilyEnumSwitch FeatureFamily = "enum_switch"
	FamilyCondy      FeatureFamily = "constant_dynamic"
)

// Diagnostic codes recorded on DispatchResult and DecompileResult.Diagnostics.
const (
	DiagBootstrapUnknown     = "bootstrap_unknown"
	DiagBootstrapInvalid     = "bootstrap_invalid"
	DiagBootstrapVersion     = "bootstrap_unsupported_version"
	DiagBootstrapPartial     = "bootstrap_partial"
	DiagCondyUnsupported     = "condy_unsupported"
	DiagCondyInvalid         = "condy_invalid"
	DiagBootstrapNeighbor    = "bootstrap_identity_neighbor"
	DiagBootstrapArgMismatch = "bootstrap_argument_mismatch"
)

// BootstrapIdentity is the full symbol identity of a bootstrap method handle.
// All four fields must match a builtin for a whitelist hit. Owner is dotted.
type BootstrapIdentity struct {
	Owner      string
	Name       string
	Descriptor string
	RefKind    uint8
}

func (id BootstrapIdentity) Normalized() BootstrapIdentity {
	id.Owner = NormalizeBootstrapOwner(id.Owner)
	return id
}

func (id BootstrapIdentity) Format() string {
	id = id.Normalized()
	return fmt.Sprintf("%s.%s:%s kind=%d", id.Owner, id.Name, id.Descriptor, id.RefKind)
}

func (id BootstrapIdentity) Equal(other BootstrapIdentity) bool {
	a, b := id.Normalized(), other.Normalized()
	return a.Owner == b.Owner && a.Name == b.Name && a.Descriptor == b.Descriptor && a.RefKind == b.RefKind
}

// NormalizeBootstrapOwner maps internal names to dotted binary names.
func NormalizeBootstrapOwner(owner string) string {
	return strings.ReplaceAll(owner, "/", ".")
}

// CallSiteRequest is the frozen adapter input.
type CallSiteRequest struct {
	Identity            BootstrapIdentity
	CallSiteName        string
	CallSiteDescriptor  string
	StaticArgs          []values.JavaValue
	StaticArgTags       []uint8
	DynamicArgs         []values.JavaValue
	TargetSourceVersion int
	ClassMajor          uint16
	OriginPC            int
}

// DispatchResult is the frozen adapter output. ExecutedBootstrap must stay false.
type DispatchResult struct {
	Status            string
	Family            FeatureFamily
	Identity          BootstrapIdentity
	Value             values.JavaValue
	Reason            string
	DiagnosticCode    string
	Capability        Capability
	ExecutedBootstrap bool
	Effects           values.Effects
}

// BootstrapAdapter reconstructs a callsite without executing the handle.
type BootstrapAdapter func(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult

var (
	builtinByIdentity = map[BootstrapIdentity]FeatureFamily{}
	familyMu          sync.RWMutex
	familyAdapters    = map[FeatureFamily]BootstrapAdapter{}
	defaultAdapters   = map[FeatureFamily]BootstrapAdapter{}
)

func init() {
	registerBuiltin(IdentityMakeConcat, FamilyConcat)
	registerBuiltin(IdentityMakeConcatWithConstants, FamilyConcat)
	registerBuiltin(IdentityLambdaMetafactory, FamilyLambda)
	registerBuiltin(IdentityLambdaAltMetafactory, FamilyLambda)
	registerBuiltin(IdentityObjectMethods, FamilyRecord)
	registerBuiltin(IdentityTypeSwitch, FamilyTypeSwitch)
	registerBuiltin(IdentityEnumSwitch, FamilyEnumSwitch)
}

func registerBuiltin(id BootstrapIdentity, fam FeatureFamily) {
	builtinByIdentity[id.Normalized()] = fam
}

// SetFamilyAdapter replaces reconstruction for a family. Identity matching stays in T17.
// Passing nil removes the override so the T17 default adapter is used again.
func SetFamilyAdapter(family FeatureFamily, adapter BootstrapAdapter) {
	familyMu.Lock()
	defer familyMu.Unlock()
	if adapter == nil {
		delete(familyAdapters, family)
		return
	}
	familyAdapters[family] = adapter
}

// FamilyAdapter returns the override registered by SetFamilyAdapter, or nil.
func FamilyAdapter(family FeatureFamily) BootstrapAdapter {
	familyMu.RLock()
	defer familyMu.RUnlock()
	return familyAdapters[family]
}

func setDefaultAdapter(family FeatureFamily, adapter BootstrapAdapter) {
	familyMu.Lock()
	defer familyMu.Unlock()
	defaultAdapters[family] = adapter
}

func adapterFor(family FeatureFamily) BootstrapAdapter {
	familyMu.RLock()
	defer familyMu.RUnlock()
	if a := familyAdapters[family]; a != nil {
		return a
	}
	return defaultAdapters[family]
}

// LookupBuiltin returns the family for an exact four-field identity.
func LookupBuiltin(id BootstrapIdentity) (FeatureFamily, bool) {
	fam, ok := builtinByIdentity[id.Normalized()]
	return fam, ok
}

// AllBuiltinIdentities is the complete whitelist (for T17-M03).
func AllBuiltinIdentities() []BootstrapIdentity {
	out := make([]BootstrapIdentity, 0, len(builtinByIdentity))
	for id := range builtinByIdentity {
		out = append(out, id)
	}
	return out
}

// IdentityFromMember builds an identity from a resolved bootstrap MethodHandle member.
func IdentityFromMember(member *values.JavaClassMember) (BootstrapIdentity, error) {
	if member == nil {
		return BootstrapIdentity{}, fmt.Errorf("missing bootstrap method handle")
	}
	id := BootstrapIdentity{
		Owner:      NormalizeBootstrapOwner(member.Name),
		Name:       member.Member,
		Descriptor: member.Description,
		RefKind:    member.RefKind,
	}
	if id.Owner == "" || id.Name == "" || id.Descriptor == "" {
		return id, fmt.Errorf("incomplete bootstrap identity %s", id.Format())
	}
	if id.RefKind == 0 {
		return id, fmt.Errorf("bootstrap method handle missing reference_kind")
	}
	return id, nil
}

// DispatchInvokeDynamic is the production invokedynamic entry. It never runs the
// bootstrap handle. Unknown legal identities are unsupported; malformed ones invalid.
func DispatchInvokeDynamic(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
	req.Identity = req.Identity.Normalized()
	if d != nil && req.TargetSourceVersion == 0 {
		req.TargetSourceVersion = d.TargetSourceVersion
	}
	if d != nil && req.ClassMajor == 0 {
		req.ClassMajor = d.ClassMajor
	}
	if req.TargetSourceVersion == 0 {
		req.TargetSourceVersion = ClassMajorToSourceVersion(req.ClassMajor)
	}

	if req.Identity.Owner == "" || req.Identity.Name == "" || req.Identity.Descriptor == "" || req.Identity.RefKind == 0 {
		return invalidDispatch(req, FamilyUnknown, DiagBootstrapInvalid, "malformed bootstrap identity "+req.Identity.Format(), resultType)
	}

	fam, ok := LookupBuiltin(req.Identity)
	if !ok {
		return unsupportedDispatch(req, FamilyUnknown, DiagBootstrapUnknown, "unknown bootstrap "+req.Identity.Format(), resultType)
	}

	cap := EvaluateCapability(fam, req.TargetSourceVersion, req.ClassMajor)
	if !cap.Lossless {
		res := unsupportedDispatch(req, fam, DiagBootstrapVersion, cap.Notes, resultType)
		res.Capability = cap
		return res
	}

	if err := validateCallSiteDescriptor(req); err != nil {
		return invalidDispatch(req, fam, DiagBootstrapArgMismatch, err.Error(), resultType)
	}

	ad := adapterFor(fam)
	if ad == nil {
		res := unsupportedDispatch(req, fam, DiagBootstrapUnknown, "no adapter registered for "+string(fam), resultType)
		res.Capability = cap
		return res
	}
	out := ad(req, d, sim, resultType)
	out.Family = fam
	out.Identity = req.Identity
	out.Capability = cap
	out.ExecutedBootstrap = false
	if out.Value == nil {
		out.Value = unsupportedPlaceholder(req, resultType)
	}
	return out
}

func validateCallSiteDescriptor(req CallSiteRequest) error {
	if req.CallSiteDescriptor == "" {
		return fmt.Errorf("missing callsite descriptor")
	}
	ft, err := types.ParseMethodDescriptor(req.CallSiteDescriptor)
	if err != nil {
		return fmt.Errorf("invalid callsite descriptor %q: %w", req.CallSiteDescriptor, err)
	}
	n := 0
	if ft != nil && ft.FunctionType() != nil {
		n = len(ft.FunctionType().ParamTypes)
	}
	if len(req.DynamicArgs) != n {
		return fmt.Errorf("dynamic operand count %d != descriptor arity %d", len(req.DynamicArgs), n)
	}
	return nil
}

func invalidDispatch(req CallSiteRequest, fam FeatureFamily, code, reason string, resultType types.JavaType) DispatchResult {
	return DispatchResult{
		Status:            "invalid_input",
		Family:            fam,
		Identity:          req.Identity,
		Value:             unsupportedPlaceholder(req, resultType),
		Reason:            reason,
		DiagnosticCode:    code,
		ExecutedBootstrap: false,
		Effects:           values.EffectOpaque,
	}
}

func unsupportedDispatch(req CallSiteRequest, fam FeatureFamily, code, reason string, resultType types.JavaType) DispatchResult {
	return DispatchResult{
		Status:            "unsupported",
		Family:            fam,
		Identity:          req.Identity,
		Value:             unsupportedPlaceholder(req, resultType),
		Reason:            reason,
		DiagnosticCode:    code,
		ExecutedBootstrap: false,
		Effects:           values.EffectOpaque,
	}
}

func okDispatch(req CallSiteRequest, fam FeatureFamily, v values.JavaValue, effects values.Effects) DispatchResult {
	return DispatchResult{
		Status:            "",
		Family:            fam,
		Identity:          req.Identity,
		Value:             v,
		ExecutedBootstrap: false,
		Effects:           effects,
	}
}

func unsupportedPlaceholder(req CallSiteRequest, resultType types.JavaType) values.JavaValue {
	id := req.Identity.Format()
	cs := req.CallSiteName + ":" + req.CallSiteDescriptor
	typ := resultType
	cv := values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		_ = funcCtx
		return fmt.Sprintf("/* unsupported bootstrap %s callsite %s */ null", id, cs)
	}, func() types.JavaType {
		if typ == nil {
			return types.NewJavaClass("java.lang.Object")
		}
		return typ
	})
	cv.Flag = "bootstrap_unsupported"
	return cv
}

// CountConcatRecipeTags counts U+0001 (dynamic) and U+0002 (static const) in a recipe.
// Literal U+0001/U+0002 in constants are NOT in the recipe string as tags; they appear
// as data only inside the recipe's ordinary characters — tags are single UTF-16 units.
func CountConcatRecipeTags(recipe string) (args, consts int) {
	for _, r := range recipe {
		switch r {
		case '\u0001':
			args++
		case '\u0002':
			consts++
		}
	}
	return args, consts
}

// LiteralStringData returns the raw Java string from a bootstrap static argument.
func LiteralStringData(v values.JavaValue) (string, bool) {
	units, ok := LiteralStringUnits(v)
	if !ok {
		return "", false
	}
	return string(utf16.Decode(units)), true
}

// LiteralStringUnits returns lossless UTF-16 of a string constant (recipe / concat payload).
func LiteralStringUnits(v values.JavaValue) ([]uint16, bool) {
	if v == nil {
		return nil, false
	}
	lit, ok := v.(*values.JavaLiteral)
	if !ok || lit == nil {
		return nil, false
	}
	if len(lit.Units) > 0 {
		out := make([]uint16, len(lit.Units))
		copy(out, lit.Units)
		return out, true
	}
	if s, ok := lit.Data.(string); ok {
		return utf16.Encode([]rune(s)), true
	}
	return nil, false
}

// ParamCountOfDescriptor returns the number of method parameters in a JVM descriptor.
func ParamCountOfDescriptor(desc string) (int, error) {
	ft, err := types.ParseMethodDescriptor(desc)
	if err != nil {
		return 0, err
	}
	if ft == nil || ft.FunctionType() == nil {
		return 0, fmt.Errorf("not a method descriptor")
	}
	return len(ft.FunctionType().ParamTypes), nil
}
