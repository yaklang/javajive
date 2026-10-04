package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"math"
	"strings"
)

type nativeAnonymousInitializer struct {
	field, descriptor, capture string
	literal                    *values.JavaLiteral
}

// These are original post-SUPER stores, not moved constructor captures. Java
// recreates the proved captures before SUPER, then runs this exact ordered
// instance block. Lexical capture expressions regenerate physical GETFIELD
// reads here; direct parameter loads are not equivalent after a parent changes
// the captured field reflectively. No unproved parameter read or effect escapes.
func nativeAnonymousInitializerPackets(obj *ClassObject, ops []*core.OpCode, start int, child *nativeAnonymousClass, params []string, work *workbudget.Budget) ([]nativeAnonymousInitializer, bool) {
	if obj == nil || child == nil || start < 0 || start >= len(ops) {
		return nil, false
	}
	if len(ops) == start+1 && ops[start] != nil && ops[start].Instr != nil && ops[start].Instr.OpCode == core.OP_RETURN && len(ops[start].Data) == 0 {
		return nil, true
	}
	fields := map[string]*MemberInfo{}
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		name, known := sourceBridgeUTF8(obj, f.NameIndex)
		if !known || fields[name] != nil {
			return nil, false
		}
		// A source constant declaration is itself an initialization event; it
		// cannot be interleaved with this block under the current capability.
		if f.AccessFlags&0x0008 == 0 && fieldHasConstantValue(f) {
			return nil, false
		}
		fields[name] = f
	}
	used := map[string]bool{}
	result := []nativeAnonymousInitializer{}
	for i := start; i < len(ops); {
		if !nativeProofWork(work, 1) || ops[i] == nil || ops[i].Instr == nil {
			return nil, false
		}
		if ops[i].Instr.OpCode == core.OP_RETURN {
			return result, i == len(ops)-1 && len(ops[i].Data) == 0
		}
		if !constructorMotionLoad(ops[i], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[i]) != 0 || i+2 >= len(ops) {
			return nil, false
		}
		if ops[i+1] == nil || ops[i+1].Instr == nil {
			return nil, false
		}
		rhs := i + 1
		capture := ""
		descriptor := ""
		var literal *values.JavaLiteral
		if kind, known := constructorMotionLiteral(obj, ops[rhs]); known {
			descriptor = kind
			literal = nativeAnonymousInitializerLiteral(obj, ops[rhs])
			if literal == nil {
				return nil, false
			}
			rhs++
		} else if core.GetRetrieveIdx(ops[rhs]) == 0 && constructorMotionLoad(ops[rhs], "Ljava/lang/Object;") && rhs+1 < len(ops) {
			read := constructorMotionMember(obj, ops[rhs+1], core.OP_GETFIELD)
			if read == nil || read.Name != obj.GetClassName() {
				return nil, false
			}
			index, known := child.fields[read.Member]
			if !known || index < 0 || index >= len(params) || read.Description != params[index] {
				return nil, false
			}
			capture = read.Member
			descriptor = read.Description
			rhs += 2
		} else {
			return nil, false
		}
		if rhs >= len(ops) {
			return nil, false
		}
		store := constructorMotionMember(obj, ops[rhs], core.OP_PUTFIELD)
		if store == nil || store.Name != obj.GetClassName() {
			return nil, false
		}
		field := fields[store.Member]
		if field == nil || field.AccessFlags&(0x0008|0x1000) != 0 || class_context.SafeIdentifier(store.Member) != store.Member {
			return nil, false
		}
		if _, captured := child.fields[store.Member]; captured {
			return nil, false
		}
		declared, known := sourceBridgeUTF8(obj, field.DescriptorIndex)
		if !known || declared != store.Description || used[store.Member] {
			return nil, false
		}
		if kinds, _, err := callbinding.Descriptor("(" + declared + ")V"); err != nil || len(kinds) != 1 {
			return nil, false
		}
		if descriptor != declared && !(descriptor == "null" && callbinding.Reference(declared)) && !(descriptor == "I" && strings.Contains("ZBCSI", declared) && len(declared) == 1) {
			return nil, false
		}
		if literal != nil {
			literal = nativeAnonymousInitializerTypedLiteral(literal, declared, descriptor)
			if literal == nil {
				return nil, false
			}
		}
		used[store.Member] = true
		result = append(result, nativeAnonymousInitializer{field: store.Member, descriptor: declared, capture: capture, literal: literal})
		i = rhs + 1
	}
	return nil, false
}

func nativeAnonymousInitializerLiteral(obj *ClassObject, op *core.OpCode) *values.JavaLiteral {
	code := op.Instr.OpCode
	var value any
	kind := ""
	switch {
	case code == core.OP_ACONST_NULL:
		value = "null"
		kind = "Ljava/lang/Object;"
	case code >= core.OP_ICONST_M1 && code <= core.OP_ICONST_5:
		value = code - core.OP_ICONST_0
		kind = "I"
	case code == core.OP_BIPUSH:
		value = int(int8(op.Data[0]))
		kind = "I"
	case code == core.OP_SIPUSH:
		value = int(int16(core.Convert2bytesToInt(op.Data)))
		kind = "I"
	case code >= core.OP_LCONST_0 && code <= core.OP_LCONST_1:
		value = int64(code - core.OP_LCONST_0)
		kind = "J"
	case code >= core.OP_FCONST_0 && code <= core.OP_FCONST_2:
		value = float32(code - core.OP_FCONST_0)
		kind = "F"
	case code >= core.OP_DCONST_0 && code <= core.OP_DCONST_1:
		value = float64(code - core.OP_DCONST_0)
		kind = "D"
	case code == core.OP_LDC || code == core.OP_LDC_W || code == core.OP_LDC2_W:
		index := int(op.Data[0])
		if code != core.OP_LDC {
			index = int(core.Convert2bytesToInt(op.Data))
		}
		literal, known := GetLiteralFromCP(obj.ConstantPool, index).(*values.JavaLiteral)
		if !known {
			return nil
		}
		return literal
	default:
		return nil
	}
	typ, err := types.ParseDescriptor(kind)
	if err != nil {
		return nil
	}
	return values.NewJavaLiteral(value, typ)
}

