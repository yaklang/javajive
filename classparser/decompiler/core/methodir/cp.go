package methodir

import (
	"encoding/binary"
	"math"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func decodeCP(d *core.Decompiler, index int) (class, member, desc string, cnst Const) {
	if d == nil || index <= 0 {
		return "", "", "", Const{}
	}
	defer func() { _ = recover() }()
	v := d.GetValueFromPool(index)
	if v == nil {
		return "", "", "", Const{}
	}
	switch t := v.(type) {
	case *values.JavaClassMember:
		return slashName(t.Name), t.Member, t.Description, Const{}
	case *values.JavaLiteral:
		return "", "", "", literalConst(t)
	case *values.JavaClassValue:
		name := typeInternal(t.JavaType)
		return name, "", "", Const{Kind: ConstClass, Class: name}
	case *types.JavaClass:
		name := slashName(t.Name)
		return name, "", "", Const{Kind: ConstClass, Class: name}
	default:
		if jt, ok := v.(interface{ Type() types.JavaType }); ok && jt.Type() != nil {
			name := typeInternal(jt.Type())
			if name != "" {
				return name, "", "", Const{Kind: ConstClass, Class: name}
			}
		}
	}
	return "", "", "", Const{}
}

func literalConst(t *values.JavaLiteral) Const {
	if t == nil {
		return Const{}
	}
	switch data := t.Data.(type) {
	case int:
		return Const{Kind: ConstInt, Int: int32(data)}
	case int32:
		return Const{Kind: ConstInt, Int: data}
	case int64:
		return Const{Kind: ConstLong, Long: data}
	case float32:
		return Const{Kind: ConstFloat, FloatBits: math.Float32bits(data)}
	case float64:
		return Const{Kind: ConstDouble, DoubleBits: math.Float64bits(data)}
	case string:
		return Const{Kind: ConstString, String: data}
	}
	if t.JavaType != nil {
		s := t.JavaType.String(&class_context.ClassContext{})
		switch s {
		case types.JavaLong:
			if n, ok := asInt64(t.Data); ok {
				return Const{Kind: ConstLong, Long: n}
			}
		case types.JavaDouble:
			if f, ok := t.Data.(float64); ok {
				return Const{Kind: ConstDouble, DoubleBits: math.Float64bits(f)}
			}
		case types.JavaFloat:
			if f, ok := t.Data.(float32); ok {
				return Const{Kind: ConstFloat, FloatBits: math.Float32bits(f)}
			}
		}
	}
	return Const{}
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	}
	return 0, false
}

func typeInternal(t types.JavaType) string {
	if t == nil {
		return ""
	}
	return slashName(t.String(&class_context.ClassContext{}))
}

func slashName(s string) string {
	b := make([]byte, len(s))
	copy(b, s)
	for i := range b {
		if b[i] == '.' {
			b[i] = '/'
		}
	}
	return string(b)
}

func cpIndex(op *core.OpCode) uint16 {
	if op == nil {
		return 0
	}
	switch op.Instr.OpCode {
	case core.OP_LDC:
		if len(op.Data) >= 1 {
			return uint16(op.Data[0])
		}
	default:
		if len(op.Data) >= 2 {
			return binary.BigEndian.Uint16(op.Data[:2])
		}
	}
	return 0
}
