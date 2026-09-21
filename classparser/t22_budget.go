package javaclassparser

import (
	"context"
	"errors"

	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Public re-exports. Semantics live in internal/workbudget.
type Limits = workbudget.Limits
type WorkBudget = workbudget.Budget

func copyDecompileOptions(in DecompileOptions) DecompileOptions {
	out := in
	if out.EnvSnapshot == nil {
		out.EnvSnapshot = snapshotJDECEnv()
	} else {
		out.EnvSnapshot = cloneStringMap(out.EnvSnapshot)
	}
	applySecureGraphBounds(&out)
	return out
}

// defaultGraphBound is the public-path cap for graph scans/edges/node copies
// when MaxAnalysisUpdates is left at the zero default (solver 1e6).
const defaultGraphBound int64 = 1_000_000

// analysisGraphFactor scales an explicit MaxAnalysisUpdates into graph/scan/copy
// caps so a tiny analysis budget cannot explode CFG construction. LongTest with
// MaxAnalysisUpdates=1 still fits (~28 edges). A 40×40 throw×handler graph does not.
const analysisGraphFactor int64 = 64

func applySecureGraphBounds(opts *DecompileOptions) {
	if opts == nil {
		return
	}
	derived := defaultGraphBound
	if opts.MaxAnalysisUpdates > 0 {
		derived = int64(opts.MaxAnalysisUpdates) * analysisGraphFactor
		if derived < analysisGraphFactor {
			derived = analysisGraphFactor
		}
		if derived > defaultGraphBound {
			derived = defaultGraphBound
		}
	}
	if opts.Limits.MaxGraphEdges == 0 {
		opts.Limits.MaxGraphEdges = derived
	}
	if opts.Limits.MaxGraphScans == 0 {
		opts.Limits.MaxGraphScans = derived
	}
	if opts.Limits.MaxSetElementWork == 0 {
		opts.Limits.MaxSetElementWork = derived
	}
	if opts.Limits.MaxNodeCopies == 0 {
		opts.Limits.MaxNodeCopies = derived
	}
}

func wrapResolve(ctx context.Context, budget *workbudget.Budget, snap map[string]string, resolve func(internalName string) ([]byte, bool)) func(internalName string) ([]byte, bool) {
	if resolve == nil {
		return nil
	}
	return func(internalName string) ([]byte, bool) {
		if budget != nil {
			if err := budget.Check(); err != nil {
				return nil, false
			}
		} else if ctx != nil {
			select {
			case <-ctx.Done():
				return nil, false
			default:
			}
		}
		var data []byte
		var ok bool
		_ = jdecenv.Run(snap, func() error {
			data, ok = resolve(internalName)
			return nil
		})
		if budget != nil {
			if err := budget.Check(); err != nil {
				return nil, false
			}
		} else if ctx != nil && ctx.Err() != nil {
			return nil, false
		}
		return data, ok
	}
}

func statusForDecompileError(err error) string {
	if err == nil {
		return "complete"
	}
	var be *workbudget.Error
	if errors.As(err, &be) {
		if be.Kind == workbudget.KindCanceled {
			return "canceled"
		}
		return "resource_limit"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	return "unsupported"
}

func isRequestWorkError(err error) bool {
	return workbudget.Is(err)
}

func (c *ClassObjectDumper) checkWork() error {
	if c == nil {
		return nil
	}
	if c.Work != nil {
		return c.Work.Check()
	}
	if c.options.Context != nil {
		if err := c.options.Context.Err(); err != nil {
			return &workbudget.Error{Kind: workbudget.KindCanceled, Err: err}
		}
	}
	return nil
}

func (c *ClassObjectDumper) chargeOutput(n int64) error {
	return c.holdOutput(n)
}

func (c *ClassObjectDumper) holdOutput(n int64) error {
	if c == nil {
		return nil
	}
	if c.FuncCtx != nil {
		return c.FuncCtx.HoldOutput(n)
	}
	if c.Work == nil {
		return nil
	}
	if err := c.Work.CheckAlloc(n); err != nil {
		return err
	}
	return c.Work.CheckOutput(n)
}

func (c *ClassObjectDumper) ensureOutput(total int64) error {
	if c == nil || c.Work == nil {
		return nil
	}
	if err := c.Work.CheckAlloc(total); err != nil {
		return err
	}
	if c.FuncCtx != nil && total > c.FuncCtx.OutputHeld {
		c.FuncCtx.OutputHeld = total
	}
	return c.Work.EnsureOutput(total)
}

func (c *ClassObjectDumper) appendDiagnostic(d DecompileDiagnostic) {
	if c == nil || c.report == nil {
		return
	}
	if c.Work != nil {
		if err := c.Work.Charge(workbudget.CounterDiagnostics, 1); err != nil {
			c.noteDiagnosticsTruncated(err)
			return
		}
	}
	c.report.Diagnostics = append(c.report.Diagnostics, d)
}

func (c *ClassObjectDumper) noteDiagnosticsTruncated(err error) {
	if c == nil || c.report == nil || c.diagTruncated {
		return
	}
	c.diagTruncated = true
	msg := "diagnostics_truncated"
	if err != nil {
		msg = err.Error()
	}
	c.report.Diagnostics = append(c.report.Diagnostics, DecompileDiagnostic{Code: "diagnostics_truncated", Message: msg})
}

func (c *ClassObjectDumper) consultResolverForCancel() error {
	if c == nil || c.foldSiblingResolver == nil {
		return c.checkWork()
	}
	super := ""
	if c.obj != nil {
		super = c.obj.GetSupperClassName()
	}
	if super != "" {
		_, _ = c.foldSiblingResolver(super)
	}
	return c.checkWork()
}
