package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/utils"
	"maps"
	"slices"
)

// snapshotRetryState restores every dumper-owned mutable collection and scalar
// after a declined retry. Work already charged stays charged (retry is real work).
func (c *ClassObjectDumper) snapshotRetryState() func() {
	saved := *c
	originalContext := c.FuncCtx
	var originalRenderState *class_context.SourceRenderState
	if originalContext != nil {
		originalRenderState = originalContext.SourceRendering
	}
	saved.FuncCtx = c.FuncCtx.CloneForRetry()
	saved.rewriteSeen = maps.Clone(c.rewriteSeen)
	saved.overloadUnknownSeen = maps.Clone(c.overloadUnknownSeen)
	saved.fieldDefaultValue = maps.Clone(c.fieldDefaultValue)
	saved.lambdaMethods = maps.Clone(c.lambdaMethods)
	for k, v := range saved.lambdaMethods {
		saved.lambdaMethods[k] = slices.Clone(v)
	}
	saved.lambdaCaptureCount = maps.Clone(c.lambdaCaptureCount)
	saved.lambdaAnonymousScopes = maps.Clone(c.lambdaAnonymousScopes)
	saved.lambdaCaptureTypes = maps.Clone(c.lambdaCaptureTypes)
	for k, v := range saved.lambdaCaptureTypes {
		saved.lambdaCaptureTypes[k] = slices.Clone(v)
	}
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
	saved.lambdaMethodSourceBodies = maps.Clone(c.lambdaMethodSourceBodies)
	for member, receipt := range saved.lambdaMethodSourceBodies {
		if receipt != nil {
			copy := *receipt
			if body := saved.dumpedMethodsSet[copy.key]; body != nil {
				copy.body = body
			}
			copy.shared = slices.Clone(receipt.shared)
			for i, occurrence := range copy.shared {
				if occurrence == nil {
					continue
				}
				entry := *occurrence
				entry.shared = nil
				if body := saved.dumpedMethodsSet[entry.key]; body != nil {
					entry.body = body
				}
				copy.shared[i] = &entry
			}
			saved.lambdaMethodSourceBodies[member] = &copy
		}
	}
	saved.nativeLambdaFactorySources = maps.Clone(c.nativeLambdaFactorySources)
	for site, source := range saved.nativeLambdaFactorySources {
		if source != nil {
			copy := *source
			copy.values, copy.body = slices.Clone(source.values), slices.Clone(source.body)
			if source.implementationBody != nil {
				copy.implementationBody = saved.dumpedMethodsSet[source.implementationBody.sourceCacheKey]
			}
			saved.nativeLambdaFactorySources[site] = &copy
		}
	}
	saved.nativeLambdaLocalSources = maps.Clone(c.nativeLambdaLocalSources)
	for key, source := range saved.nativeLambdaLocalSources {
		if source != nil {
			copy := *source
			copy.values, copy.body = slices.Clone(source.values), slices.Clone(source.body)
			if source.implementationBody != nil {
				copy.implementationBody = saved.dumpedMethodsSet[source.implementationBody.sourceCacheKey]
			}
			saved.nativeLambdaLocalSources[key] = &copy
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
		spentRequestSerial := c.methodSourceRequestSerial
		*c = saved
		// Actual callback identities and their finite capacity are spent work,
		// even when the retry restores every failed cache and source index.
		if spentRequestSerial > c.methodSourceRequestSerial {
			c.methodSourceRequestSerial = spentRequestSerial
		}
		if originalContext != nil {
			*originalContext = *saved.FuncCtx
			if originalRenderState != nil && saved.FuncCtx.SourceRendering != nil {
				*originalRenderState = *saved.FuncCtx.SourceRendering
				originalContext.SourceRendering = originalRenderState
			}
			c.FuncCtx = originalContext
		}
		if c.report != nil {
			*c.report = report
		}
	}
}
