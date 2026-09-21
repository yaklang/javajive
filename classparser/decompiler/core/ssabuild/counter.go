package ssabuild

import "fmt"

type WorkCounter interface {
	Charge(units uint64) error
}

type LimitCounter struct {
	Max  uint64
	Used uint64
}

func (c *LimitCounter) Charge(units uint64) error {
	if c == nil {
		return nil
	}
	c.Used += units
	if c.Max > 0 && c.Used > c.Max {
		return fmt.Errorf("analysis_budget_exceeded: after %d updates", c.Used)
	}
	return nil
}

func charge(c WorkCounter, n uint64) error {
	if c == nil {
		return nil
	}
	return c.Charge(n)
}
