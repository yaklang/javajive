package statements

import "github.com/yaklang/javajive/classparser/decompiler/core/class_context"

// Source transfers carry no Java value operands. Keep that fact separate from
// the CFG target certificate used by protected-region and monitor proofs. The
// renderer is sealed to a keyword and a compiler identifier; a text closure
// cannot introduce hidden local-variable references into this source leaf.
func NewSourceTransferStatement(kind, label string) *CustomStatement {
	if !validSourceTransfer(kind, label) {
		return nil
	}
	return &CustomStatement{sourceTransferKind: kind, LoopTargetLabel: label}
}
func validSourceTransfer(kind, label string) bool {
	return (kind == "break" || kind == "continue") && (label == "" || class_context.SafeIdentifier(label) == label)
}
func (s *CustomStatement) SourceTransferOnly() bool {
	return s != nil && s.ThrownValue == nil && validSourceTransfer(s.sourceTransferKind, s.LoopTargetLabel)
}

// An invalidated structured leaf must not fall back to an opaque text or Name
// convention. Its source-binding certificate has to be revalidated as a whole.
func (s *CustomStatement) HasSourceTransfer() bool {
	return s != nil && s.sourceTransferKind != ""
}

// Retarget only an existing structured source transfer. Qualifying an opaque
// CustomStatement never manufactures this binding capability.
func (s *CustomStatement) RetargetSourceTransfer(kind, label string) bool {
	if s == nil || s.sourceTransferKind == "" || !validSourceTransfer(kind, label) {
		return false
	}
	s.sourceTransferKind = kind
	s.LoopTargetLabel = label
	return true
}
