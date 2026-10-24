package javaclassparser

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A declaration and a factory occurrence are different identities. Modern
// javac can share one static implementation between several original factories
// while still consuming a counter slot at each source occurrence. Keep the
// request's own receipt; a later occurrence cannot replace an earlier writer.
func (c *ClassObjectDumper) lambdaReceiptForBody(body *dumpedMethods) *lambdaMethodSourceReceipt {
	if c == nil || body == nil {
		return nil
	}
	r := c.lambdaMethodSourceBodies[body.member]
	if r == nil || r.object != c.obj || r.method != body.member {
		return nil
	}
	if r.body == body {
		if r.request == body.sourceRequest && r.key == body.sourceCacheKey {
			return r
		}
		return nil
	}
	if len(r.shared) > methodSourceLambdaRequestLimit || !nativeProofWork(c.Work, int64(len(r.shared))+1) {
		return nil
	}
	for _, occurrence := range r.shared {
		if occurrence != nil && occurrence.body == body && occurrence.request == body.sourceRequest && occurrence.key == body.sourceCacheKey && c.sharedStaticLambdaSites(occurrence, r) {
			return occurrence
		}
	}
	return nil
}

// This initial deduplication domain has no captures, free type variables,
// lexical child declarations or nested factory bodies. Both actual bootstrap
// edges must target the same original implementation with exactly its physical
// inputs/result. Debug-line/variable emission disables javac's deduplication.
// Source occurrences in other domains retain the existing refusal.
func (c *ClassObjectDumper) sharedStaticLambdaSites(a, b *lambdaMethodSourceReceipt) bool {
	if c == nil || c.obj == nil || c.obj.MajorVersion != 52 || c.options.SourceCompiler.effective() != ModernJavac || a == nil || b == nil || a.body == nil || b.body == nil || a.method != b.method || a.object != c.obj || b.object != c.obj || a.body.compilerReference != nil || b.body.compilerReference != nil || a.body.lambdaParameterBinding != nil || b.body.lambdaParameterBinding != nil || a.request.role != methodSourceLambdaBody || b.request.role != methodSourceLambdaBody || a.request.captureCount != 0 || b.request.captureCount != 0 || a.body.code != b.body.code || a.body.descriptor != b.body.descriptor || a.method == nil || a.method.AccessFlags != 0x100a || !a.body.lambdaEmission.closed() || !b.body.lambdaEmission.closed() || len(a.body.lambdaEmission.children) != 0 || len(b.body.lambdaEmission.children) != 0 {
		return false
	}
	if a.body.lambdaEmission.body != a.body || b.body.lambdaEmission.body != b.body || a.body.lambdaEmission.request != a.request || b.body.lambdaEmission.request != b.request {
		return false
	}
	af, bf := a.request.factory, b.request.factory
	if !af.closed() || !bf.closed() || !af.sealed || !bf.sealed || af.method != bf.method || af.code != bf.code || af.pc == bf.pc || isSyntheticMethod(af.method.AccessFlags) || !c.compilerReferenceConcreteFactoryScope(af) || !c.compilerReferenceConcreteFactoryScope(bf) || !c.lambdaScheduleOwnDeclarationScope() {
		return false
	}
	if p := c.nativeMemberRoot; p != nil && p.owner != "" {
		return false
	}
	// A consumed arrow has no declaration header, so its rendering cache need
	// not carry descriptor. The physical port contract is the original member's
	// typed descriptor; neither a cache label nor a source name supplies it.
	name, named := sourceBridgeUTF8(c.obj, a.method.NameIndex)
	descriptor, typed := sourceBridgeUTF8(c.obj, a.method.DescriptorIndex)
	if !named || !typed || a.body.methodName != name || b.body.methodName != name {
		return false
	}
	as, ak := c.originalCompilerReferenceSite(af)
	bs, bk := c.originalCompilerReferenceSite(bf)
	if !ak || !bk || as.captureCount != 0 || bs.captureCount != 0 || as.implementation != bs.implementation || as.implementation.Member != name || as.implementation.Description != descriptor || as.actual != descriptor || bs.actual != descriptor {
		return false
	}
	signature, known := nativeMethodLocalOriginalSignature(c.obj, a.method.Attributes, c.Work)
	if !known {
		return false
	}
	if signature != "" {
		erased, _, known := types.EraseConcreteMethodSignatureWithThrows(signature)
		if !known || erased != descriptor {
			return false
		}
	}
	for _, attribute := range a.method.Attributes {
		if code, ok := attribute.(*CodeAttribute); ok {
			for _, nested := range code.Attributes {
				if !nativeProofWork(c.Work, 1) {
					return false
				}
				switch value := nested.(type) {
				case *LineNumberTableAttribute:
					return false
				case *UnparsedAttribute:
					if value == nil || value.Name == "LineNumberTable" || value.Name == "LocalVariableTable" || value.Name == "LocalVariableTypeTable" {
						return false
					}
				}
			}
		}
	}
	return true
}