func nativeAnonymousInitializerTypedLiteral(literal *values.JavaLiteral, desc, kind string) *values.JavaLiteral {
	copy := *literal
	if kind == "null" {
		return &copy
	}
	typ, err := types.ParseDescriptor(desc)
	if err != nil {
		return nil
	}
	copy.JavaType = typ
	if v, ok := literal.Data.(float32); ok && math.IsNaN(float64(v)) && math.Float32bits(v) != 0x7fc00000 {
		return nil
	}
	if v, ok := literal.Data.(float64); ok && math.IsNaN(v) && math.Float64bits(v) != 0x7ff8000000000000 {
		return nil
	}
	v, ok := literal.Data.(int)
	if w, isWord := literal.Data.(int32); isWord {
		v, ok = int(w), true
	}
	if ok {
		switch desc {
		case "Z":
			copy.Data = v & 1
		case "B":
			copy.Data = int(int8(v))
		case "S":
			copy.Data = int(int16(v))
		case "C":
			copy.Data = int(uint16(v))
		}
	}
	return &copy
}

func nativeAnonymousInitializerSource(child *nativeAnonymousClass, bindings map[string]string, ctx *class_context.ClassContext, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) (string, bool) {
	if child == nil || ctx == nil {
		return "", false
	}
	if len(child.initializers) == 0 {
		return "", true
	}
	var source strings.Builder
	var fieldNames map[string]bool
	source.WriteString("{\n")
	for _, init := range child.initializers {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		rhs := ""
		if init.literal != nil {
			rhs = init.literal.String(ctx)
		} else {
			rhs = bindings[init.capture]
			if rhs == "" {
				return "", false
			}
			if fieldNames == nil {
				var known bool
				fieldNames, known = nativeAnonymousInitializerFieldNames(child.object, resolve, work)
				if !known {
					return "", false
				}
			}
			if fieldNames[rhs] {
				return "", false
			}
		}
		if rhs == "" {
			return "", false
		}
		source.WriteString("this." + init.field + " = " + rhs + ";\n")
	}
	source.WriteString("}\n")
	return source.String(), true
}

func nativeAnonymousInitializerStack(child *nativeAnonymousClass, params []string, code *CodeAttribute) bool {
	if child == nil || code == nil {
		return false
	}
	locals, maximum := 1, 1
	for _, param := range params {
		width := constructorEffectType(param).width()
		if width == 0 {
			return false
		}
		locals += width
		if 1+width > maximum {
			maximum = 1 + width
		}
	}
	ps, ret, err := callbinding.Descriptor(child.superDescriptor)
	if err != nil || ret != "V" {
		return false
	}
	words := 1
	for _, param := range ps {
		words += constructorEffectType(param).width()
	}
	if words > maximum {
		maximum = words
	}
	for _, init := range child.initializers {
		words = 1 + constructorEffectType(init.descriptor).width()
		if words > maximum {
			maximum = words
		}
	}
	return int(code.MaxLocals) >= locals && int(code.MaxStack) >= maximum
}

// An unqualified captured local can bind to an own or inherited field instead.
// Require the complete original field graph; missing metadata cannot prove a
// free lexical name. Conservatively include private fields as well.
func nativeAnonymousInitializerFieldNames(root *ClassObject, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) (map[string]bool, bool) {
	if root == nil || resolve == nil {
		return nil, false
	}
	names, active, seen := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var visit func(*ClassObject, int) bool
	visit = func(obj *ClassObject, depth int) bool {
		if obj == nil || depth > 64 || !nativeProofWork(work, 1) {
			return false
		}
		owner := obj.GetClassName()
		if active[owner] {
			return false
		}
		if seen[owner] {
			return true
		}
		if len(seen)+len(active) >= 128 {
			return false
		}
		active[owner] = true
		for _, field := range obj.Fields {
			if field == nil || !nativeProofWork(work, 1) {
				return false
			}
			name, known := sourceBridgeUTF8(obj, field.NameIndex)
			if !known {
				return false
			}
			names[class_context.SafeIdentifier(name)] = true
		}
		parents := append([]string{}, obj.GetInterfacesName()...)
		if parent := obj.GetSupperClassName(); parent != "" {
			parents = append(parents, parent)
		}
		for _, name := range parents {
			parent, known := resolve(name)
			if !known || parent == nil || parent.GetClassName() != name || !visit(parent, depth+1) {
				return false
			}
		}
		delete(active, owner)
		seen[owner] = true
		return true
	}
	return names, visit(root, 0)
}
