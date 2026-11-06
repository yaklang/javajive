package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

func init() {
	core.StackPhiEvidenceProvider = ssaStackPhiEvidence
}

type ssaJoinEvidenceCache struct {
	function *ssabuild.Function
	err      error
}

// ssaStackPhiEvidence builds MethodIR + SSA for the current method on the first
// stack-join query and caches the result on the Decompiler. The SSA frame model
// proves whether the joined word is primitive or a reference; a contradiction
// with the expression-level inferred type aborts that method instead of
// emitting a mixed assignment. Unsupported SSA inputs simply provide no
// evidence, leaving the existing structural proof in charge.
func ssaStackPhiEvidence(d *core.Decompiler, merge *core.OpCode) (core.StackPhiEvidence, bool) {
	if d == nil || merge == nil {
		return core.StackPhiEvidence{}, false
	}
	cache, _ := d.StackPhiEvidenceCache.(*ssaJoinEvidenceCache)
	if cache == nil {
		cache = &ssaJoinEvidenceCache{}
		d.StackPhiEvidenceCache = cache
		cfg := d.SemanticCFGForEvidence()
		if cfg != nil && d.FunctionContext != nil && len(d.TestBytecodes()) != 0 {
			meta := methodir.MethodMeta{
				Limits:     d.CodeLimits,
				ClassName:  d.FunctionContext.ClassName,
				Name:       d.FunctionContext.FunctionName,
				Descriptor: d.FunctionContext.CurrentMethodDesc,
				Bytecode:   d.TestBytecodes(),
				IsStatic:   d.FunctionContext.IsStatic,
			}
			ir, err := methodir.BuildFromCFG(cfg, meta, d)
			if err != nil {
				cache.err = err
			} else {
				maxUpdates := d.MaxAnalysisUpdates
				if maxUpdates <= 0 {
					maxUpdates = 1_000_000
				}
				fn, buildErr := ssabuild.Build(ir, ssabuild.Options{MaxUpdates: maxUpdates})
				if buildErr != nil {
					cache.err = buildErr
				} else {
					cache.function = fn
				}
			}
		}
	}
	if cache.function == nil {
		return core.StackPhiEvidence{}, false
	}
	var block *ssabuild.BlockFrame
	for i := range cache.function.Blocks {
		if cache.function.Blocks[i].First == merge.CurrentOffset {
			block = &cache.function.Blocks[i]
			break
		}
	}
	if block == nil {
		return core.StackPhiEvidence{}, false
	}
	var top *ssabuild.Phi
	for i := range cache.function.Phis {
		phi := &cache.function.Phis[i]
		if phi.Block != block.ID || phi.Slot.Local {
			continue
		}
		if top == nil || phi.Slot.Index > top.Slot.Index {
			top = phi
		}
	}
	if top == nil {
		return core.StackPhiEvidence{}, false
	}
	switch top.Type.Kind {
	case frametransfer.Int, frametransfer.Float, frametransfer.Long, frametransfer.Double:
		return core.StackPhiEvidence{Kind: core.StackPhiPrimitive}, true
	case frametransfer.Ref, frametransfer.UninitThis, frametransfer.UninitNew:
		return core.StackPhiEvidence{
			Kind:  core.StackPhiReference,
			Class: strings.ReplaceAll(top.Type.Class, "/", "."),
		}, true
	case frametransfer.Null:
		return core.StackPhiEvidence{Kind: core.StackPhiNull}, true
	default:
		return core.StackPhiEvidence{}, false
	}
}
