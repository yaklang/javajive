package javaclassparser

import (
	"slices"
	"strings"
)

// These events record the actual method-body render, not an analysis preview
// or a second render of captured operands. Two equal packets can still denote
// different original factories. Their ordered identities must survive until
// final compiler registration is checked against the actual source arrows.
type lambdaMethodEmission struct {
	body                     *dumpedMethods
	request                  methodSourceRequest
	previous                 *lambdaMethodEmission
	complete, ended, invalid bool
	bytes                    int64
	children                 []lambdaMethodOccurrence
	seal                     *lambdaMethodEmissionSeal
}

type lambdaMethodEmissionSeal struct {
	body     *dumpedMethods
	request  methodSourceRequest
	bytes    int64
	children []lambdaMethodOccurrence
}

type lambdaMethodOccurrence struct {
	body    *dumpedMethods
	request methodSourceRequest
	packet  string
}

func (c *ClassObjectDumper) beginLambdaMethodEmission(body *dumpedMethods) *lambdaMethodEmission {
	if body == nil || c.options.SourceCompiler != NativeJavac8 && c.options.SourceCompiler != ModernJavac {
		return nil
	}
	frame := &lambdaMethodEmission{body: body, request: body.sourceRequest, previous: c.lambdaMethodRendering}
	body.lambdaEmission, c.lambdaMethodRendering = frame, frame
	return frame
}

func (c *ClassObjectDumper) endLambdaMethodEmission(frame *lambdaMethodEmission, complete bool) {
	if frame == nil || frame.ended {
		return
	}
	frame.complete = complete && c.lambdaMethodRendering == frame && !frame.invalid && nativeProofWork(c.Work, int64(len(frame.children))+1) && (c.Work == nil || c.Work.CheckAlloc(int64(len(frame.children))*96+128) == nil)
	if frame.complete {
		frame.seal = &lambdaMethodEmissionSeal{body: frame.body, request: frame.request, bytes: frame.bytes, children: slices.Clone(frame.children)}
	}
	frame.ended = true
	c.lambdaMethodRendering = frame.previous
}

func (frame *lambdaMethodEmission) closed() bool {
	return frame != nil && frame.complete && frame.ended && !frame.invalid && frame.seal != nil && frame.body == frame.seal.body && frame.request == frame.seal.request && frame.bytes == frame.seal.bytes && slices.Equal(frame.children, frame.seal.children)
}

// Preview renders can use names which later belong to another declaration.
// Only the completed parent's original factory occurrence selects this body's
// actual capture packet. Text elsewhere in the class cannot identify it.
func (c *ClassObjectDumper) lambdaCompletedCapturePacket(parent *dumpedMethods, receipt *lambdaMethodSourceReceipt, charge func(int64) bool) (string, bool) {
	if c == nil || parent == nil || receipt == nil || receipt.body == nil || charge == nil || receipt.object != c.obj || receipt.method != receipt.body.member || receipt.request != receipt.body.sourceRequest || receipt.key != receipt.body.sourceCacheKey || c.lambdaMethodSourceBodies[receipt.method] != receipt || c.dumpedMethodsSet[receipt.key] != receipt.body || receipt.renderedUnknown || receipt.renderedTemplate != receipt.body.code || len(receipt.renderedPackets) > 32 {
		return "", false
	}
	frame := parent.lambdaEmission
	if !frame.closed() || frame.body != parent || frame.request != parent.sourceRequest || len(frame.children) > methodSourceLambdaRequestLimit || receipt.request.factory == nil || !receipt.request.factory.closed() || receipt.request.factory.object != c.obj || receipt.request.factory.method != parent.member || !nativeProofWork(c.Work, int64(len(frame.children))+1) {
		return "", false
	}
	packet := ""
	for _, event := range frame.children {
		if event.body != receipt.body {
			continue
		}
		if packet != "" || event.request != receipt.request || event.packet == "" || len(event.packet) > 1<<20 || !charge(int64(len(event.packet))) {
			return "", false
		}
		packet = event.packet
	}
	if packet == "" {
		return "", false
	}
	matches := 0
	for _, observed := range receipt.renderedPackets {
		if len(observed) > 1<<20 || !charge(int64(len(observed))) {
			return "", false
		}
		if observed == packet {
			matches++
		}
	}
	return packet, matches == 1
}

