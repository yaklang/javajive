package javaclassparser

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The writer supplies these bytes once. A symbolic packet belongs to its
// actual parent factory frame; it is not a concrete source observation until
// that parent's completed substitutions have been composed and checked.
type lambdaCaptureProjection struct {
	body     *dumpedMethods
	frame    *lambdaMethodEmission
	request  methodSourceRequest
	packet   string
	captures []core.LambdaCaptureRender
	seal     *lambdaCaptureProjection
}

func (p *lambdaCaptureProjection) closed() bool {
	return p != nil && p.seal != nil && p.body == p.seal.body && p.frame == p.seal.frame && p.request == p.seal.request && p.packet == p.seal.packet && slices.Equal(p.captures, p.seal.captures) && p.body != nil && p.body.sourceRequest == p.request && p.request.factory != nil && p.request.factory.closed()
}

type lambdaCaptureComposition struct {
	origin               *lambdaCaptureProjection
	parentPacket, packet string
	captures             []core.LambdaCaptureRender
	seal                 *lambdaCaptureComposition
}

func (p *lambdaCaptureComposition) closed() bool {
	return p != nil && p.seal != nil && p.origin == p.seal.origin && p.parentPacket == p.seal.parentPacket && p.packet == p.seal.packet && slices.Equal(p.captures, p.seal.captures) && p.origin.closed()
}

// Existing concrete render observations retain their original path and cost.
// Only an actual forwarded prefix, or its enclosing factory chain, needs the
// additional substitution observation.
func (c *ClassObjectDumper) lambdaCaptureCompositionRequired(body *dumpedMethods) bool {
	active := map[*dumpedMethods]bool{}
	var walk func(*dumpedMethods) bool
	walk = func(body *dumpedMethods) bool {
		if body == nil {
			return false
		}
		if len(active) >= methodSourceLambdaDepthLimit || active[body] || !nativeProofWork(c.Work, 1) {
			return true
		}
		active[body] = true
		defer delete(active, body)
		if body.lambdaParameterBinding != nil && body.lambdaParameterBinding.hasForwardedCaptures() {
			return true
		}
		if frame := body.lambdaEmission; frame != nil {
			for _, event := range frame.children {
				if walk(event.body) {
					return true
				}
			}
		}
		return false
	}
	return walk(body)
}

func lambdaCaptureMap(captures []core.LambdaCaptureRender, count int, work *workbudget.Budget) (map[int]string, bool) {
	if count < 0 || count > 255 || len(captures) > count || !nativeProofWork(work, int64(len(captures))+1) {
		return nil, false
	}
	result := map[int]string{}
	bytes := 0
	for _, capture := range captures {
		bytes += len(capture.Text)
		if capture.Index < 0 || capture.Index >= count || bytes > 4<<20 || !nativeProofWork(work, int64(len(capture.Text))+1) {
			return nil, false
		}
		if _, duplicate := result[capture.Index]; duplicate {
			return nil, false
		}
		result[capture.Index] = capture.Text
	}
	return result, true
}

