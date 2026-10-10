package class_context

// InvocationReceiverSource never changes executable tokens. The optional
// original-family proof moves only its compiler registration comments.
func (c *ClassContext) InvocationReceiverSource(source string) (string, string) {
	if c != nil && c.SourceInvocationReceiver != nil {
		return c.SourceInvocationReceiver(source)
	}
	return source, ""
}
