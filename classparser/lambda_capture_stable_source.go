package javaclassparser

// Only actual completed factory occurrences with closed original local-name
// receipts can protect a capture from the legacy text alias pass. Coordinates
// belong to this exact input source. A name elsewhere, a same-spelled sibling,
// a preview render or an unproved original value supplies no protection.
func (c *ClassObjectDumper) completedLambdaStableSourceBindings(source string, methods []*dumpedMethods) map[int]map[string]bool {
	if c == nil || c.obj == nil || len(source) > 16<<20 || len(methods) > 4096 || len(c.lambdaMethodSourceBodies) == 0 || len(c.lambdaMethodSourceBodies) > 4096 || !nativeProofWork(c.Work, int64(len(c.lambdaMethodSourceBodies))) {
		return nil
	}
	// Modern ordinary arrows have no native parameter-name bindings. Classes
	// without any captured binding need no protection map at all, regardless of
	// source size or the number of completed (but factory-free) method frames.
	// Charge the whole inventory before candidate selection so map iteration
	// order cannot choose whether a request exhausts its budget.
	candidate := false
	for _, receipt := range c.lambdaMethodSourceBodies {
		if receipt != nil && receipt.body != nil && receipt.body.lambdaParameterBinding != nil {
			binding := receipt.body.lambdaParameterBinding
			candidate = candidate || binding.metadata != nil && len(binding.captured) != 0
		}
	}
	if !candidate || !nativeProofWork(c.Work, int64(len(source))+1) {
		return nil
	}
	arrows, known := c.lambdaOwnImplementationTokens(source, false)
	if !known || len(arrows) > 4096 || c.Work != nil && c.Work.CheckAlloc(int64(len(arrows))*128+256) != nil {
		return nil
	}
	lines := map[int]int{}
	lineAt := map[int]int{}
	line, next := 0, 0
	for at := range source {
		if next < len(arrows) && at == arrows[next] {
			lines[line]++
			lineAt[at] = line
			next++
		}
		if source[at] == '\n' {
			line++
		}
	}
	if next != len(arrows) {
		return nil
	}
	scanned := int64(0)
	charge := func(n int64) bool {
		if n < 0 || n > (128<<20)-scanned {
			return false
		}
		scanned += n
		return nativeProofWork(c.Work, n)
	}
	stable := map[int]map[string]bool{}
	entries := 0
	for _, parent := range methods {
		if parent == nil || parent.sourceDumper != c || parent.sourceObject != c.obj || c.dumpedMethodsSet[parent.sourceCacheKey] != parent || !parent.lambdaEmission.closed() || len(parent.lambdaEmission.children) == 0 || !charge(int64(len(source))+int64(len(parent.code))+1) {
			continue
		}
		declaration, start, known := c.lambdaParentDeclaration(source, parent)
		if !known {
			continue
		}
		for _, event := range parent.lambdaEmission.children {
			child := event.body
			if child == nil || child.lambdaParameterBinding == nil {
				continue
			}
			binding := child.lambdaParameterBinding
			receipt := c.lambdaMethodSourceBodies[child.member]
			if binding.metadata == nil || len(binding.captured) == 0 || !binding.capturedNamesClosed(c.Work) || !c.lambdaMethodSourcePacketClosed(child.member, child, c.Work) {
				continue
			}
			if _, known := c.lambdaCompletedCapturePacket(parent, receipt, charge); !known {
				continue
			}
			at, known := c.lambdaEmissionPosition(parent, child, declaration, charge)
			if !known || start+at < 0 || start+at >= len(source) {
				continue
			}
			line, originalArrow := lineAt[start+at]
			if !originalArrow || lines[line] != 1 {
				continue
			}
			for i, operand := range binding.metadata.captures {
				if operand != nil && operand.local != nil && nativeProofWork(c.Work, 1) {
					entries++
					if entries > 4096*64 || c.Work != nil && c.Work.CheckAlloc(int64(entries)*64+int64(len(arrows))*128+256) != nil {
						return nil
					}
					if stable[line] == nil {
						stable[line] = map[string]bool{}
					}
					stable[line][binding.metadata.names[i]] = true
				}
			}
		}
	}
	return stable
}
