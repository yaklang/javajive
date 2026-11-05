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
