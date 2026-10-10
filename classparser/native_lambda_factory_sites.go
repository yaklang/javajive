package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The implementation signature is not a factory identity. In particular javac
// can deduplicate a hidden-field capture and a method-parameter capture into
// the same private helper. Join the original method, Code attribute and PC.
func (c *ClassObjectDumper) recordNativeLambdaFactorySource(name, desc string, code *CodeAttribute, captured []values.JavaValue, pc int, hasPC bool) (*nativeLambdaLocalCaptureSite, bool) {
	if c == nil {
		return nil, false
	}
	current := c.nativeMemberCurrent
	if current == nil {
		current = c.nativeAnonymousLambdaCurrent
	}
	if current == nil || len(current.lambdaContext.factorySites[name+desc]) <= 1 {
		return nil, c.recordNativeLambdaLocalCaptureSource(name, desc, code, captured)
	}
	factories := current.lambdaContext.factorySites[name+desc]
	if !hasPC || current.object != c.obj || c.FuncCtx == nil || len(factories) > 64 || len(c.nativeLambdaFactorySources) >= 4096 || !nativeProofWork(c.Work, int64(len(factories)+len(captured))) || c.Work != nil && c.Work.CheckAlloc(int64(len(captured))*64+512) != nil {
		return nil, false
	}
	var selected *nativeLambdaLocalCaptureSite
	for _, site := range factories {
		if site == nil || site.method == nil || site.code == nil || site.pc < 0 || site.pc >= len(site.code.Code) {
			return nil, false
		}
		if site.method == c.CurrentMethod && site.code == code && site.pc == pc {
			if selected != nil {
				return nil, false
			}
			selected = site
		}
	}
	if selected == nil || len(captured) != len(selected.operands) || c.nativeLambdaFactorySources[selected] != nil {
		return nil, false
	}
	if c.nativeLambdaFactorySources == nil {
		c.nativeLambdaFactorySources = map[*nativeLambdaLocalCaptureSite]*nativeLambdaLocalCaptureSource{}
	}
	c.nativeLambdaFactorySources[selected] = &nativeLambdaLocalCaptureSource{method: c.CurrentMethod, code: code, factoryPC: pc, hasFactoryPC: true, context: *c.FuncCtx, values: append([]values.JavaValue(nil), captured...)}
	return selected, true
}

// Complete source consumption includes every original factory, even factories
// with only parameters or no captures. A successful render at a neighboring PC
// cannot cover a missing factory or authorize another source type environment.
func nativeLambdaFactorySourcesClosed(method *MemberInfo, factories []*nativeLambdaLocalCaptureSite, dumper *ClassObjectDumper, work *workbudget.Budget) bool {
	if method == nil || dumper == nil || len(factories) < 2 || len(factories) > 64 || !nativeProofWork(work, int64(len(factories))) || work != nil && work.CheckAlloc(int64(len(factories))*64) != nil {
		return false
	}
	seen := map[*nativeLambdaLocalCaptureSite]bool{}
	seenBodies := map[*dumpedMethods]bool{}
	for _, site := range factories {
		if site == nil || seen[site] {
			return false
		}
		seen[site] = true
		source := dumper.nativeLambdaFactorySources[site]
		if source == nil || !source.hasFactoryPC || source.factoryPC != site.pc || source.method != site.method || source.code != site.code || site.method == nil || site.code == nil || site.pc < 0 || site.pc >= len(site.code.Code) || len(source.values) != len(site.operands) || source.body == nil {
			return false
		}
		if !nativeLambdaFactoryRetained(source, work) {
			return false
		}
		body := source.implementationBody
		if body == nil || seenBodies[body] || body.member != method || body.bodyCode == "stub" || body.checkedEscape || strings.Contains(body.code, DecompileStubMarker) {
			return false
		}
		seenBodies[body] = true
		if len(site.operands) != 0 && !nativeLambdaLocalCaptureSourceClosed(site, source, work) {
			return false
		}
	}
	return true
}

// Parsing a factory is not enough: dead-store removal or later source
// structuring can discard it. Require its exact capture vector in the final
// retained graph. JavaRef children follow only rendered inline values, never
// the historical Val of an ordinary local declaration.
func nativeLambdaFactoryRetained(source *nativeLambdaLocalCaptureSource, work *workbudget.Budget) bool {
	if source == nil || source.body == nil || !source.hasFactoryPC || work != nil && work.CheckAlloc(8192*32) != nil {
		return false
	}
	remaining, matches := 8192, 0
	activeValues := map[values.JavaValue]bool{}
	activeStatements := map[statements.Statement]bool{}
	step := func() bool { remaining--; return remaining >= 0 && nativeProofWork(work, 1) }
	var value func(values.JavaValue) bool
	value = func(v values.JavaValue) bool {
		if sourceProofNil(v) {
			return true
		}
		if !step() || activeValues[v] {
			return false
		}
		activeValues[v] = true
		defer delete(activeValues, v)
		if lambda, ok := v.(*values.CustomValue); ok && lambda.Flag == "lambda" && lambda.HasOriginPC && lambda.OriginPC == source.factoryPC {
			if lambda.IsMethodRef || !lambda.CapturesKnown || len(lambda.Captures) != len(source.values) {
				return false
			}
			for i, capture := range lambda.Captures {
				if capture != source.values[i] {
					return false
				}
			}
			matches++
			if matches > 1 {
				return false
			}
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !value(child) {
				return false
			}
		}
		return true
	}
	var walk func([]statements.Statement) bool
	walk = func(body []statements.Statement) bool {
		for _, st := range body {
			if sourceProofNil(st) || !step() || activeStatements[st] {
				return false
			}
			activeStatements[st] = true
			roots, children, known := nativeSourceNameChildren(st)
			if !known {
				return false
			}
			for _, root := range roots {
				if !value(root) {
					return false
				}
			}
			for _, child := range children {
				if !walk(child) {
					return false
				}
			}
			delete(activeStatements, st)
		}
		return true
	}
	return walk(source.body) && matches == 1
}
