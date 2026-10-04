package class_context

// OrderSourceConditionalArms preserves the condition's single evaluation and
// exchanges whole arm bodies only when the owning declaration proves that
// lexical order is necessary to regenerate original binary identities.
func (c *ClassContext) OrderSourceConditionalArms(condition, left, right string) (string, string, string) {
	if c != nil && c.SourceBranchSwap != nil && c.SourceBranchSwap(left, right) {
		return "!(" + condition + ")", right, left
	}
	return condition, left, right
}
