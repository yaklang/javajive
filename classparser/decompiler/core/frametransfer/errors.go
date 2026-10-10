package frametransfer

import "fmt"

type KindErr int

const (
	InvalidInput KindErr = iota
	Unsupported
	InconsistentSMT
)

type Error struct {
	Kind KindErr
	PC   uint16
	Op   int
	Msg  string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	label := "invalid_input"
	switch e.Kind {
	case Unsupported:
		label = "unsupported"
	case InconsistentSMT:
		label = "inconsistent_stackmap"
	}
	if e.PC != 0 || e.Op != 0 {
		return fmt.Sprintf("%s: PC %d op %#x: %s", label, e.PC, e.Op, e.Msg)
	}
	return fmt.Sprintf("%s: %s", label, e.Msg)
}

func invalidf(format string, args ...any) error {
	return &Error{Kind: InvalidInput, Msg: fmt.Sprintf(format, args...)}
}

func unsupportedf(format string, args ...any) error {
	return &Error{Kind: Unsupported, Msg: fmt.Sprintf(format, args...)}
}

func smtf(format string, args ...any) error {
	return &Error{Kind: InconsistentSMT, Msg: fmt.Sprintf(format, args...)}
}

func IsInvalid(err error) bool {
	e, ok := err.(*Error)
	return ok && e.Kind == InvalidInput
}

func IsUnsupported(err error) bool {
	e, ok := err.(*Error)
	return ok && e.Kind == Unsupported
}
