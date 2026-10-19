package javaclassparser

import (
	"strconv"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Native javac8 numbers non-serializable lambda implementations in emitted
// source order, independently in each class. Parameter names alone therefore
// cannot certify regeneration of the method carrying those parameters. Reopen
// the actual body/factory receipts against the final source, after recovery and
// member layout. This query grants no lexical, capture or invocation permission.
type nativeLambdaSourceIdentityEvent struct {
	ordinal int
	parent  *dumpedMethods
}

func (c *ClassObjectDumper) native8LambdaSourceIdentitiesClosed(source string, members []string, methods []*dumpedMethods) bool {
	events, known := c.native8LambdaSourceIdentityEvents(source, members, methods)
	return known && nativeLambdaSourceIdentityOrderClosed(events)
}

func nativeLambdaSourceIdentityOrderClosed(events []nativeLambdaSourceIdentityEvent) bool {
	for ordinal, event := range events {
		if event.ordinal != ordinal {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) native8LambdaSourceIdentityEvents(source string, members []string, methods []*dumpedMethods) ([]nativeLambdaSourceIdentityEvent, bool) {
	if c == nil {
		return nil, false
	}
	// A failed/mutated receipt must not erase the requirement it carried.
	// This latch records only an attempted original initializer factory; every
	// positive body, ordinal and source-occurrence proof is still reopened.
	required := c.lambdaInitializerFactorySeen && (c.options.SourceCompiler == NativeJavac8 || c.options.SourceCompiler == ModernJavac)
	for _, receipt := range c.lambdaMethodSourceBodies {
		if receipt != nil && receipt.body != nil && c.options.SourceCompiler == NativeJavac8 && receipt.body.lambdaParameterBinding != nil {
			required = true
		}
		// Modern javac does not emit the native implementation parameter
		// tables, but its initializer factories still share the class-wide
		// implementation ordinal. Observe the actual factory, never a name
		// prefix in a synthetic declaration which may have been dropped.
		if receipt != nil && receipt.request.factory.closed() && c.options.SourceCompiler == ModernJavac {
			name, known := sourceBridgeUTF8(c.obj, receipt.request.factory.method.NameIndex)
			required = required || known && name == "<clinit>"
		}
	}
	if !required {
		return nil, true
	}
	if c.obj == nil || len(c.obj.Methods) > 4096 || c.options.TargetSourceVersion != 8 || len(source) > 16<<20 || len(members) > 4096 || len(methods) > 4096 || len(c.lambdaMethodSourceBodies) > 4096 || !nativeProofWork(c.Work, int64(len(source))+int64(len(members))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(source))*2+4096*128) != nil {
		return nil, false
	}
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		// A dropped trailing implementation would leave every remaining
		// ordinal apparently correct. Private synthetic declarations
		// still need actual consumed body receipts, not just a matching prefix.
		if (method.AccessFlags == 0x100a || method.AccessFlags == 0x1002) && c.lambdaMethodSourceBodies[method] == nil {
			return nil, false
		}
	}
	// Bound aggregate searches even for internal callers without a request
	// budget. Per-packet bounds alone allow a quadratic class-wide scan.
	scanned := int64(len(source))
	charge := func(bytes int64) bool {
		if bytes < 0 || bytes > (128<<20)-scanned {
			return false
		}
		scanned += bytes
		return nativeProofWork(c.Work, bytes)
	}
	// Completed member declarations have their own compiler counters. Their
	// actual emitted packets, never dollar-name guesses, identify these spans.
	own := []byte(source)
	for _, member := range members {
		if member == "" || !charge(int64(len(source))+int64(len(member))) {
			return nil, false
		}
		start := strings.Index(source, member)
		if start < 0 || strings.Index(source[start+len(member):], member) >= 0 {
			return nil, false
		}
		for i := start; i < start+len(member); i++ {
			if own[i] != source[i] {
				return nil, false
			}
			if own[i] != '\n' && own[i] != '\r' {
				own[i] = ' '
			}
		}
	}
	text := string(own)
	arrows, known := nativeLambdaSourceArrows(text, c.Work)
	if !known || len(arrows) == 0 {
		return nil, false
	}
	for _, receipt := range c.lambdaMethodSourceBodies {
		if receipt != nil && receipt.request.factory != nil && receipt.request.factory.method != nil && (receipt.request.factory.method.AccessFlags == 0x100a || receipt.request.factory.method.AccessFlags == 0x1002) {
			return c.native8NestedLambdaSourceIdentityEvents(text, arrows, methods, charge)
		}
	}
	byArrow := map[int]nativeLambdaSourceIdentityEvent{}
	for method, receipt := range c.lambdaMethodSourceBodies {
		if receipt == nil || receipt.object != c.obj || receipt.method != method || receipt.body == nil || !c.lambdaMethodSourcePacketClosed(method, receipt.body, c.Work) {
			return nil, false
		}
		factory := receipt.request.factory
		if !factory.closed() || !factory.sealed || factory.object != c.obj {
			return nil, false
		}
		var parent *dumpedMethods
		for _, candidate := range methods {
			if !nativeProofWork(c.Work, 1) {
				return nil, false
			}
			if candidate != nil && candidate.member == factory.method {
				if parent != nil {
					return nil, false
				}
				parent = candidate
			}
		}
		if parent == nil || parent.sourceDumper != c || parent.sourceObject != c.obj || parent.sourceRequest.role != methodSourceDeclaration || parent.sourceCacheKey == "" || c.dumpedMethodsSet[parent.sourceCacheKey] != parent || parent.sourceReconstructionFailed() || !c.methodParameterNameCacheClosed(parent.parameterNameStamp) || parent.code == "" || receipt.body.code == "" || !charge(int64(len(text))+int64(len(parent.code))+int64(len(receipt.body.code))) {
			return nil, false
		}
		packet := receipt.body.code
		if binding := receipt.body.lambdaParameterBinding; binding != nil && binding.metadata != nil && binding.metadata.factoryCaptureCount() != 0 {
			if !binding.capturedNamesClosed(c.Work) || receipt.renderedUnknown || receipt.renderedTemplate != packet {
				return nil, false
			}
			if parent.lambdaEmission != nil {
				var closed bool
				packet, closed = c.lambdaCompletedCapturePacket(parent, receipt, charge)
				if !closed {
					return nil, false
				}
			} else {
				packet = ""
				for _, rendered := range receipt.renderedPackets {
					if !charge(int64(len(source))) {
						return nil, false
					}
					if strings.Contains(text, rendered) {
						if packet != "" {
							return nil, false
						}
						packet = rendered
					}
				}
				if packet == "" {
					return nil, false
				}
			}
		}
		declaration, start, owned := c.lambdaParentDeclaration(text, parent)
		if !owned {
			return nil, false
		}
		component, named := lambdaFactoryNameComponent(factory)
		if !named || (component == "static" || c.options.SourceCompiler == ModernJavac) && parent.lambdaEmission == nil {
			return nil, false
		}
		position, known := 0, false
		if parent.lambdaEmission != nil {
			position, known = c.lambdaEmissionPosition(parent, receipt.body, declaration, charge)
		} else {
			position, known = nativeLambdaSourcePacketArrow(declaration, packet, c.Work, charge)
		}
		if !known {
			return nil, false
		}
		position += start
		if _, duplicate := byArrow[position]; duplicate {
			return nil, false
		}
		implementation, known := sourceBridgeUTF8(c.obj, receipt.method.NameIndex)
		suffix, prefixed := strings.CutPrefix(implementation, "lambda$"+component+"$")
		ordinal, err := strconv.Atoi(suffix)
		if !named || !known || !prefixed || err != nil || ordinal < 0 || suffix != strconv.Itoa(ordinal) {
			return nil, false
		}
		byArrow[position] = nativeLambdaSourceIdentityEvent{ordinal: ordinal, parent: parent}
	}
	if len(byArrow) != len(arrows) {
		return nil, false
	}
	ordered := make([]nativeLambdaSourceIdentityEvent, 0, len(arrows))
	for _, position := range arrows {
		event, known := byArrow[position]
		if !known {
			return nil, false
		}
		ordered = append(ordered, event)
	}
	return ordered, true
}

// Capture substitution is observed by the existing completed writer. Reopen
// that receipt before the occurrence graph uses its source position. A cached
// template with capture markers is never a final emitted packet.
func (c *ClassObjectDumper) native8LambdaRenderedIdentityPacketClosed(receipt *lambdaMethodSourceReceipt, source string, charge func(int64) bool, parent ...*dumpedMethods) bool {
	if receipt == nil || receipt.body == nil || charge == nil {
		return false
	}
	binding := receipt.body.lambdaParameterBinding
	if binding == nil || binding.metadata == nil || binding.metadata.factoryCaptureCount() == 0 {
		return true
	}
	if !binding.capturedNamesClosed(c.Work) || receipt.renderedUnknown || receipt.renderedTemplate != receipt.body.code {
		return false
	}
	if len(parent) == 1 {
		_, closed := c.lambdaCompletedCapturePacket(parent[0], receipt, charge)
		return closed
	}
	if len(parent) != 0 {
		return false
	}
	matches := 0
	for _, packet := range receipt.renderedPackets {
		if !charge(int64(len(source))) {
			return false
		}
		if strings.Contains(source, packet) {
			matches++
		}
	}
	return matches == 1
}

// Locate an already closed body packet only at actual lexical arrows. A formal
// name can legitimately recur in several lambdas, so its header is not a unique
// locator. Prefer the complete sealed packet when it identifies one occurrence.
// A consumer may independently adapt the return expression to its own binder;
// in that case only a unique sealed header can locate the event. Neither text
// comparison grants body, factory or binding permission to its caller.
func nativeLambdaSourcePacketArrow(source, packet string, work *workbudget.Budget, charge func(int64) bool) (int, bool) {
	local, known := nativeLambdaSourceArrows(packet, work)
	// Nested inline scopes require their own registration evidence.
	if !known || len(local) != 1 || charge == nil {
		return 0, false
	}
	arrows, known := nativeLambdaSourceArrows(source, work)
	if !known {
		return 0, false
	}
	header := packet[:local[0]+2]
	headers, packets, headerPosition, packetPosition := 0, 0, 0, 0
	for _, arrow := range arrows {
		start := arrow + 2 - len(header)
		if start < 0 {
			continue
		}
		if !charge(int64(len(header))) {
			return 0, false
		}
		if source[start:arrow+2] != header {
			continue
		}
		headers++
		headerPosition = arrow
		if len(packet) > len(source)-start {
			continue
		}
		if !charge(int64(len(packet))) {
			return 0, false
		}
		if source[start:start+len(packet)] == packet {
			packets++
			packetPosition = arrow
		}
	}
	if packets == 1 {
		return packetPosition, true
	}
	return headerPosition, packets == 0 && headers == 1
}

// Source spelling only locates already certified body packets. It never
// identifies an original method or grants permission to reconstruct a body.
func nativeLambdaSourceArrows(source string, work *workbudget.Budget) ([]int, bool) {
	if len(source) > 16<<20 || !nativeProofWork(work, int64(len(source))+1) || work != nil && work.CheckAlloc(4096*8) != nil {
		return nil, false
	}
	var positions []int
	state, depth := scanNormal, 0
	for i := 0; i < len(source); i++ {
		state = scanAdvance(source, &i, state, &depth)
		if state != scanNormal || i >= len(source) {
			continue
		}
		if source[i] == '-' && i+1 < len(source) && source[i+1] == '>' {
			if len(positions) == 4096 {
				return nil, false
			}
			positions = append(positions, i)
			i++
		}
	}
	return positions, state == scanNormal || state == scanLineComment
}