func (c *ClassObjectDumper) publishedFlatLambdaReceipts(methods []*dumpedMethods) ([]*lambdaMethodSourceReceipt, bool) {
	if !nativeProofWork(c.Work, int64(len(methods))+1) {
		return nil, false
	}
	var receipts []*lambdaMethodSourceReceipt
	seen := map[*dumpedMethods]bool{}
	for _, parent := range methods {
		if parent == nil || parent.lambdaEmission == nil || len(parent.lambdaEmission.children) == 0 {
			continue
		}
		frame := parent.lambdaEmission
		if !frame.closed() || frame.body != parent || frame.request != parent.sourceRequest {
			return nil, false
		}
		for _, event := range frame.children {
			r := c.lambdaReceiptForBody(event.body)
			if r == nil || r.body != event.body || r.request != event.request || r.request != event.body.sourceRequest || seen[event.body] || len(receipts) >= methodSourceLambdaRequestLimit || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(len(receipts)+1)*128) != nil {
				return nil, false
			}
			seen[event.body] = true
			receipts = append(receipts, r)
		}
	}
	return receipts, true
}

// Compare actual final block bytes, excluding the functional-interface carrier
// casts. These are recorded by the original parent writer and reopened at its
// final source position. Identical cached templates alone grant no permission
// to claim that javac will deduplicate two changed final source bodies.
func (c *ClassObjectDumper) sharedLambdaFinalBlock(parent, child *dumpedMethods, declaration string, position int, charge func(int64) bool) (string, bool) {
	var packet string
	for _, event := range parent.lambdaEmission.children {
		if event.body == child {
			if packet != "" {
				return "", false
			}
			packet = event.packet
		}
	}
	local, known := c.nativeLambdaDirectBlockScopes(packet, charge)
	if !known || len(local) != 1 {
		return "", false
	}
	final, known := c.nativeLambdaDirectBlockScopes(declaration, charge)
	if !known {
		return "", false
	}
	for _, scope := range final {
		if scope.arrow == position {
			body := declaration[scope.arrow+2 : scope.end+1]
			return body, body == packet[local[0].arrow+2:local[0].end+1]
		}
	}
	return "", false
}

func (c *ClassObjectDumper) normalizeSharedLambdaCounters(events []nativeLambdaSourceIdentityEvent) ([]nativeLambdaSourceIdentityEvent, bool) {
	if c.options.SourceCompiler.effective() != ModernJavac {
		return events, true
	}
	candidate := false
	for _, receipt := range c.lambdaMethodSourceBodies {
		candidate = candidate || receipt != nil && len(receipt.shared) != 0
	}
	if !candidate {
		return events, true
	}
	if c.Work != nil && c.Work.CheckAlloc(int64(len(events))*256) != nil {
		return nil, false
	}
	first := map[*MemberInfo]nativeLambdaSourceIdentityEvent{}
	next := 0
	result := slices.Clone(events)
	for i, event := range result {
		if event.body == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		method := event.body.member
		if prior, repeated := first[method]; repeated {
			a, b := c.lambdaReceiptForBody(prior.body), c.lambdaReceiptForBody(event.body)
			if a == nil || b == nil || prior.parent != event.parent || prior.packet == "" || prior.packet != event.packet || !c.sharedStaticLambdaSites(a, b) {
				return nil, false
			}
			result[i].ordinal = next
		} else {
			first[method] = event
		}
		next = result[i].ordinal + 1
	}
	return result, true
}
