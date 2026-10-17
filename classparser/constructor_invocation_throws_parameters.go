package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

type constructorThrowsParameter struct {
	ref                          *values.JavaRef
	id                           *utils.VariableId
	slot                         int
	typeKey, erasure, sourceName string
}

func (c *ClassObjectDumper) constructorThrowsParameters(plan *constructorSourceBoundary, descriptor string) ([]constructorThrowsParameter, bool) {
	physical, result, err := callbinding.Descriptor(descriptor)
	if err != nil || result != "V" || len(physical) != len(plan.params) || len(physical) > 128 {
		return nil, false
	}
	var parameters []constructorThrowsParameter
	seen := map[*utils.VariableId]bool{}
	slot := 1
	for i, param := range plan.params {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		ref, ok := param.(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || seen[ref.Id] {
			return nil, false
		}
		// Match renderMethodParamType's typed force-erasure decision. Neither
		// a source spelling nor a later Object retype can prove this header.
		if raw, ok := ref.Type().RawType().(*types.JavaClass); ok && raw != nil {
			if replacement, changed := c.FuncCtx.ForceParamEraseTypeVar(raw.Name); changed && replacement != "" {
				return nil, false
			}
		}
		originalSlot, original := ref.OriginalParameterSlot()
		key, erasure, known := constructorThrowsTypeKey(ref.Type(), c.FuncCtx)
		if !original || originalSlot != slot || !known || erasure != physical[i] {
			return nil, false
		}
		seen[ref.Id] = true
		parameters = append(parameters, constructorThrowsParameter{ref: ref, id: ref.Id, slot: slot, typeKey: key, erasure: erasure})
		slot++
		if physical[i] == "J" || physical[i] == "D" {
			slot++
		}
	}
	return parameters, true
}

func (r *constructorInvocationThrowsReceipt) sourceParametersFresh(ctx *class_context.ClassContext) bool {
	if r == nil || r.plan == nil || len(r.plan.params) != len(r.parameters) {
		return false
	}
	current, known := r.dumper.constructorThrowsParameters(r.plan, r.methodDesc)
	if !known || len(current) != len(r.parameters) {
		return false
	}
	for i, parameter := range current {
		old := r.parameters[i]
		if parameter.ref != old.ref || parameter.id != old.id || parameter.slot != old.slot || parameter.typeKey != old.typeKey || parameter.erasure != old.erasure || r.parameterBindingCount != 0 && parameter.ref.String(ctx) != old.sourceName {
			return false
		}
	}
	return true
}

// This is called exactly where the actual header parameter refs have been
// projected. Source naming may settle before this point. It cannot change after
// it without invalidating the delegation's source-identifier binding.
func (r *constructorInvocationThrowsReceipt) bindSourceParameters(plan *constructorSourceBoundary, header []values.JavaValue, ctx *class_context.ClassContext) bool {
	if r == nil || plan != r.plan || plan.throwsReceipt != r || r.parameterBindingCount != 0 || r.renderCount != 0 || len(header) != len(r.parameters) || !r.fresh(ctx) {
		return false
	}
	names := make([]string, len(header))
	seen := map[string]bool{}
	for i, parameter := range header {
		if parameter != r.parameters[i].ref || !nativeProofWork(r.dumper.Work, 1) {
			return false
		}
		name := parameter.String(ctx)
		if name == "" || seen[name] || class_context.SafeIdentifier(name) != name {
			return false
		}
		names[i] = name
		seen[name] = true
	}
	for i, name := range names {
		r.parameters[i].sourceName = name
	}
	r.parameterBindingCount = 1
	return true
}

func (c *ClassObjectDumper) constructorThrowsOriginalExceptions(method *MemberInfo) ([]string, bool) {
	if c == nil || method == nil {
		return nil, false
	}
	count, attributes := 0, 0
	for _, attribute := range method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if exceptions, ok := attribute.(*ExceptionsAttribute); ok {
			attributes++
			if exceptions == nil || attributes > 1 || len(exceptions.ExceptionIndexTable) > 128-count {
				return nil, false
			}
			for range exceptions.ExceptionIndexTable {
				if !nativeProofWork(c.Work, 1) {
					return nil, false
				}
				count++
			}
		}
	}
	declared, known := originalMethodExceptions(c.obj, method)
	if !known || len(declared) != count {
		return nil, false
	}
	for _, exception := range declared {
		if len(exception) > 8192 || !nativeProofWork(c.Work, int64(len(exception))+1) {
			return nil, false
		}
	}
	return declared, true
}

// The current header writer recovers Signature throws only for a visible
// formal. Check its typed visibility/projection decisions, never its text.
func (c *ClassObjectDumper) constructorThrowsHeaderView(methodSig string) bool {
	if methodSig == "" {
		return true
	}
	_, _, result, throws := types.ParseMethodSignatureFullWithThrows(methodSig, c.FuncCtx)
	if result == nil {
		return false
	}
	formal := false
	for _, thrown := range throws {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if name, variable := types.TypeVariableName(thrown); variable {
			formal = true
			if !c.FuncCtx.IsTypeParam(name) || class_context.SafeIdentifier(name) != name {
				return false
			}
		}
	}
	if !formal {
		return true
	}
	if c.getenv("JDEC_THROWS_SIG_RECOVERY_OFF") != "" {
		return false
	}
	for _, thrown := range throws {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if raw, ok := thrown.RawType().(*types.JavaClass); ok && raw != nil {
			if replacement, changed := c.FuncCtx.StandaloneEraseTypeVar(raw.Name); changed && replacement != "" {
				return false
			}
		}
	}
	return true
}
