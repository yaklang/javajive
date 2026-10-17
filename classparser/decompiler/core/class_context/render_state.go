package class_context

// SourceRenderState belongs to one method rendering. Text in the original
// program is data, never evidence that reconstruction failed. Context copies
// used while rendering operands share this receipt; independent methods install
// their own receipt and restore their caller's state on return.
type SourceRenderState struct {
	IncompleteValue bool
	MalformedTry    bool
}

func (f *ClassContext) RecordIncompleteValue() {
	if f != nil && f.SourceRendering != nil {
		f.SourceRendering.IncompleteValue = true
	}
}
