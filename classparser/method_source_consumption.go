package javaclassparser

import "maps"

type lambdaSourceConsumptionSnapshot struct {
	bodies    map[*MemberInfo]*lambdaMethodSourceReceipt
	factories map[*nativeLambdaLocalCaptureSite]*nativeLambdaLocalCaptureSource
	locals    map[string]*nativeLambdaLocalCaptureSource
}

// Roll back consumption authority when an enclosing source transaction fails.
// Completed independent render packets stay cached; their request serials stay
// spent so a later callback cannot alias the same lexical request.
func (c *ClassObjectDumper) snapshotLambdaSourceConsumption() (*lambdaSourceConsumptionSnapshot, bool) {
	if c == nil {
		return nil, false
	}
	n := len(c.lambdaMethodSourceBodies) + len(c.nativeLambdaFactorySources) + len(c.nativeLambdaLocalSources)
	if n > 12288 || !nativeProofWork(c.Work, int64(n)+1) || c.Work != nil && c.Work.CheckAlloc(int64(n)*256+128) != nil {
		return nil, false
	}
	s := &lambdaSourceConsumptionSnapshot{bodies: maps.Clone(c.lambdaMethodSourceBodies), factories: maps.Clone(c.nativeLambdaFactorySources), locals: maps.Clone(c.nativeLambdaLocalSources)}
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
}
