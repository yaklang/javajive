package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"slices"
	"strings"
)

// A successful constant-body renderer owns this receipt. Its lambda plans
// remain provisional until the entire enum constant list is actually returned.
// Class-name resemblance or a body hidden in comments confers no authority.
type sourceFlatEnumPublication struct {
	proof       *nativeEnumConstantBody
	body        string
	sourceHash  [32]byte
	bodyHash    [32]byte
	publication *sourceLambdaPublication
}

func (c *ClassObjectDumper) recordFlatEnumPublication(proof *nativeEnumConstantBody, child *ClassObjectDumper, source, body string) bool {
	if c.options.sourceLambdaMetadata == nil {
		return true
	}
	if proof == nil || child == nil || child.obj != proof.object || child.flatEnumConstantCurrent != proof || child.sourceDegraded || body == "" {
		return false
	}
	a := c.options.sourceLambdaMetadata
	p := a.publication(proof.object.GetClassName(), source)
	if p == nil {
		return true
	}
	if !c.artifactLambdaMetadataAllowed() || len(c.sourceFlatEnumBodies) >= 4096 || !nativeProofWork(c.Work, int64(len(body))+1) || c.Work.CheckAlloc(int64(len(body))+256) != nil {
		return false
	}
	for _, prior := range c.sourceFlatEnumBodies {
		if prior.proof.plan.constant == proof.plan.constant {
			return false
		}
	}
	c.sourceFlatEnumBodies = append(c.sourceFlatEnumBodies, sourceFlatEnumPublication{proof: proof, body: strings.Clone(body), sourceHash: sha256.Sum256([]byte(source)), bodyHash: sha256.Sum256([]byte(body)), publication: p})
	return true
}

func (c *ClassObjectDumper) bindFlatEnumPublicationPackets(packets []enumLambdaConstantPacket) bool {
	if len(c.sourceFlatEnumBodies) == 0 {
		return true
	}
	if len(packets) == 0 || len(packets) > 4096 || !nativeProofWork(c.Work, int64(len(packets))+1) || c.Work.CheckAlloc(int64(len(packets))*64) != nil {
		return false
	}
	if c.sourceFlatEnumPackets != nil {
		return slices.Equal(c.sourceFlatEnumPackets, packets) && slices.Equal(c.sourceFlatEnumPackets, c.sourceFlatEnumPacketSeal)
	}
	c.sourceFlatEnumPackets = slices.Clone(packets)
	c.sourceFlatEnumPacketSeal = slices.Clone(packets)
	return true
}

func (c *ClassObjectDumper) flatEnumPublicationsClosed(source string) ([]*sourceLambdaPublication, bool) {
	if len(c.sourceFlatEnumBodies) == 0 {
		return nil, true
	}
	if !c.artifactLambdaMetadataAllowed() || c.sourceDegraded || legacySourceHasFailureMarker(source) || len(c.sourceFlatEnumBodies) > 4096 || len(source) > 16<<20 || !nativeProofWork(c.Work, int64(len(source))+1) {
		return nil, false
	}
	plans, err := c.nativeEnumConstantInitializations()
	if err != nil || len(plans) != len(c.sourceFlatEnumPackets) || !slices.Equal(c.sourceFlatEnumPackets, c.sourceFlatEnumPacketSeal) || c.Work.CheckAlloc(int64(len(source))*2+int64(len(plans))*512) != nil {
		return nil, false
	}
	var packets strings.Builder
	ordinals := make(map[string]int, len(plans))
	bodyOrdinal := 0
	for i, packet := range c.sourceFlatEnumPackets {
		plan, found := plans[packet.name]
		if !found || plan.constant != packet.name || plan.ordinal != i || packet.source == "" || !nativeProofWork(c.Work, int64(len(packet.source))+1) || packets.Len()+len(packet.source) > 16<<20 {
			return nil, false
		}
		packets.WriteString(packet.source)
		if plan.allocatedClass != c.obj.GetClassName() {
			bodyOrdinal++
			ordinals[packet.name] = bodyOrdinal
		}
	}
	// Only the leading constant-list region is eligible, never a substring in
	// an unrelated method or comment. The packets came from the typed writer.
	start := javaIndexTopBrace(source) + 1
	if start <= 0 {
		return nil, false
	}
	for start < len(source) {
		if strings.ContainsRune(" \t\r\n", rune(source[start])) {
			start++
			continue
		}
		if start+1 < len(source) && source[start] == '/' && (source[start+1] == '/' || source[start+1] == '*') {
			next, ok := skipJavaNonCode(source, start)
			if !ok || next <= start {
				return nil, false
			}
			start = next
			continue
		}
		break
	}
	if !strings.HasPrefix(source[start:], strings.TrimLeft(packets.String(), " \t\r\n")) {
		return nil, false
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	bridges := c.originalNativeConstructorAccessBridges()
	out := make([]*sourceLambdaPublication, 0, len(c.sourceFlatEnumBodies))
	seen := map[string]bool{}
	for _, member := range c.sourceFlatEnumBodies {
		p := member.proof
		if p == nil || p.object == nil || member.publication == nil || seen[p.plan.constant] || plans[p.plan.constant] != p.plan {
			return nil, false
		}
		seen[p.plan.constant] = true
		if !nativeProofWork(c.Work, int64(len(member.body))+1) || member.bodyHash != sha256.Sum256([]byte(member.body)) {
			return nil, false
		}
		proof := flatEnumConstantBodyProof(c.obj, p.plan, ordinals[p.plan.constant], bridges, resolve, c.Work)
		if proof == nil || !bytes.Equal(proof.object.Bytes(), p.object.Bytes()) || proof.descriptor != p.descriptor || proof.superDescriptor != p.superDescriptor || proof.superPC != p.superPC {
			return nil, false
		}
		raw, bound := c.options.sourceLambdaMetadata.original(p.object, c.Work)
		pub := member.publication
		if !bound || pub.owner != p.object.GetClassName() || pub.originalHash != sha256.Sum256(raw) || pub.sourceHash != member.sourceHash || len(pub.plans) == 0 || strings.Count(c.sourceFlatEnumPackets[p.plan.ordinal].source, member.body) != 1 {
			return nil, false
		}
		out = append(out, pub)
	}
	return out, true
}
