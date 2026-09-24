package javaclassparser

import (
	"github.com/yaklang/javajive/internal/utils"
	"maps"
	"slices"
)

// snapshotRetryState restores every dumper-owned mutable collection and scalar
// after a declined retry. Work already charged stays charged (retry is real work).
func (c *ClassObjectDumper) snapshotRetryState() func() {
	saved := *c
	originalContext := c.FuncCtx
	saved.FuncCtx = c.FuncCtx.CloneForRetry()
	saved.rewriteSeen = maps.Clone(c.rewriteSeen)
	saved.overloadUnknownSeen = maps.Clone(c.overloadUnknownSeen)
	saved.fieldDefaultValue = maps.Clone(c.fieldDefaultValue)
	saved.lambdaMethods = maps.Clone(c.lambdaMethods)
	for k, v := range saved.lambdaMethods {
		saved.lambdaMethods[k] = slices.Clone(v)
	}
	saved.lambdaCaptureCount = maps.Clone(c.lambdaCaptureCount)
	saved.fieldStoreTotals = maps.Clone(c.fieldStoreTotals)
	saved.methodReturnTypes = maps.Clone(c.methodReturnTypes)
	saved.recordSkipMethods = maps.Clone(c.recordSkipMethods)
	saved.recordSkipFields = maps.Clone(c.recordSkipFields)
	saved.aggressiveRetried = maps.Clone(c.aggressiveRetried)
	saved.bootstrapReports = slices.Clone(c.bootstrapReports)
	saved.dumpedMethodsSet = maps.Clone(c.dumpedMethodsSet)
	for k, v := range saved.dumpedMethodsSet {
		if v != nil {
			vCopy := *v
			saved.dumpedMethodsSet[k] = &vCopy
		}
	}
	saved.deepStack = utils.NewStack[int]()
	if c.deepStack != nil {
		xs := c.deepStack.Values()
		for i := len(xs) - 1; i >= 0; i-- {
			saved.deepStack.Push(xs[i])
		}
	}
	var report DecompileResult
	if c.report != nil {
		report = *c.report
		report.Members = slices.Clone(report.Members)
		report.Shadow = slices.Clone(report.Shadow)
		report.Diagnostics = slices.Clone(report.Diagnostics)
		report.StubMethods = slices.Clone(report.StubMethods)
		report.RulesApplied = slices.Clone(report.RulesApplied)
	}
	return func() {
		*c = saved
		if originalContext != nil {
			*originalContext = *saved.FuncCtx
			c.FuncCtx = originalContext
		}
		if c.report != nil {
			*c.report = report
		}
	}
}
