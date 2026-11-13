// Package report provides source-aware diagnostics for DEX processing.
package report

import (
	"errors"
	"fmt"
	"strings"
)

// FailureKind classifies why DEX processing failed. Values are stable and may
// be persisted by callers.
type FailureKind string

const (
	FailureKindInvalidInput  FailureKind = "invalid_input"
	FailureKindUnsupported   FailureKind = "unsupported"
	FailureKindResourceLimit FailureKind = "resource_limit"
	FailureKindCanceled      FailureKind = "canceled"
	FailureKindInternal      FailureKind = "internal"

	KindInvalidInput  = FailureKindInvalidInput
	KindUnsupported   = FailureKindUnsupported
	KindResourceLimit = FailureKindResourceLimit
	KindCanceled      = FailureKindCanceled
	KindInternal      = FailureKindInternal
)

// FailureScope identifies the smallest unit affected by a failure.
type FailureScope string

const (
	FailureScopeRequest     FailureScope = "request"
	FailureScopeInput       FailureScope = "input"
	FailureScopeContainer   FailureScope = "container"
	FailureScopeEntry       FailureScope = "entry"
	FailureScopeClass       FailureScope = "class"
	FailureScopeMember      FailureScope = "member"
	FailureScopeInstruction FailureScope = "instruction"

	ScopeRequest     = FailureScopeRequest
	ScopeInput       = FailureScopeInput
	ScopeContainer   = FailureScopeContainer
	ScopeEntry       = FailureScopeEntry
	ScopeClass       = FailureScopeClass
	ScopeMember      = FailureScopeMember
	ScopeInstruction = FailureScopeInstruction
)

// FailureStage identifies the processing stage at which a failure occurred.
type FailureStage string

const (
	FailureStageIdentify  FailureStage = "identify"
	FailureStageRead      FailureStage = "read"
	FailureStageParse     FailureStage = "parse"
	FailureStageValidate  FailureStage = "validate"
	FailureStageAnalyze   FailureStage = "analyze"
	FailureStageTranslate FailureStage = "translate"
	FailureStageLower     FailureStage = "lower"
	FailureStageEmit      FailureStage = "emit"
	FailureStageWrite     FailureStage = "write"

	StageIdentify  = FailureStageIdentify
	StageRead      = FailureStageRead
	StageParse     = FailureStageParse
	StageValidate  = FailureStageValidate
	StageAnalyze   = FailureStageAnalyze
	StageTranslate = FailureStageTranslate
	StageLower     = FailureStageLower
	StageEmit      = FailureStageEmit
	StageWrite     = FailureStageWrite
)

// RecoveryAction describes the bounded response permitted for a failure.
type RecoveryAction string

const (
	RecoveryActionNone             RecoveryAction = "none"
	RecoveryActionAbort            RecoveryAction = "abort"
	RecoveryActionRetry            RecoveryAction = "retry"
	RecoveryActionSkipEntry        RecoveryAction = "skip_entry"
	RecoveryActionSkipClass        RecoveryAction = "skip_class"
	RecoveryActionStubMethod       RecoveryAction = "stub_method"
	RecoveryActionStubClass        RecoveryAction = "stub_class"
	RecoveryActionStubClassMinimal RecoveryAction = "stub_class_minimal"
	RecoveryActionDropAttribute    RecoveryAction = "drop_attribute"

	RecoveryNone             = RecoveryActionNone
	RecoveryAbort            = RecoveryActionAbort
	RecoveryRetry            = RecoveryActionRetry
	RecoverySkipEntry        = RecoveryActionSkipEntry
	RecoverySkipClass        = RecoveryActionSkipClass
	RecoveryStubMethod       = RecoveryActionStubMethod
	RecoveryStubMember       = RecoveryActionStubMethod
	RecoveryStubClass        = RecoveryActionStubClass
	RecoveryStubClassMinimal = RecoveryActionStubClassMinimal
	RecoveryDropAttribute    = RecoveryActionDropAttribute
)

// Severity describes the operational impact of a failure.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
	SeverityFatal   Severity = "fatal"
)

// Failure is the stable, typed failure envelope used by DEX production and
// reporting. Counter remains a string so this package does not depend on an
// internal resource-budget package.
type Failure struct {
	Code       string
	Kind       FailureKind
	Scope      FailureScope
	Stage      FailureStage
	Action     RecoveryAction
	Severity   Severity
	Entry      string
	Class      string
	Member     string
	Descriptor string
	Offset     int64
	Op         string
	Feature    string
	Counter    string
	Used       int64
	Limit      int64
	Err        error
}

func (f *Failure) Error() string {
	if f == nil {
		return "dex failure: <nil>"
	}
	var b strings.Builder
	b.WriteString("dex failure")
	writeFailureString(&b, "code", f.Code)
	writeFailureString(&b, "kind", string(f.Kind))
	writeFailureString(&b, "scope", string(f.Scope))
	writeFailureString(&b, "stage", string(f.Stage))
	writeFailureString(&b, "action", string(f.Action))
	writeFailureString(&b, "severity", string(f.Severity))
	writeFailureString(&b, "entry", f.Entry)
	writeFailureString(&b, "class", f.Class)
	writeFailureString(&b, "member", f.Member)
	writeFailureString(&b, "descriptor", f.Descriptor)
	if f.Offset >= 0 {
		fmt.Fprintf(&b, " offset=0x%x", f.Offset)
	}
	writeFailureString(&b, "op", f.Op)
	writeFailureString(&b, "feature", f.Feature)
	writeFailureString(&b, "counter", f.Counter)
	if f.Counter != "" || f.Used != 0 || f.Limit != 0 {
		fmt.Fprintf(&b, " used=%d limit=%d", f.Used, f.Limit)
	}
	if f.Err != nil {
		fmt.Fprintf(&b, ": %v", f.Err)
	}
	return b.String()
}

