package values

import "github.com/yaklang/javajive/classparser/decompiler/core/class_context"

// IncompleteSourceValue retains readable debug output while recording the
// actual renderer failure. A Java string with the same contents has no receipt.
func IncompleteSourceValue(ctx *class_context.ClassContext) string {
	ctx.RecordIncompleteValue()
	return EmptySlotValuePlaceholder
}
