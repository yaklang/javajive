package methodir

import "fmt"

type Kind int

const (
	InvalidInput Kind = iota
	Unsupported
)

type Error struct {
	Kind Kind
	PC   uint16
	Op   int
	Msg  string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	label := "invalid_input"
	if e.Kind == Unsupported {
		label = "unsupported"
	}
	if e.PC != 0 || e.Op != 0 {
		return fmt.Sprintf("%s: PC %d op %#x: %s", label, e.PC, e.Op, e.Msg)
	}
	return fmt.Sprintf("%s: %s", label, e.Msg)
}

func errInvalid(pc uint16, op int, msg string) error {
	return &Error{Kind: InvalidInput, PC: pc, Op: op, Msg: msg}
}

func errUnsupported(pc uint16, op int, msg string) error {
	return &Error{Kind: Unsupported, PC: pc, Op: op, Msg: msg}
}
