package javaclassparser

import "maps"

type lambdaSourceConsumptionSnapshot struct {
	bodies                   map[*MemberInfo]*lambdaMethodSourceReceipt
	factories                map[*nativeLambdaLocalCaptureSite]*nativeLambdaLocalCaptureSource
	locals                   map[string]*nativeLambdaLocalCaptureSource
	serializable             map[*serializableReferencePacket]*serializableReferenceEmission
	serializableFingerprints int
}

// Roll back consumption authority when an enclosing source transaction fails.
// Completed independent render packets stay cached; their request serials stay
// spent so a later callback cannot alias the same lexical request.
func (c *ClassObjectDumper) snapshotLambdaSourceConsumption() (*lambdaSourceConsumptionSnapshot, bool) {
	if c == nil {
		return nil, false
	}
	legacy := len(c.lambdaMethodSourceBodies) + len(c.nativeLambdaFactorySources) + len(c.nativeLambdaLocalSources)
	n := legacy + len(c.serializableReferenceEmissions)
	if legacy > 12288 || len(c.serializableReferenceEmissions) > 256 || c.serializableReferenceFingerprintCount < 0 || c.serializableReferenceFingerprintCount > 4096 || !nativeProofWork(c.Work, int64(n+c.serializableReferenceFingerprintCount)+1) || c.Work != nil && c.Work.CheckAlloc(int64(n)*256+int64(c.serializableReferenceFingerprintCount)*64+128) != nil {
		return nil, false
	}
	s := &lambdaSourceConsumptionSnapshot{bodies: maps.Clone(c.lambdaMethodSourceBodies), factories: maps.Clone(c.nativeLambdaFactorySources), locals: maps.Clone(c.nativeLambdaLocalSources), serializable: maps.Clone(c.serializableReferenceEmissions), serializableFingerprints: c.serializableReferenceFingerprintCount}
	observations := 0
	for key, emission := range s.serializable {
		if emission == nil || len(emission.fingerprints) > 16 {
			return nil, false
		}
		copy := *emission
		copy.fingerprints = append([]serializableReferenceFingerprint(nil), emission.fingerprints...)
		s.serializable[key] = &copy
		observations += len(copy.fingerprints)
	}
	if observations != s.serializableFingerprints {
		return nil, false
	}
	for key, receipt := range s.bodies {
		if receipt != nil {
			copy := *receipt
			copy.renderedPackets = append([]string(nil), receipt.renderedPackets...)
			s.bodies[key] = &copy
		}
	}
	for key, source := range s.factories {
		if source != nil {
			copy := *source
			s.factories[key] = &copy
		}
	}
	for key, source := range s.locals {
		if source != nil {
			copy := *source
			s.locals[key] = &copy
		}
	}
	return s, true
}

func (c *ClassObjectDumper) restoreLambdaSourceConsumption(s *lambdaSourceConsumptionSnapshot) {
	c.lambdaMethodSourceBodies = s.bodies
	c.nativeLambdaFactorySources = s.factories
	c.nativeLambdaLocalSources = s.locals
	c.serializableReferenceEmissions = s.serializable
	c.serializableReferenceFingerprintCount = s.serializableFingerprints
}
