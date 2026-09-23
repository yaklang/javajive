package ssabuild

import (
	"fmt"
	"math"
)

type WorkCounter interface {
	Charge(units uint64) error
}

func sortWorkEstimate(n int) uint64 {
	if n < 2 {
		return 0
	}
	log := uint64(0)
	for size := uint64(n); size > 1; size = (size + 1) / 2 {
		log++
	}
	if uint64(n) > math.MaxUint64/log {
		return math.MaxUint64
	}
	return uint64(n) * log
}

type LimitCounter struct {
	Max  uint64
	Used uint64
}

func (c *LimitCounter) Charge(units uint64) error {
	if c == nil {
		return nil
	}
	if units > math.MaxUint64-c.Used || c.Max > 0 && (c.Used > c.Max || units > c.Max-c.Used) {
		return fmt.Errorf("analysis_budget_exceeded: after %d updates", c.Used)
	}
	c.Used += units
	return nil
}

func charge(c WorkCounter, n uint64) error {
	if c == nil {
		return nil
	}
	return c.Charge(n)
}
