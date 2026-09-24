package javaclassparser

import (
	"fmt"
	"math"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssalower"
	"github.com/yaklang/javajive/internal/workbudget"
)

const defaultShadowAnalysisUpdates = 1_000_000

// Install the production shadow pipeline after methodir's snapshot-only hook.
// It is only called when EnableShadowIR is explicitly set; source reconstruction
// continues to use the existing printer until the new pipeline has separate
// parity evidence.
func init() { core.ShadowIRBuilder = buildShadowIRAlgorithms }

type shadowAnalysisCounter struct {
	limit  uint64
	used   uint64
	budget *workbudget.Budget
}

func (c *shadowAnalysisCounter) Charge(units uint64) error {
	if c == nil || units == 0 {
		return nil
	}
	if units > math.MaxUint64-c.used {
		return &workbudget.Error{
			Kind: workbudget.KindResource, Counter: workbudget.CounterAnalysisUpdates,
			Used: math.MaxInt64, Limit: uint64AsInt64(c.limit),
		}
	}
	next := c.used + units
	if c.used > c.limit || units > c.limit-c.used {
		return &workbudget.Error{
			Kind: workbudget.KindResource, Counter: workbudget.CounterAnalysisUpdates,
			Used: uint64AsInt64(next), Limit: uint64AsInt64(c.limit),
		}
	}
	if units > math.MaxInt64 {
		return &workbudget.Error{Kind: workbudget.KindResource, Counter: workbudget.CounterAnalysisUpdates, Used: math.MaxInt64, Limit: uint64AsInt64(c.limit)}
	}
	if c.budget != nil {
		if err := c.budget.Charge(workbudget.CounterAnalysisUpdates, int64(units)); err != nil {
			return err
		}
	}
	c.used += units
	return nil
}

func uint64AsInt64(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}

func buildShadowIRAlgorithms(req core.ShadowIRRequest) (string, uint64, error) {
	meta := methodir.MethodMeta{
		Limits: req.Limits, ClassName: req.ClassName, Name: req.MethodName,
		Descriptor: req.Descriptor, Bytecode: append([]byte(nil), req.Bytecode...), IsStatic: req.IsStatic,
	}
	ir, err := methodir.BuildFromCFG(req.CFG, meta, req.D)
	if err != nil {
		return "", 0, fmt.Errorf("shadow MethodIR: %w", err)
	}
	maxUpdates := defaultShadowAnalysisUpdates
	var budget *workbudget.Budget
	if req.D != nil {
		if req.D.MaxAnalysisUpdates > 0 {
			maxUpdates = req.D.MaxAnalysisUpdates
		}
		if req.D.Work != nil {
			budget = req.D.Work
		}
	}
	counter := &shadowAnalysisCounter{limit: uint64(maxUpdates), budget: budget}
	ssa, err := ssabuild.Build(ir, ssabuild.Options{MaxUpdates: maxUpdates, Counter: counter})
	if err != nil {
		return "", 0, fmt.Errorf("shadow SSA: %w", err)
	}
	// Lowering charges its actual block/edge/phi/operand traversals and reserves
	// container work before allocation. Reuse the same request counter as SSA.
	lowered, err := ssalower.DestroyWithWorkCounter(ssa, ssalower.Options{MaxSpills: maxUpdates}, counter)
	if err != nil {
		return "", 0, fmt.Errorf("shadow lowering: %w", err)
	}
	if lowered == nil || lowered.IR != ir || lowered.SSA != ssa {
		return "", 0, fmt.Errorf("shadow lowering: incomplete emission plan")
	}
	throwsUnmoved, err := lowered.ThrowsUnmovedWithCounter(counter)
	if err != nil {
		return "", 0, fmt.Errorf("shadow lowering budget: %w", err)
	}
	if !throwsUnmoved {
		return "", 0, fmt.Errorf("shadow lowering: exception coverage invariant failed")
	}
	// Bind the observation to all three stages, not just the bytecode snapshot.
	// The lowered formatter includes only ordered slices, never its internal maps.
	return methodir.CombineHashes(
		methodir.CombineHashes(ir.Hash(), ssa.Normalize()),
		loweredShadowCanonical(lowered),
	), ir.Version, nil
}

func loweredShadowCanonical(lowered *ssalower.Lowered) string {
	if lowered == nil {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "temps=%d\n", lowered.Temps)
	writeMoves := func(prefix string, moves []ssalower.Move) {
		for _, move := range moves {
			fmt.Fprintf(&out, "%s %d<-%d tmp=%t\n", prefix, move.Dst, move.Src, move.Tmp)
		}
	}
	writeMoves("entry", lowered.EntryMoves)
	for _, assign := range lowered.Assigns {
		fmt.Fprintf(&out, "edge %s split=%t id=%d origin=%d coverage=%v\n", assign.Edge.String(), assign.Split, assign.SplitID, assign.OriginPC, assign.Coverage)
		writeMoves("move", assign.Moves)
	}
	for _, split := range lowered.Splits {
		fmt.Fprintf(&out, "split %d %s origin=%d coverage=%v\n", split.ID, split.Edge.String(), split.OriginPC, split.Coverage)
		writeMoves("move", split.Moves)
	}
	for _, site := range lowered.Exception.Sites {
		fmt.Fprintf(&out, "throw %d coverage=%v\n", site.PC, site.Coverage)
		for _, spill := range site.Spills {
			fmt.Fprintf(&out, "spill handler=%d dst=%d src=%d slot=%d type=%s\n", spill.Handler, spill.Destination, spill.Source, spill.LocalSlot, spill.Type)
		}
		writeMoves("exception-move", site.Moves)
	}
	for _, binding := range lowered.Exception.Catch {
		fmt.Fprintf(&out, "catch handler=%d dst=%d type=%s\n", binding.Handler, binding.Destination, binding.Type)
	}
	return out.String()
}
