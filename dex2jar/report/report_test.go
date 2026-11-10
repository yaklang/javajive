package report

import (
	"errors"
	"fmt"
	"testing"
)

func TestDiagnosticStableCategoriesAndRendering(t *testing.T) {
	cause := errors.New("bad operand")
	tests := []struct {
		name string
		d    *Diagnostic
		want string
	}{
		{"nil diagnostic", nil, "dex: <nil>"},
		{"category only", &Diagnostic{Offset: -1, Op: "header", Err: cause}, "dex header: bad operand"},
		{"input category", &Diagnostic{Entry: "classes.dex", Offset: 0, Op: "input", Err: cause}, `dex entry="classes.dex" offset=0x0 input: bad operand`},
		{"opcode category", &Diagnostic{Entry: "classes2.dex", Class: "Lpkg/C;", Method: "run", Offset: 42, Op: "invoke-static(0x71)", Err: cause}, `dex entry="classes2.dex" class="Lpkg/C;" method="run" offset=0x2a invoke-static(0x71): bad operand`},
		{"context without category", &Diagnostic{Class: "LC;", Offset: -1, Err: cause}, `dex class="LC;": bad operand`},
		{"context without cause", &Diagnostic{Method: "m", Offset: -1, Op: "class"}, `dex method="m" class`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.d.Error(); got != test.want {
				t.Fatalf("Error() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFailureEnumValues(t *testing.T) {
	tests := []struct {
		name string
		got  fmt.Stringer
		want string
	}{
		{"kind invalid input", stringValue(FailureKindInvalidInput), "invalid_input"},
		{"kind unsupported", stringValue(FailureKindUnsupported), "unsupported"},
		{"kind resource limit", stringValue(FailureKindResourceLimit), "resource_limit"},
		{"kind canceled", stringValue(FailureKindCanceled), "canceled"},
		{"kind internal", stringValue(FailureKindInternal), "internal"},
		{"scope request", stringValue(FailureScopeRequest), "request"},
		{"scope input", stringValue(FailureScopeInput), "input"},
		{"scope container", stringValue(FailureScopeContainer), "container"},
		{"scope entry", stringValue(FailureScopeEntry), "entry"},
		{"scope class", stringValue(FailureScopeClass), "class"},
		{"scope member", stringValue(FailureScopeMember), "member"},
		{"scope instruction", stringValue(FailureScopeInstruction), "instruction"},
		{"stage identify", stringValue(FailureStageIdentify), "identify"},
		{"stage read", stringValue(FailureStageRead), "read"},
		{"stage parse", stringValue(FailureStageParse), "parse"},
		{"stage validate", stringValue(FailureStageValidate), "validate"},
		{"stage analyze", stringValue(FailureStageAnalyze), "analyze"},
		{"stage translate", stringValue(FailureStageTranslate), "translate"},
		{"stage lower", stringValue(FailureStageLower), "lower"},
		{"stage emit", stringValue(FailureStageEmit), "emit"},
		{"stage write", stringValue(FailureStageWrite), "write"},
		{"recovery none", stringValue(RecoveryActionNone), "none"},
		{"recovery abort", stringValue(RecoveryActionAbort), "abort"},
		{"recovery retry", stringValue(RecoveryActionRetry), "retry"},
		{"recovery skip entry", stringValue(RecoveryActionSkipEntry), "skip_entry"},
		{"recovery skip class", stringValue(RecoveryActionSkipClass), "skip_class"},
		{"recovery stub method", stringValue(RecoveryActionStubMethod), "stub_method"},
		{"recovery stub class", stringValue(RecoveryActionStubClass), "stub_class"},
		{"recovery stub class minimal", stringValue(RecoveryActionStubClassMinimal), "stub_class_minimal"},
		{"recovery drop attribute", stringValue(RecoveryActionDropAttribute), "drop_attribute"},
		{"severity info", stringValue(SeverityInfo), "info"},
		{"severity warning", stringValue(SeverityWarning), "warning"},
		{"severity error", stringValue(SeverityError), "error"},
		{"severity fatal", stringValue(SeverityFatal), "fatal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.got.String(); got != test.want {
				t.Fatalf("value = %q, want %q", got, test.want)
			}
		})
	}
}

type stringValue string

func (s stringValue) String() string { return string(s) }

func TestFailureStableRenderingAndUnwrap(t *testing.T) {
	cause := errors.New("bad operand")
	failure := &Failure{
		Code:       "dex.ir.bad_operand",
		Kind:       FailureKindInvalidInput,
		Scope:      FailureScopeInstruction,
		Stage:      FailureStageAnalyze,
		Action:     RecoveryAbort,
		Severity:   SeverityError,
		Entry:      "base/classes.dex",
		Class:      "Lpkg/C;",
		Member:     "run",
		Descriptor: "(I)V",
		Offset:     42,
		Op:         "invoke-static(0x71)",
		Feature:    "quoted \"feature\"",
		Counter:    "analysis_updates",
		Used:       101,
		Limit:      100,
		Err:        cause,
	}
	want := `dex failure code="dex.ir.bad_operand" kind="invalid_input" scope="instruction" stage="analyze" action="abort" severity="error" entry="base/classes.dex" class="Lpkg/C;" member="run" descriptor="(I)V" offset=0x2a op="invoke-static(0x71)" feature="quoted \"feature\"" counter="analysis_updates" used=101 limit=100: bad operand`
	if got := failure.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if got := failure.Error(); got != want {
		t.Fatalf("second Error() = %q, want deterministic %q", got, want)
	}
	if failure.Unwrap() != cause || !errors.Is(failure, cause) {
		t.Fatalf("failure lost cause: %#v", failure.Unwrap())
	}

	if got := (&Failure{Offset: -1}).Error(); got != "dex failure" {
		t.Fatalf("empty failure = %q", got)
	}
	if got := (&Failure{Offset: 0, Used: 1}).Error(); got != "dex failure offset=0x0 used=1 limit=0" {
		t.Fatalf("sparse failure = %q", got)
	}
	var nilFailure *Failure
	if nilFailure.Error() != "dex failure: <nil>" || nilFailure.Unwrap() != nil {
		t.Fatalf("nil Failure = %q / %v", nilFailure.Error(), nilFailure.Unwrap())
	}
}

func TestFailureConstructionClassificationAndWrapping(t *testing.T) {
	cause := errors.New("root cause")
	if got := NewFailure("code", FailureKindInvalidInput, FailureScopeEntry, FailureStageParse, nil); got != nil {
		t.Fatalf("NewFailure(..., nil) = %v", got)
	}

	err := NewFailure("dex.parse.truncated", FailureKindInvalidInput, FailureScopeEntry, FailureStageParse, cause)
	failure, ok := AsFailure(err)
	if !ok {
		t.Fatalf("AsFailure(%v) failed", err)
	}
	if failure.Code != "dex.parse.truncated" || failure.Kind != FailureKindInvalidInput || failure.Scope != FailureScopeEntry || failure.Stage != FailureStageParse || failure.Offset != -1 {
		t.Fatalf("failure = %#v", failure)
	}
	if !errors.Is(err, cause) || !IsKind(err, FailureKindInvalidInput) || !IsScope(err, FailureScopeEntry) {
		t.Fatalf("classification lost through direct error: %v", err)
	}
	if IsKind(err, FailureKindInternal) || IsScope(err, FailureScopeMember) {
		t.Fatalf("failure matched the wrong classification: %#v", failure)
	}

	wrapped := fmt.Errorf("outer: %w", err)
	got, ok := AsFailure(wrapped)
	if !ok || got != failure || !IsKind(wrapped, FailureKindInvalidInput) || !IsScope(wrapped, FailureScopeEntry) {
		t.Fatalf("classification through wrapper = %#v, %v", got, ok)
	}
	if again := NewFailure("other", FailureKindInternal, FailureScopeRequest, FailureStageEmit, wrapped); again != wrapped {
		t.Fatalf("NewFailure double-wrapped an existing failure: %v", again)
	}

	if got, ok := AsFailure(cause); ok || got != nil {
		t.Fatalf("AsFailure(non-failure) = %#v, %v", got, ok)
	}
	if got, ok := AsFailure(nil); ok || got != nil || IsKind(nil, FailureKindInvalidInput) || IsScope(nil, FailureScopeEntry) {
		t.Fatalf("nil classification = %#v, %v", got, ok)
	}
	var nilFailure *Failure
	var typedNil error = nilFailure
	if got, ok := AsFailure(typedNil); ok || got != nil || IsKind(typedNil, FailureKindInvalidInput) || IsScope(typedNil, FailureScopeEntry) {
		t.Fatalf("typed nil classification = %#v, %v", got, ok)
	}
}

func TestDiagnosticProjectsSourceContextIntoFailure(t *testing.T) {
	cause := errors.New("bad instruction")
	original := &Failure{
		Code:     "dex.ir.unsupported_opcode",
		Kind:     FailureKindUnsupported,
		Scope:    FailureScopeMember,
		Stage:    FailureStageTranslate,
		Action:   RecoveryStubMethod,
		Severity: SeverityError,
		Offset:   -1,
		Feature:  "odex opcode",
		Err:      cause,
	}
	err := New("classes2.dex", "Lpkg/C;", "run", 42, "invoke-virtual-quick(0xf8)", original)

	diagnostic, ok := AsDiagnostic(err)
	if !ok || diagnostic.Entry != "classes2.dex" || diagnostic.Class != "Lpkg/C;" || diagnostic.Method != "run" || diagnostic.Offset != 42 || diagnostic.Op != "invoke-virtual-quick(0xf8)" {
		t.Fatalf("diagnostic context = %#v, %v", diagnostic, ok)
	}
	failure, ok := AsFailure(err)
	if !ok {
		t.Fatalf("AsFailure(%v) failed", err)
	}
	if failure == original || failure.Entry != diagnostic.Entry || failure.Class != diagnostic.Class || failure.Member != diagnostic.Method || failure.Offset != diagnostic.Offset || failure.Op != diagnostic.Op {
		t.Fatalf("projected failure = %#v", failure)
	}
	if original.Entry != "" || original.Class != "" || original.Member != "" || original.Offset != -1 || original.Op != "" {
		t.Fatalf("AsFailure mutated wrapped failure: %#v", original)
	}
	if !errors.Is(err, cause) || !IsKind(err, FailureKindUnsupported) || !IsScope(err, FailureScopeMember) {
		t.Fatalf("diagnostic lost typed classification: %v", err)
	}

	explicit := &Failure{Kind: FailureKindInternal, Scope: FailureScopeClass, Entry: "explicit.dex", Member: "explicit", Offset: 7, Err: cause}
	projected, ok := AsFailure(New("diagnostic.dex", "Ldiag;", "diag", 9, "diag-op", explicit))
	if !ok || projected.Entry != "explicit.dex" || projected.Class != "Ldiag;" || projected.Member != "explicit" || projected.Offset != 7 || projected.Op != "diag-op" {
		t.Fatalf("partial context projection = %#v, %v", projected, ok)
	}
	complete := &Failure{Kind: FailureKindInternal, Scope: FailureScopeClass, Entry: "explicit.dex", Class: "Lexplicit;", Member: "explicit", Offset: 7, Op: "explicit-op", Err: cause}
	unchanged, ok := AsFailure(New("diagnostic.dex", "Ldiag;", "diag", 9, "diag-op", complete))
	if !ok || unchanged != complete {
		t.Fatalf("complete context projection = %#v, %v", unchanged, ok)
	}
}

func TestCapabilityErrorContract(t *testing.T) {
	var nilCapability *CapabilityError
	if nilCapability.Error() != "unsupported DEX capability" || nilCapability.Unwrap() != nil {
		t.Fatalf("nil CapabilityError = %q / %v", nilCapability.Error(), nilCapability.Unwrap())
	}
	if Unsupported("feature", nil) != nil {
		t.Fatal("Unsupported(feature, nil) did not return nil")
	}

	cause := errors.New("root cause")
	err := Unsupported("feature", cause)
	if !IsUnsupported(err) || err.Error() != "root cause" {
		t.Fatalf("Unsupported error = %v", err)
	}
	var capability *CapabilityError
	if !errors.As(err, &capability) || capability.Feature != "feature" || capability.Unwrap() != cause {
		t.Fatalf("capability = %#v", capability)
	}
	failure, ok := AsFailure(err)
	if !ok || failure.Code != "dex.unsupported_capability" || failure.Kind != FailureKindUnsupported || failure.Scope != FailureScopeMember || failure.Stage != FailureStageTranslate || failure.Action != RecoveryStubMethod || failure.Severity != SeverityError || failure.Offset != -1 || failure.Feature != "feature" || failure.Unwrap() != capability {
		t.Fatalf("capability failure = %#v, %v", failure, ok)
	}
	if !IsKind(err, FailureKindUnsupported) || !IsScope(err, FailureScopeMember) {
		t.Fatalf("capability was not classified: %#v", failure)
	}
	if again := Unsupported("other", err); again != err {
		t.Fatalf("Unsupported rewrapped an existing capability: %v", again)
	}
	wrapped := fmt.Errorf("outer: %w", err)
	if !IsUnsupported(wrapped) || !IsKind(wrapped, FailureKindUnsupported) || !IsScope(wrapped, FailureScopeMember) {
		t.Fatal("wrapped capability lost compatibility or typed classification")
	}
	if again := NewFailure("other", FailureKindInternal, FailureScopeRequest, FailureStageEmit, wrapped); again != wrapped {
		t.Fatalf("NewFailure rewrapped a legacy capability: %v", again)
	}
	if IsUnsupported(cause) {
		t.Fatal("plain error was classified as unsupported")
	}
	if got := (&CapabilityError{Feature: "feature"}).Error(); got != "unsupported DEX capability: feature" {
		t.Fatalf("feature-only error = %q", got)
	}
	if got := (&CapabilityError{}).Error(); got != "unsupported DEX capability: " {
		t.Fatalf("empty capability error = %q", got)
	}
	var typedNil error = nilCapability
	if failure, ok := AsFailure(typedNil); ok || failure != nil || IsKind(typedNil, FailureKindUnsupported) || IsScope(typedNil, FailureScopeMember) {
		t.Fatalf("typed nil capability classification = %#v, %v", failure, ok)
	}
}

func TestDiagnosticWrappingContract(t *testing.T) {
	cause := errors.New("root cause")
	if got := New("entry", "class", "method", 1, "opcode", nil); got != nil {
		t.Fatalf("New(..., nil) = %v", got)
	}

	err := New("entry", "class", "method", 1, "opcode", cause)
	diagnostic, ok := AsDiagnostic(err)
	if !ok {
		t.Fatalf("AsDiagnostic(%v) failed", err)
	}
	if diagnostic.Entry != "entry" || diagnostic.Class != "class" || diagnostic.Method != "method" || diagnostic.Offset != 1 || diagnostic.Op != "opcode" {
		t.Fatalf("diagnostic context = %#v", diagnostic)
	}
	if !errors.Is(err, cause) || diagnostic.Unwrap() != cause {
		t.Fatalf("diagnostic lost cause: %v", err)
	}

	wrapped := fmt.Errorf("outer: %w", err)
	got, ok := AsDiagnostic(wrapped)
	if !ok || got != diagnostic {
		t.Fatalf("AsDiagnostic through wrapper = %#v, %v", got, ok)
	}
	if rewrapped := New("other", "other", "other", 99, "other", wrapped); rewrapped != wrapped {
		t.Fatalf("New double-wrapped diagnostic: %v", rewrapped)
	}

	var nilDiagnostic *Diagnostic
	if nilDiagnostic.Unwrap() != nil {
		t.Fatal("nil Diagnostic.Unwrap returned a cause")
	}
	if got, ok := AsDiagnostic(cause); ok || got != nil {
		t.Fatalf("AsDiagnostic(non-diagnostic) = %#v, %v", got, ok)
	}
}