func (c *ClassObjectDumper) recordLambdaMethodEmission(body *dumpedMethods, template, packet string) bool {
	frame := c.lambdaMethodRendering
	if frame == nil {
		return true
	}
	if frame.body == nil {
		frame.invalid = true
		return false
	}
	if body == nil || body.sourceRequest.factory == nil || body.sourceRequest.factory.method != frame.body.member {
		// A separately owned nested declaration is not this method's factory.
		// If an arrow really appears here, final arrow/event cardinality rejects it.
		return true
	}
	r := c.lambdaReceiptForBody(body)
	if frame.ended || frame.invalid || frame.body == nil || frame.request != frame.body.sourceRequest || body.sourceDumper != c || body.sourceObject != c.obj || template != body.code || r == nil || r.body != body || r.request != body.sourceRequest || r.key != body.sourceCacheKey || c.dumpedMethodsSet[r.key] != body || !r.request.factory.closed() || packet == "" || len(packet) > 1<<20 || len(frame.children) >= methodSourceLambdaRequestLimit || int64(len(packet)) > (4<<20)-frame.bytes || !nativeProofWork(c.Work, int64(len(packet))+1) || c.Work != nil && c.Work.CheckAlloc(frame.bytes+int64(len(packet))+int64(len(frame.children)+1)*96) != nil {
		frame.invalid = true
		return false
	}
	for _, prior := range frame.children {
		if prior.body == body || prior.request == body.sourceRequest {
			frame.invalid = true
			return false
		}
	}
	if !c.bindLambdaLocalCaptureEmission(body, frame) {
		frame.invalid = true
		return false
	}
	if strings.IndexByte(packet, 0) >= 0 && !lambdaSymbolicCapturePacketClosed(body, frame, packet, c.Work) {
		frame.invalid = true
		return false
	}
	frame.children = append(frame.children, lambdaMethodOccurrence{body: body, request: body.sourceRequest, packet: strings.Clone(packet)})
	frame.bytes += int64(len(packet))
	return true
}

// The original header binds an already completed declaration. Later checked
// return-cast projections can change other body expressions without changing
// that declaration or a lambda factory. No header comparison grants body,
// invocation, capture or source-ownership permission.
func (c *ClassObjectDumper) lambdaParentDeclaration(source string, parent *dumpedMethods) (string, int, bool) {
	if c == nil || parent == nil || len(source) > 16<<20 || !nativeProofWork(c.Work, int64(len(source))+int64(len(parent.code))+1) {
		return "", 0, false
	}
	if parent.enumLambdaInitializer != nil {
		return c.enumLambdaInitializerSourceSpan(source, parent)
	}
	if parent.code == "" {
		return c.interfaceLambdaInitializerSourceSpan(source, parent)
	}
	start := strings.Index(source, parent.code)
	if start >= 0 && strings.Index(source[start+len(parent.code):], parent.code) < 0 {
		return parent.code, start, true
	}
	if frame := parent.lambdaEmission; !frame.closed() || frame.body != parent || frame.request != parent.sourceRequest {
		return "", 0, false
	}
	open := javaIndexTopBrace(source)
	end := -1
	if open >= 0 {
		end = javaMatchBrace(source, open)
	}
	headerEnd := javaIndexTopBrace(parent.code)
	if open < 0 || end < 0 || headerEnd < 0 || strings.TrimSpace(source[end+1:]) != "" {
		return "", 0, false
	}
	nodes, known := nativeMemberLayoutDeclarations(source[open+1 : end])
	if !known || len(nodes) > 4096 {
		return "", 0, false
	}
	header := strings.TrimSpace(parent.code[:headerEnd])
	offset := open + 1
	result := ""
	position := 0
	for _, node := range nodes {
		if !nativeProofWork(c.Work, int64(len(node))+1) {
			return "", 0, false
		}
		brace := javaIndexTopBrace(node)
		if brace >= 0 && strings.TrimSpace(node[:brace]) == header {
			if result != "" {
				return "", 0, false
			}
			result, position = node, offset
		}
		offset += len(node)
	}
	return result, position, result != ""
}

func (c *ClassObjectDumper) lambdaEmissionPosition(parent, child *dumpedMethods, declaration string, charge func(int64) bool) (int, bool) {
	if c == nil || parent == nil || child == nil || charge == nil {
		return 0, false
	}
	frame := parent.lambdaEmission
	if !frame.closed() || frame.body != parent || frame.request != parent.sourceRequest || len(frame.children) > methodSourceLambdaRequestLimit || !nativeProofWork(c.Work, int64(len(frame.children))+1) {
		return 0, false
	}
	hasReference := false
	for _, event := range frame.children {
		hasReference = hasReference || event.body != nil && event.body.compilerReference != nil
	}
	arrows, known := nativeLambdaSourceImplementationTokens(declaration, c.Work, hasReference)
	if !known || len(arrows) != len(frame.children) {
		return 0, false
	}
	position, found := 0, false
	for i, event := range frame.children {
		if event.body == nil {
			return 0, false
		}
		r := c.lambdaReceiptForBody(event.body)
		if r == nil || r.body != event.body || r.request != event.request || r.request != event.body.sourceRequest || r.request.factory == nil || r.request.factory.method != parent.member || !r.request.factory.closed() || c.dumpedMethodsSet[r.key] != event.body || !charge(int64(len(event.packet))) {
			return 0, false
		}
		local, closed := nativeLambdaSourceImplementationTokens(event.packet, c.Work, hasReference)
		if !closed || len(local) != 1 || (event.packet[local[0]:local[0]+2] == "::") != (event.body.compilerReference != nil) {
			return 0, false
		}
		header := event.packet[:local[0]+2]
		start := arrows[i] + 2 - len(header)
		if start < 0 || !charge(int64(len(header))) || declaration[start:arrows[i]+2] != header {
			return 0, false
		}
		if event.body == child {
			if found {
				return 0, false
			}
			position, found = arrows[i], true
		}
	}
	return position, found
}
