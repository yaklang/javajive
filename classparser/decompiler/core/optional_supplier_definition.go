package core

import "github.com/yaklang/javajive/classparser/decompiler/core/values"

// Keep the factory declaration as a use-site view, not a local type. Retyping a
// raw Supplier<X> globally would introduce an X checkcast at unrelated get()
// uses, changing polluted raw-interface behavior. Only a unique recorded store
// can supply the exception type for Optional.orElseThrow's erased supplier.
func (d *Decompiler) restoreOptionalSupplierDefinitionViews() {
	if d.FunctionContext == nil || d.semanticCFG == nil || d.getenv("JDEC_OPTIONAL_SUPPLIER_DEFINITION_OFF") != "" {
		return
	}
	// Identity may already have been unified across distinct physical refs. Count
	// every store of that source variable, including unknown/null/reused stores.
	definitions := map[string][]*OpCode{}
	factories := map[*values.FunctionCallExpression]bool{}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			continue
		}
		if call := d.invokeFuncCall[op]; call != nil && call.HasOriginPC && call.OriginPC == int(op.CurrentOffset) {
			factories[call] = true
		}
		if !isLocalStoreOpcode(op.Instr.OpCode) {
			continue
		}
		for _, info := range d.opcodeIdToRef[op] {
			if ref, ok := info[0].(*values.JavaRef); ok && ref != nil {
				definitions[ref.VarUid] = append(definitions[ref.VarUid], op)
			}
		}
	}
	for _, op := range d.opCodes {
		call := d.invokeFuncCall[op]
		if call == nil || len(call.Arguments) != 1 {
			continue
		}
		ref, ok := values.UnpackSoltValue(call.Arguments[0]).(*values.JavaRef)
		if !ok || ref == nil || ref.IsParam || ref.VarUid == "" {
			continue
		}
		stores := definitions[ref.VarUid]
		if len(stores) != 1 || len(stores[0].stackConsumed) != 1 {
			continue
		}
		// The immutable consumed RHS is authoritative. Do not chase ref.Val,
		// which can still denote an obsolete definition after a slot reuse.
		factory, ok := values.UnpackSoltValue(stores[0].stackConsumed[0]).(*values.FunctionCallExpression)
		if !ok || factory == nil || !factories[factory] || factory.SourceReturnType == nil {
			continue
		}
		if target := call.OptionalGenericThrowsSupplierTarget(factory.SourceReturnType, d.FunctionContext); target != nil {
			storeIndex := d.semanticCFG.indexOfNode(stores[0])
			callIndex := d.semanticCFG.indexOfNode(op)
			if storeIndex < 0 || callIndex < 0 || !d.semanticCFG.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{Roots: []int{0}, IncludeException: true}).Dominates(storeIndex, callIndex) {
				continue
			}
			call.Arguments[0] = &values.CastExpression{Value: call.Arguments[0], TargetType: target, Binding: true, OriginPC: call.OriginPC}
		}
	}
}
