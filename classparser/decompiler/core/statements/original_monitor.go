package statements

import "github.com/yaklang/javajive/classparser/decompiler/core/values"

// These witnesses are emitted only after immutable CFG reaching-definition and
// held-state proofs. Public source edits cannot retain a certificate while
// changing the operation, receiver or original acquisition identity.
func NewOriginalMonitorStatement(kind string, value values.JavaValue, pc, owner int) *MiddleStatement {
	s := NewMiddleStatement(kind, nil)
	if kind == "monitor_enter" {
		s.Data = value
	}
	if (kind != "monitor_enter" && kind != "monitor_exit") || pc < 0 || owner < 0 || (kind == "monitor_enter" && (pc != owner || value == nil)) {
		return s
	}
	s.monitorKind = kind
	s.monitorPC = pc
	s.monitorOwner = owner
	s.monitorValue = value
	s.monitorKnown = true
	return s
}
func (s *MiddleStatement) OriginalMonitor() (pc, owner int, known bool) {
	if s == nil || !s.monitorKnown || s.Flag != s.monitorKind {
		return 0, 0, false
	}
	if s.Flag == "monitor_enter" {
		if s.Data != s.monitorValue {
			return 0, 0, false
		}
	} else if s.Data != nil {
		return 0, 0, false
	}
	return s.monitorPC, s.monitorOwner, true
}
func NewSynchronizedStatementFromMonitor(entry *MiddleStatement, body []Statement) *SynchronizedStatement {
	if entry == nil {
		return NewSynchronizedStatement(nil, body)
	}
	value, _ := entry.Data.(values.JavaValue)
	s := NewSynchronizedStatement(value, body)
	pc, owner, known := entry.OriginalMonitor()
	if known && entry.Flag == "monitor_enter" && pc == owner {
		s.monitorPC = pc
		s.monitorValue = value
		s.monitorKnown = true
	}
	return s
}
func (s *SynchronizedStatement) OriginalMonitorEnterPC() (int, bool) {
	if s == nil {
		return 0, false
	}
	return s.monitorPC, s.monitorKnown && s.Argument == s.monitorValue && s.Argument != nil
}
