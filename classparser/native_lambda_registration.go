package javaclassparser

import (
	"sort"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// LambdaToMethod appends implementation methods to their owning class before
// Lower registers private-access bridges. The arrow's body is therefore not
// visited at the factory expression's textual position. A completed anonymous
// source packet supplies both the actual bodies and original emitted method
// order; no method-name spelling or source scan grants implementation ownership.
func nativeAnonymousLambdaRegistrationSource(child *nativeMemberClass, c *ClassObjectDumper, source string) (string, bool) {
	if child == nil {
		return source, true
	}
	if c == nil || child.object == nil || c.obj != child.object {
		return source, false
	}
	p := c.nativeMemberRoot
	if p == nil || len(p.getters) == 0 || len(child.lambdaImplementations) == 0 {
		return source, true
	}
	if len(child.lambdaImplementations) > 64 || len(child.object.Methods) > 4096 || !nativeMemberLambdaSourceClosed(child, c, c.Work) {
		return source, false
	}
	constructors, known := nativeMemberConstructorRegistrations(p, c.Work)
	if !known {
		return source, false
	}
	var packets []string
	for _, method := range child.object.Methods {
		if !nativeProofWork(c.Work, 1) {
			return source, false
		}
		if !child.lambdaImplementations[method] {
			continue
		}
		body := c.lambdaMethodSourceBody(method, c.Work)
		if body == nil || body.sourceReconstructionFailed() || body.member != method {
			return source, false
		}
		name, named := sourceBridgeUTF8(child.object, method.NameIndex)
		desc, typed := sourceBridgeUTF8(child.object, method.DescriptorIndex)
		if !named || !typed || len(child.lambdaContext.factorySites[name+desc]) != 1 {
			return source, false
		}
		receipt := c.lambdaMethodSourceBodies[method]
		if receipt == nil || receipt.renderedUnknown || receipt.renderedTemplate != body.code {
			return source, false
		}
		packet := ""
		for _, rendered := range receipt.renderedPackets {
			if !nativeProofWork(c.Work, int64(len(source))) {
				return source, false
			}
			if strings.Contains(source, rendered) {
				if packet != "" {
					return source, false
				}
				packet = rendered
			}
		}
		if packet == "" {
			return source, false
		}
		packets = append(packets, packet)
	}
	if len(packets) != len(child.lambdaImplementations) {
		return source, false
	}
	return nativeDeferredAccessorComments(p, source, packets, constructors, c.Work)
}

// The streaming renderer supplies these exact bytes; do not reconstruct its
// capture spelling from ref.Val, names, or another method's current context.
// Reopen the live consumption receipt so failed enclosing transactions cannot
// publish observations from a discarded body or an earlier source request.
func (c *ClassObjectDumper) recordNativeLambdaRenderedPacket(body *dumpedMethods, template, rendered string) bool {
	if c == nil || body == nil || template == "" || rendered == "" || strings.IndexByte(rendered, 0) >= 0 || len(rendered) > 1<<20 || body.code != template {
		return false
	}
	r := c.lambdaReceiptForBody(body)
	if r == nil || r.body != body || r.object != c.obj || r.method != body.member || r.request != body.sourceRequest || r.key != body.sourceCacheKey || r.request.factory == nil || !r.request.factory.closed() || !c.lambdaMethodSourcePacketClosed(body.member, body, c.Work) || r.renderedUnknown {
		return false
	}
	if r.renderedTemplate != "" && r.renderedTemplate != template {
		return false
	}
	for _, prior := range r.renderedPackets {
		if prior == rendered {
			return r.renderedTemplate == template
		}
	}
	if len(r.renderedPackets) >= 32 || len(rendered) > (4<<20)-r.renderedBytes {
		r.renderedUnknown = true
		return false
	}
	if !nativeProofWork(c.Work, int64(len(rendered))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(rendered))+int64(len(r.renderedPackets)+1)*16) != nil {
		return false
	}
	r.renderedTemplate = template
	r.renderedPackets = append(r.renderedPackets, strings.Clone(rendered))
	r.renderedBytes += len(rendered)
	return true
}

// Move only existing, owned compiler-event comments. Every executable token,
// ordinary comment, factory, capture and invocation remains byte-for-byte in
// place. Whole unique packets and lexical arrows rule out quoted/stale bodies;
// nested/multiply emitted implementations need their own phase proof.
func nativeDeferredAccessorComments(p *nativeMemberFamily, source string, packets []string, constructors map[string]bool, work *workbudget.Budget) (string, bool) {
	if p == nil || p.failed || len(source) > 16<<20 || len(packets) > 64 || !nativeProofWork(work, int64(len(source))+1) || work != nil && work.CheckAlloc(int64(len(source))*3+4096) != nil {
		return source, false
	}
	open := javaIndexTopBrace(source)
	close := -1
	if open >= 0 {
		close = javaMatchBrace(source, open)
	}
	if open < 0 || close < 0 || strings.TrimSpace(source[close+1:]) != "" {
		return source, false
	}
	type span struct{ start, end int }
	var bodies, removed []span
	var deferred strings.Builder
	previous := len(source)
	scanned := int64(len(source))
	charge := func(n int64) bool {
		if n < 0 || n > 128<<20-scanned {
			return false
		}
		scanned += n
		return nativeProofWork(work, n)
	}
	for _, packet := range packets {
		if packet == "" || !charge(int64(len(source))+int64(len(packet))) {
			return source, false
		}
		start := strings.Index(source, packet)
		if start <= open || start+len(packet) > close || strings.Index(source[start+len(packet):], packet) >= 0 {
			return source, false
		}
		// The supported flat LambdaToMethod lowering prepends completed
		// implementations: their emitted order is the reverse of factories
		// in this actual source scope. An arbitrarily reordered classfile
		// cannot borrow that compiler-phase schedule.
		if start >= previous {
			return source, false
		}
		previous = start
		local, known := nativeLambdaSourceArrows(packet, work)
		arrow, located := nativeLambdaSourcePacketArrow(source, packet, work, charge)
		if !known || len(local) != 1 || !located || arrow != start+local[0] {
			return source, false
		}
		body := span{start, start + len(packet)}
		for _, prior := range bodies {
			if body.start < prior.end && prior.start < body.end {
				return source, false
			}
		}
		bodies = append(bodies, body)
		events, known := nativeMemberAccessorEvents(p, packet, constructors, work)
		if !known {
			return source, false
		}
		for _, event := range events {
			removed = append(removed, span{start + event.start, start + event.end})
			deferred.WriteString(packet[event.start:event.end])
		}
	}
	if len(removed) == 0 {
		return source, true
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].start < removed[j].start })
	var out strings.Builder
	out.Grow(len(source))
	cursor := 0
	for _, span := range removed {
		if span.start < cursor {
			return source, false
		}
		out.WriteString(source[cursor:span.start])
		cursor = span.end
	}
	out.WriteString(source[cursor:close])
	out.WriteString(deferred.String())
	out.WriteString(source[close:])
	return out.String(), true
}