func writeFailureString(b *strings.Builder, name, value string) {
	if value != "" {
		fmt.Fprintf(b, " %s=%q", name, value)
	}
}

func (f *Failure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.Err
}

// NewFailure wraps err in a typed failure. Nil errors remain nil and an error
// that already contains a Failure is returned unchanged.
func NewFailure(code string, kind FailureKind, scope FailureScope, stage FailureStage, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsFailure(err); ok {
		return err
	}
	return &Failure{
		Code:   code,
		Kind:   kind,
		Scope:  scope,
		Stage:  stage,
		Offset: -1,
		Err:    err,
	}
}

// AsFailure returns the first typed failure in err. When a legacy Diagnostic
// supplies source context outside the Failure, missing context is copied into
// the returned value without mutating either error.
func AsFailure(err error) (*Failure, bool) {
	var failure *Failure
	if !errors.As(err, &failure) || failure == nil {
		return nil, false
	}
	var diagnostic *Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic == nil {
		return failure, true
	}
	copy := *failure
	changed := false
	if copy.Entry == "" && diagnostic.Entry != "" {
		copy.Entry = diagnostic.Entry
		changed = true
	}
	if copy.Class == "" && diagnostic.Class != "" {
		copy.Class = diagnostic.Class
		changed = true
	}
	if copy.Member == "" && diagnostic.Method != "" {
		copy.Member = diagnostic.Method
		changed = true
	}
	if copy.Offset < 0 && diagnostic.Offset >= 0 {
		copy.Offset = diagnostic.Offset
		changed = true
	}
	if copy.Op == "" && diagnostic.Op != "" {
		copy.Op = diagnostic.Op
		changed = true
	}
	if !changed {
		return failure, true
	}
	return &copy, true
}

// IsKind reports whether err contains a Failure of kind.
func IsKind(err error, kind FailureKind) bool {
	failure, ok := AsFailure(err)
	return ok && failure.Kind == kind
}

// IsScope reports whether err contains a Failure of scope.
func IsScope(err error, scope FailureScope) bool {
	failure, ok := AsFailure(err)
	return ok && failure.Scope == scope
}

// Diagnostic identifies a failure within an archive entry and DEX entity.
type Diagnostic struct {
	Entry  string
	Class  string
	Method string
	Offset int64
	Op     string
	Err    error
}

func (d *Diagnostic) Error() string {
	if d == nil {
		return "dex: <nil>"
	}
	var b strings.Builder
	b.WriteString("dex")
	if d.Entry != "" {
		fmt.Fprintf(&b, " entry=%q", d.Entry)
	}
	if d.Class != "" {
		fmt.Fprintf(&b, " class=%q", d.Class)
	}
	if d.Method != "" {
		fmt.Fprintf(&b, " method=%q", d.Method)
	}
	if d.Offset >= 0 {
		fmt.Fprintf(&b, " offset=0x%x", d.Offset)
	}
	if d.Op != "" {
		fmt.Fprintf(&b, " %s", d.Op)
	}
	if d.Err != nil {
		fmt.Fprintf(&b, ": %v", d.Err)
	}
	return b.String()
}

func (d *Diagnostic) Unwrap() error {
	if d == nil {
		return nil
	}
	return d.Err
}

func New(entry, class, method string, offset int64, op string, err error) error {
	if err == nil {
		return nil
	}
	var existing *Diagnostic
	if errors.As(err, &existing) {
		return err
	}
	return &Diagnostic{Entry: entry, Class: class, Method: method, Offset: offset, Op: op, Err: err}
}

func AsDiagnostic(err error) (*Diagnostic, bool) {
	var diagnostic *Diagnostic
	ok := errors.As(err, &diagnostic)
	return diagnostic, ok
}

// CapabilityError identifies a recognized DEX construct whose structure is
// valid but whose semantics are not implemented by the translator. Only errors
// of this type are eligible for explicit best-effort method stubbing.
type CapabilityError struct {
	Feature string
	Err     error
}

// As exposes the corresponding typed failure while retaining CapabilityError's
// existing concrete type, fields, rendering, and unwrap chain.
func (e *CapabilityError) As(target any) bool {
	failure, ok := target.(**Failure)
	if !ok || failure == nil || e == nil {
		return false
	}
	*failure = &Failure{
		Code:     "dex.unsupported_capability",
		Kind:     FailureKindUnsupported,
		Scope:    FailureScopeMember,
		Stage:    FailureStageTranslate,
		Action:   RecoveryStubMethod,
		Severity: SeverityError,
		Offset:   -1,
		Feature:  e.Feature,
		Err:      e,
	}
	return true
}

func (e *CapabilityError) Error() string {
	if e == nil {
		return "unsupported DEX capability"
	}
	if e.Err == nil {
		return fmt.Sprintf("unsupported DEX capability: %s", e.Feature)
	}
	return e.Err.Error()
}

func (e *CapabilityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Unsupported marks a validated, method-local capability boundary.
func Unsupported(feature string, err error) error {
	if err == nil {
		return nil
	}
	var existing *CapabilityError
	if errors.As(err, &existing) {
		return err
	}
	return &CapabilityError{Feature: feature, Err: err}
}

// IsUnsupported reports whether err is an explicit capability boundary rather
// than malformed input, a resource failure, or an internal error.
func IsUnsupported(err error) bool {
	var unsupported *CapabilityError
	return errors.As(err, &unsupported)
}
