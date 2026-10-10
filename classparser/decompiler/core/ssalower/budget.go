package ssalower

import "github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"

// chargeWork records deterministic lowering work before the corresponding
// traversal or allocation. Units represent visited IR objects/relationships,
// not wall time or machine instructions.
func chargeWork(counter ssabuild.WorkCounter, units uint64) error {
	if counter == nil || units == 0 {
		return nil
	}
	return counter.Charge(units)
}

// sortWork is a conservative comparison-work estimate used to charge sorting
// before entering library sort routines whose comparator cannot return errors.
func sortWork(n int) uint64 {
	if n < 2 {
		return 0
	}
	log := uint64(0)
	for size := uint64(n); size > 1; size = (size + 1) / 2 {
		log++
	}
	if uint64(n) > ^uint64(0)/log {
		return ^uint64(0)
	}
	return uint64(n) * log
}

func chargeScaledWork(counter ssabuild.WorkCounter, n int, scale uint64) error {
	if n <= 0 || scale == 0 {
		return nil
	}
	count := uint64(n)
	if count > ^uint64(0)/scale {
		return chargeWork(counter, ^uint64(0))
	}
	return chargeWork(counter, count*scale)
}
