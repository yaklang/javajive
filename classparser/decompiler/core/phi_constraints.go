package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

// Descriptors supply upper bounds on the source declaration. In particular,
// incomplete dependency resolution must not turn a value stored into an
// InputStream field into an Object local, and a marker interface is not a useful
// join when the bytecode invokes a different shared interface's members.
func (d *Decompiler) referenceUseConstraints() map[*values.JavaRef][]types.JavaType {
	out := map[*values.JavaRef][]types.JavaType{}
	add := func(value values.JavaValue, typ types.JavaType) {
		ref, ok := values.UnpackSoltValue(value).(*values.JavaRef)
		if !ok || typ == nil {
			return
		}
		name, ok := types.RawClassFQN(typ)
		if !ok || name == "java.lang.Object" {
			return
		}
		out[ref] = append(out[ref], typ)
	}
	for _, op := range d.opCodes {
		stack := op.stackConsumed
		switch op.Instr.OpCode {
		case OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE, OP_INVOKESTATIC, OP_INVOKESPECIAL:
			member := d.GetMethodFromPool(int(Convert2bytesToInt(op.Data[:2])))
			params := member.JavaType.FunctionType().ParamTypes
			if len(stack) < len(params) {
				continue
			}
			for i, typ := range params {
				add(stack[len(params)-1-i], typ)
			}
			if op.Instr.OpCode != OP_INVOKESTATIC && len(stack) > len(params) {
				add(stack[len(params)], types.NewJavaClass(strings.ReplaceAll(member.Name, "/", ".")))
			}
		case OP_PUTFIELD, OP_PUTSTATIC:
			if len(stack) > 0 {
				add(stack[0], d.GetMethodFromPool(int(Convert2bytesToInt(op.Data))).JavaType)
			}
		case OP_ARETURN:
			if len(stack) > 0 && d.FunctionType != nil {
				add(stack[0], d.FunctionType.ReturnType)
			}
		case OP_AASTORE:
			if len(stack) > 2 && stack[2].Type().IsArray() {
				add(stack[0], stack[2].Type().ElementType())
			}
		}
	}
	return out
}

func (d *Decompiler) constrainWebDeclaration(joined types.JavaType, stores []*OpCode, uses map[*values.JavaRef][]types.JavaType) types.JavaType {
	name, ok := types.RawClassFQN(joined)
	if !ok {
		return joined
	}
	var bounds []types.JavaType
	seen := map[string]bool{}
	// Traverse the bytecode-owned refs in opcode order, never map iteration order.
	for _, op := range stores {
		for _, info := range d.opcodeIdToRef[op] {
			ref, ok := info[0].(*values.JavaRef)
			if !ok {
				continue
			}
			for _, typ := range uses[ref] {
				n, _ := types.RawClassFQN(typ)
				if !seen[n] {
					bounds = append(bounds, typ)
					seen[n] = true
				}
			}
		}
	}
	provider := d.FunctionContext.SiblingSuperTypes
	if name != "java.lang.Object" && !types.HasKnownDirectSupertypes(name, provider) {
		// A missing dependency is not evidence that its concrete type violates
		// an invoke bound. Widening an external exception to Throwable loses
		// the precise unchecked/checked exception type recovered from bytecode.
		return joined
	}
	formalBounds := types.ClassFormalTypeParamErasures(d.FunctionContext.ClassSig)
	if formalBounds == nil {
		formalBounds = map[string]string{}
	}
	for n, bound := range types.ClassFormalTypeParamErasures(d.FunctionContext.CurrentMethodSig) {
		formalBounds[n] = bound
	}
	isSubtype := func(a, b string) bool {
		if a == b {
			return true
		}
		if bound := formalBounds[a]; bound != "" {
			a = strings.ReplaceAll(bound, "/", ".")
		}
		return types.IsReferenceSubtypeBridged(a, b, provider)
	}
	accessible := func(n string) bool {
		if check := d.FunctionContext.SiblingClassAccessible; check != nil {
			if allowed, known := check(strings.ReplaceAll(n, ".", "/")); known {
				return allowed
			}
		}
		return true
	}
	compatible := accessible(name)
	for _, bound := range bounds {
		n, _ := types.RawClassFQN(bound)
		compatible = compatible && isSubtype(name, n)
	}
	if compatible {
		return joined
	}
	for _, candidate := range bounds {
		n, _ := types.RawClassFQN(candidate)
		if !accessible(n) {
			continue
		}
		valid := true
		for _, bound := range bounds {
			b, _ := types.RawClassFQN(bound)
			if !isSubtype(n, b) {
				valid = false
				break
			}
		}
		if valid {
			return candidate.Copy()
		}
	}
	return joined
}
