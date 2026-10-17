package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A producer and all its source allocations must share a dominating lexical
// block. Enumerate actual statement blocks, never source spelling. Each candidate
// retains the original STORE/invocation witnesses, whole-method write exclusion
// and exhaustive NEW-site closure. A nested producer cannot license a sibling or
// enclosing allocation. This does not hoist any computation across a branch,
// loop iteration, exception handler or monitor boundary.
func (c *ClassObjectDumper) nativeMethodLocalProducerBindings(local *nativeMethodLocalClass, body []statements.Statement, params map[int]*values.JavaRef) (map[string]*values.JavaRef, statements.Statement, bool) {
	if c == nil || local == nil || local.constructor == nil {
		return nil, nil, false
	}
	produced := false
	for _, site := range local.allocations {
		for _, origin := range site.origins {
			if !nativeProofWork(c.Work, 1) {
				return nil, nil, false
			}
			produced = produced || origin.Kind == ssabuild.OriginInstr
		}
	}
	// Parameter-only and type-only declarations retain their existing method
	// head certificate. No produced declaration needs a new lexical anchor.
	if !produced {
		return c.nativeMethodLocalProducerBindingsInBlock(local, body, body, params)
	}
	blocks := [][]statements.Statement{}
	seen := map[statements.Statement]bool{}
	remaining := 4096
	var collect func([]statements.Statement, int, bool) bool
	collect = func(ss []statements.Statement, depth int, emittable bool) bool {
		if depth >= 64 || !nativeProofWork(c.Work, 1) {
			return false
		}
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				return false
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
			if c.Work.CheckAlloc(int64(len(seen)+len(ss))*128+int64(len(blocks)+1)*32) != nil {
				return false
			}
		}
		if emittable {
			blocks = append(blocks, ss)
		}
		for _, st := range ss {
			remaining--
			if remaining < 0 || sourceProofNil(st) || seen[st] || !nativeProofWork(c.Work, 1) {
				return false
			}
			seen[st] = true
			_, children, known := nativeSourceNameChildren(st)
			if !known {
				return false
			}
			for i, child := range children {
				// A for header is not a declaration-emission block. Its synthetic
				// visitor slices must not grant a place for a local class declaration.
				_, header := st.(*statements.ForStatement)
				if !collect(child, depth+1, emittable && !(header && i > 0)) {
					return false
				}
			}
		}
		return true
	}
	if !collect(body, 0, true) {
		return nil, nil, false
	}
	for _, block := range blocks {
		if !nativeProofWork(c.Work, int64(len(block)+1)) {
			return nil, nil, false
		}
		if bindings, anchor, known := c.nativeMethodLocalProducerBindingsInBlock(local, block, body, params); known {
			return bindings, anchor, true
		}
		if c.Work != nil && c.Work.Err() != nil {
			return nil, nil, false
		}
	}
	return nil, nil, false
}
