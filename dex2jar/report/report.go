// Package report provides source-aware diagnostics for DEX processing.
package report

import (
	"errors"
	"fmt"
	"strings"
)

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