// This substitutes already observed bytes, never Java values or renderers.
// Canonical marker indices preserve parent-port identity through composition.
func lambdaComposeCaptureBytes(packet string, captures map[int]string, work *workbudget.Budget) (string, bool) {
	if len(packet) > 1<<20 || !nativeProofWork(work, int64(len(packet))+1) {
		return "", false
	}
	if strings.IndexByte(packet, 0) < 0 {
		return packet, true
	}
	projected := int64(len(packet))
	for cursor := 0; cursor < len(packet); {
		start := strings.IndexByte(packet[cursor:], 0)
		if start < 0 {
			break
		}
		start += cursor
		end := strings.IndexByte(packet[start+1:], 0)
		if end < 0 {
			return "", false
		}
		end += start + 1
		ordinal, prefix := strings.CutPrefix(packet[start+1:end], "LCAP")
		index, err := strconv.Atoi(ordinal)
		text, known := captures[index]
		if !prefix || err != nil || index < 0 || index >= 255 || ordinal != strconv.Itoa(index) || !known || !nativeProofWork(work, int64(len(text))+1) {
			return "", false
		}
		projected += int64(len(text)) - int64(end+1-start)
		if projected < 0 || projected > 1<<20 {
			return "", false
		}
		cursor = end + 1
	}
	if !nativeProofWork(work, int64(len(packet))+1) || work != nil && work.CheckAlloc(projected) != nil {
		return "", false
	}
	var out strings.Builder
	out.Grow(int(projected))
	for cursor := 0; cursor < len(packet); {
		start := strings.IndexByte(packet[cursor:], 0)
		if start < 0 {
			out.WriteString(packet[cursor:])
			break
		}
		start += cursor
		out.WriteString(packet[cursor:start])
		end := strings.IndexByte(packet[start+1:], 0)
		if end < 0 {
			return "", false
		}
		end += start + 1
		ordinal, prefix := strings.CutPrefix(packet[start+1:end], "LCAP")
		index, err := strconv.Atoi(ordinal)
		text, known := captures[index]
		if !prefix || err != nil || index < 0 || index >= 255 || ordinal != strconv.Itoa(index) || !known || len(text) > (1<<20)-out.Len() || !nativeProofWork(work, int64(len(text))+1) || work != nil && work.CheckAlloc(int64(out.Len()+len(text))) != nil {
			return "", false
		}
		out.WriteString(text)
		cursor = end + 1
	}
	if out.Len() > 1<<20 {
		return "", false
	}
	return out.String(), true
}

func lambdaSymbolicCapturePacketClosed(body *dumpedMethods, frame *lambdaMethodEmission, packet string, work *workbudget.Budget) bool {
	if body == nil || body.lambdaParameterBinding == nil || frame == nil {
		return false
	}
	b := body.lambdaParameterBinding
	if !b.hasForwardedCaptures() || !b.forwardingParentMatches(b.forwardParent, frame) || b.forwardEmission != frame {
		return false
	}
	allowed := map[int]string{}
	for i := range b.metadata.captures {
		if index, known := lambdaForwardedCaptureIndex(b.metadata, i); known {
			allowed[index+b.forwardParent.metadata.receiverWords] = "proved-parent-port"
		}
	}
	_, known := lambdaComposeCaptureBytes(packet, allowed, work)
	return known
}

func (c *ClassObjectDumper) recordLambdaCaptureProjection(body *dumpedMethods, template string, actual core.LambdaSourceRender, recordPacket bool) (completed bool) {
	if c == nil || body == nil {
		return false
	}
	frame := c.lambdaMethodRendering
	defer func() {
		if !completed && frame != nil && frame.body != nil && body.sourceRequest.factory != nil && body.sourceRequest.factory.method == frame.body.member {
			frame.invalid = true
		}
	}()
	if !actual.Complete || body.code != template {
		return false
	}
	if _, known := lambdaCaptureMap(actual.Captures, body.sourceRequest.captureCount, c.Work); !known {
		return false
	}
	if !c.recordLambdaMethodEmission(body, template, actual.Packet) {
		return false
	}
	r := c.lambdaMethodSourceBodies[body.member]
	if r == nil || r.body != body || r.request != body.sourceRequest || r.renderedUnknown {
		return false
	}
	if frame != nil && frame.body != nil && body.sourceRequest.factory != nil && body.sourceRequest.factory.method == frame.body.member {
		captures := slices.Clone(actual.Captures)
		sort.Slice(captures, func(i, j int) bool { return captures[i].Index < captures[j].Index })
		var prior *lambdaCaptureProjection
		for _, p := range r.captureProjections {
			if !p.closed() {
				return false
			}
			if p.frame == frame && p.packet == actual.Packet {
				if prior != nil || !slices.Equal(p.captures, captures) {
					return false
				}
				prior = p
			}
		}
		if prior == nil {
			retained := len(actual.Packet) + len(captures)*32
			for _, capture := range captures {
				retained += len(capture.Text)
			}
			if len(r.captureProjections) >= 32 || retained > (4<<20)-r.captureProjectionBytes || !nativeProofWork(c.Work, int64(retained)+1) || c.Work != nil && c.Work.CheckAlloc(int64(retained)*2+256) != nil {
				return false
			}
			p := &lambdaCaptureProjection{body: body, frame: frame, request: body.sourceRequest, packet: strings.Clone(actual.Packet), captures: captures}
			seal := *p
			seal.captures = slices.Clone(captures)
			p.seal = &seal
			r.captureProjections = append(r.captureProjections, p)
			r.captureProjectionBytes += retained
		}
	}
	if strings.IndexByte(actual.Packet, 0) >= 0 {
		return lambdaSymbolicCapturePacketClosed(body, frame, actual.Packet, c.Work)
	}
	if recordPacket && !c.recordNativeLambdaRenderedPacket(body, template, actual.Packet) {
		return false
	}
	return c.materializeLambdaCaptureChildren(body, actual.Packet, actual.Captures, map[*dumpedMethods]bool{})
}

