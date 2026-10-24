package javaclassparser

// Several original factory PCs can reconstruct capture-specific views of one
// implementation. The last cache entry is not necessarily the source writer
// retained by a proved finally view. Select that actual writer's own receipt,
// while the enclosing method's emission frame is active. This does not grant
// another source occurrence or widen the existing shared-static-site domain.
func (c *ClassObjectDumper) selectLambdaWriterReceipt(body *dumpedMethods, receipt *lambdaMethodSourceReceipt) bool {
	if c == nil || body == nil || receipt == nil {
		return false
	}
	frame := c.lambdaMethodRendering
	if frame == nil || frame.body == nil || body.sourceRequest.factory == nil || body.sourceRequest.factory.method != frame.body.member {
		return true // A preview or a separately owned declaration keeps its old role.
	}
	if c.lambdaReceiptForBody(body) != nil {
		return true // Includes the existing fully proved shared-static occurrences.
	}
	current := c.lambdaMethodSourceBodies[body.member]
	valid := !frame.ended && !frame.invalid && frame.request == frame.body.sourceRequest && len(frame.children) <= methodSourceLambdaRequestLimit
	valid = valid && receipt.object == c.obj && receipt.method == body.member && receipt.body == body && receipt.request == body.sourceRequest
	valid = valid && receipt.key == body.sourceCacheKey && c.dumpedMethodsSet[receipt.key] == body && body.sourceDumper == c && body.sourceObject == c.obj
	valid = valid && receipt.identity != nil && current != nil && current.identity == receipt.identity && current.object == c.obj && current.method == body.member && len(current.shared) == 0
	valid = valid && receipt.request.factory.closed() && receipt.request.factory.sealed
	if valid {
		a, b := receipt.request.factory, current.request.factory
		valid = receipt.request.role == methodSourceLambdaBody && current.request.role == methodSourceLambdaBody && receipt.request.captureCount == current.request.captureCount && b.closed() && b.sealed && a.object == b.object && a.method == b.method && a.code == b.code
	}
	if valid {
		valid = nativeProofWork(c.Work, int64(len(frame.children))+1) && (c.Work == nil || c.Work.CheckAlloc(128) == nil) && c.lambdaMethodSourcePacketClosed(body.member, body, c.Work) && c.lambdaMethodSourcePacketClosed(current.method, current.body, c.Work)
	}
	if valid {
		for _, occurrence := range frame.children {
			if occurrence.body == nil || occurrence.body.member == body.member {
				valid = false // Two captured arrows cannot borrow one implementation.
				break
			}
		}
	}
	if !valid {
		frame.invalid = true
		return false
	}
	c.lambdaMethodSourceBodies[body.member] = receipt
	return true
}
