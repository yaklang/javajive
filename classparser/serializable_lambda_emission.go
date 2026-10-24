package javaclassparser

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// Factory identity is immutable and belongs to the original Code/PC. Rendering
// observations belong to the enclosing source transaction and must roll back.
// Keep fingerprints rather than copies of up to 256 * 16 large expressions.
type serializableReferenceEmission struct {
	fingerprints []serializableReferenceFingerprint
	attempts     int
}

type serializableReferenceFingerprint struct {
	size, referenceOffset int
	digest                [sha256.Size]byte
}

func (c *ClassObjectDumper) serializableReferenceEmission(packet *serializableReferencePacket) (*serializableReferenceEmission, bool) {
	if c == nil || packet == nil || packet.plan == nil || packet.plan != c.serializableReferencePlan || packet.plan.object != c.obj || packet.index < 0 || packet.index >= len(packet.plan.sites) || c.serializableReferencePackets[packet.index] != packet || !nativeProofWork(c.Work, 1) {
		return nil, false
	}
	if found := c.serializableReferenceEmissions[packet]; found != nil {
		return found, true
	}
	if len(c.serializableReferenceEmissions) >= 256 || c.Work != nil && c.Work.CheckAlloc(int64(len(c.serializableReferenceEmissions)+1)*128+int64(c.serializableReferenceFingerprintCount)*64) != nil {
		return nil, false
	}
	if c.serializableReferenceEmissions == nil {
		c.serializableReferenceEmissions = map[*serializableReferencePacket]*serializableReferenceEmission{}
	}
	emission := &serializableReferenceEmission{}
	c.serializableReferenceEmissions[packet] = emission
	return emission, true
}

func serializableReferenceFingerprintOf(text string) (serializableReferenceFingerprint, bool) {
	var out serializableReferenceFingerprint
	if len(text) == 0 || len(text) > 1<<20 || strings.Contains(text, `"""`) {
		return out, false
	}
	state, depth, reference := scanNormal, 0, -1
	for i := 0; i < len(text); i++ {
		state = scanAdvance(text, &i, state, &depth)
		if state == scanNormal && i+1 < len(text) && text[i:i+2] == "::" {
			if reference >= 0 {
				return out, false
			}
			reference = i
			i++
		}
	}
	if reference < 0 {
		return out, false
	}
	return serializableReferenceFingerprint{size: len(text), referenceOffset: reference, digest: sha256.Sum256([]byte(text))}, true
}

func (c *ClassObjectDumper) recordSerializableReferenceEmission(packet *serializableReferencePacket, text string) error {
	if len(text) == 0 || len(text) > 1<<20 || !nativeProofWork(c.Work, int64(len(text))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(text))+256) != nil {
		return fmt.Errorf("serializable reference writer packet unavailable")
	}
	fingerprint, known := serializableReferenceFingerprintOf(text)
	// Resolve the current transaction every time. A retained writer must not
	// capture an emission pointer that a later rollback has replaced.
	emission, live := c.serializableReferenceEmission(packet)
	if !known || !live {
		return fmt.Errorf("serializable reference writer has no original factory")
	}
	for _, previous := range emission.fingerprints {
		if previous == fingerprint {
			return nil
		}
	}
	if len(emission.fingerprints) >= 16 || c.serializableReferenceFingerprintCount >= 4096 || c.Work != nil && c.Work.CheckAlloc(int64(len(c.serializableReferenceEmissions))*128+int64(c.serializableReferenceFingerprintCount+1)*64) != nil {
		return fmt.Errorf("serializable reference writer packet limit")
	}
	emission.fingerprints = append(emission.fingerprints, fingerprint)
	c.serializableReferenceFingerprintCount++
	return nil
}

// The digest only identifies bytes observed at the already-proved factory's
// concrete writer. It grants no type, target, scope or protocol authority.
func serializableReferenceFingerprintCount(source string, fingerprint serializableReferenceFingerprint, charge func(int64) bool) (int, bool) {
	if fingerprint.size <= 0 || fingerprint.size > 1<<20 || fingerprint.referenceOffset < 0 || fingerprint.referenceOffset+2 > fingerprint.size || len(source) > 16<<20 || strings.Contains(source, `"""`) || charge == nil || !charge(int64(len(source))+1) {
		return 0, false
	}
	state, depth, count := scanNormal, 0, 0
	for i := 0; i < len(source); i++ {
		state = scanAdvance(source, &i, state, &depth)
		if state != scanNormal || i+1 >= len(source) || source[i:i+2] != "::" {
			continue
		}
		start := i - fingerprint.referenceOffset
		if start >= 0 && fingerprint.size <= len(source)-start {
			if !charge(int64(fingerprint.size)) {
				return 0, false
			}
			if sha256.Sum256([]byte(source[start:start+fingerprint.size])) == fingerprint.digest {
				count++
			}
		}
		i++
	}
	return count, true
}