func (c *ClassObjectDumper) materializeLambdaCaptureChildren(parent *dumpedMethods, packet string, captures []core.LambdaCaptureRender, active map[*dumpedMethods]bool) bool {
	if parent == nil || active[parent] || len(active) >= methodSourceLambdaDepthLimit || strings.IndexByte(packet, 0) >= 0 || !nativeProofWork(c.Work, 1) {
		return false
	}
	active[parent] = true
	defer delete(active, parent)
	frame := parent.lambdaEmission
	if frame == nil || len(frame.children) == 0 {
		return true
	}
	if !frame.closed() || frame.body != parent || frame.request != parent.sourceRequest {
		return false
	}
	values, known := lambdaCaptureMap(captures, parent.sourceRequest.captureCount, c.Work)
	if !known {
		return false
	}
	charge := func(n int64) bool { return nativeProofWork(c.Work, n) }
	outer, known := c.nativeLambdaDirectBlockScopes(packet, charge)
	if !known || len(outer) != 1 {
		return false
	}
	content := packet[outer[0].open+1 : outer[0].end]
	scopes, known := c.nativeLambdaDirectBlockScopes(content, charge)
	if !known || len(scopes) != len(frame.children) {
		return false
	}
	for i, event := range frame.children {
		child := event.body
		if child == nil || child.sourceRequest != event.request || child.sourceRequest.factory == nil || child.sourceRequest.factory.method != parent.member {
			return false
		}
		if !c.lambdaCaptureCompositionRequired(child) {
			continue
		}
		r := c.lambdaMethodSourceBodies[child.member]
		if r == nil || r.body != child || r.request != event.request || !c.lambdaMethodSourcePacketClosed(child.member, child, c.Work) {
			return false
		}
		var projection *lambdaCaptureProjection
		for _, p := range r.captureProjections {
			if !p.closed() {
				return false
			}
			if p.frame == frame && p.packet == event.packet {
				if projection != nil {
					return false
				}
				projection = p
			}
		}
		if projection == nil {
			return false
		}
		resolved, known := lambdaComposeCaptureBytes(event.packet, values, c.Work)
		if !known || strings.IndexByte(resolved, 0) >= 0 {
			return false
		}
		local, known := c.nativeLambdaDirectBlockScopes(resolved, charge)
		if !known || len(local) != 1 {
			return false
		}
		start := scopes[i].arrow - local[0].arrow
		if start < 0 || scopes[i].end+1-start != len(resolved) || content[start:scopes[i].end+1] != resolved {
			return false
		}
		var composed []core.LambdaCaptureRender
		for _, capture := range projection.captures {
			text, known := lambdaComposeCaptureBytes(capture.Text, values, c.Work)
			if !known {
				return false
			}
			composed = append(composed, core.LambdaCaptureRender{Index: capture.Index, Text: text})
		}
		if !c.recordNativeLambdaRenderedPacket(child, child.code, resolved) {
			return false
		}
		found := false
		for _, old := range r.captureCompositions {
			if !old.closed() {
				return false
			}
			if old.origin == projection && old.parentPacket == packet && old.packet == resolved {
				if !slices.Equal(old.captures, captures) {
					return false
				}
				found = true
			}
		}
		if !found {
			retained := len(packet) + len(resolved) + len(captures)*32
			if len(r.captureCompositions) >= 32 || retained > (4<<20)-r.captureProjectionBytes || !nativeProofWork(c.Work, int64(retained)+1) || c.Work != nil && c.Work.CheckAlloc(int64(retained)) != nil {
				return false
			}
			record := &lambdaCaptureComposition{origin: projection, parentPacket: strings.Clone(packet), packet: strings.Clone(resolved), captures: slices.Clone(captures)}
			seal := *record
			seal.captures = slices.Clone(captures)
			record.seal = &seal
			r.captureCompositions = append(r.captureCompositions, record)
			r.captureProjectionBytes += retained
		}
		if !c.materializeLambdaCaptureChildren(child, resolved, composed, active) {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) composedLambdaOccurrenceClosed(receipt *lambdaMethodSourceReceipt, parent *dumpedMethods, content, packet string, charge func(int64) bool) bool {
	if receipt == nil || receipt.body == nil || receipt.body.lambdaParameterBinding == nil || parent == nil || charge == nil || receipt.renderedUnknown || receipt.renderedTemplate != receipt.body.code || !receipt.body.lambdaParameterBinding.capturedNamesClosed(c.Work) {
		return false
	}
	frame := parent.lambdaEmission
	if !frame.closed() || frame.body != parent || len(receipt.captureCompositions) > 32 {
		return false
	}
	matches := 0
	type compositionKey struct {
		origin               *lambdaCaptureProjection
		parentPacket, packet string
	}
	seen := map[compositionKey]bool{}
	for _, record := range receipt.captureCompositions {
		if !record.closed() || !charge(int64(len(record.parentPacket))+int64(len(record.packet))+1) {
			return false
		}
		key := compositionKey{record.origin, record.parentPacket, record.packet}
		if seen[key] {
			return false
		}
		seen[key] = true
		p := record.origin
		if p == nil || !p.closed() || p.frame != frame || p.body != receipt.body || p.request != receipt.request || record.packet != packet || !charge(int64(len(record.parentPacket))+int64(len(packet))+1) {
			continue
		}
		outer, known := c.nativeLambdaDirectBlockScopes(record.parentPacket, charge)
		if !known || len(outer) != 1 || record.parentPacket[outer[0].open+1:outer[0].end] != content {
			continue
		}
		parentReceipt := c.lambdaMethodSourceBodies[parent.member]
		if parentReceipt == nil || parentReceipt.body != parent || parentReceipt.request != parent.sourceRequest || parentReceipt.renderedUnknown || parentReceipt.renderedTemplate != parent.code || !c.lambdaMethodSourcePacketClosed(parent.member, parent, c.Work) {
			return false
		}
		parentSeen, originSeen := 0, 0
		for _, observed := range parentReceipt.renderedPackets {
			if observed == record.parentPacket {
				parentSeen++
			}
		}
		for _, origin := range receipt.captureProjections {
			if origin == p {
				originSeen++
			}
		}
		if parentSeen != 1 || originSeen != 1 {
			return false
		}
		values, known := lambdaCaptureMap(record.captures, parent.sourceRequest.captureCount, c.Work)
		if !known {
			return false
		}
		actual, known := lambdaComposeCaptureBytes(p.packet, values, c.Work)
		if !known || actual != packet {
			return false
		}
		seen := 0
		for _, event := range frame.children {
			if event.body == receipt.body && event.request == receipt.request && event.packet == p.packet {
				seen++
			}
		}
		if seen != 1 {
			return false
		}
		matches++
	}
	observed := 0
	for _, candidate := range receipt.renderedPackets {
		if candidate == packet {
			observed++
		}
	}
	return matches == 1 && observed == 1
}
