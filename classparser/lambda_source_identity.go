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
	if c.options.SourceCompiler != NativeJavac8 {
		return nil, true
	}
	required := false
	for _, receipt := range c.lambdaMethodSourceBodies {
		if receipt != nil && receipt.body != nil && receipt.body.lambdaParameterBinding != nil {
			required = true
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
		// ordinal apparently correct. Private static synthetic declarations
		// still need actual consumed body receipts, not just a matching prefix.
		if method.AccessFlags == 0x100a && c.lambdaMethodSourceBodies[method] == nil {
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
	byArrow := map[int]nativeLambdaSourceIdentityEvent{}
	for method, receipt := range c.lambdaMethodSourceBodies {
		if receipt == nil || receipt.object != c.obj || receipt.method != method || receipt.body == nil || !c.lambdaMethodSourcePacketClosed(method, receipt.body, c.Work) {
			return nil, false
		}
		factory := receipt.request.factory
		if !factory.closed() || factory.object != c.obj {
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
		start := strings.Index(text, parent.code)
		position, known := nativeLambdaSourcePacketArrow(parent.code, receipt.body.code, c.Work, charge)
		if start < 0 || !known || strings.Index(text[start+len(parent.code):], parent.code) >= 0 {
			return nil, false
		}
		position += start
		if _, duplicate := byArrow[position]; duplicate {
			return nil, false
		}
		factoryName, named := sourceBridgeUTF8(c.obj, factory.method.NameIndex)
		implementation, known := sourceBridgeUTF8(c.obj, receipt.method.NameIndex)
		suffix, prefixed := strings.CutPrefix(implementation, "lambda$"+factoryName+"$")
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
