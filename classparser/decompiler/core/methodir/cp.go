package methodir

import (
	"encoding/binary"
	"math"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func decodeInvokeDynamicCP(d *core.Decompiler, index int) (name, descriptor string) {
	if d == nil || index <= 0 || d.ConstantPoolInvokeDynamicInfo == nil {
		return "", ""
	}
	defer func() {
		if recover() != nil {
			name, descriptor = "", ""
		}
	}()
	_, name, descriptor = d.ConstantPoolInvokeDynamicInfo(index)
	return name, descriptor
}

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

// The active printer separates literal constants from member metadata. LDC
// must use that same original literal-pool channel; the member getter does not
// accept numeric/String entries. Only literal/class witnesses are loadable in
// this subset. A dynamic value's result type is not evidence of a class literal.
func decodeLiteralCP(d *core.Decompiler, index int) (constant Const) {
	if d == nil || index <= 0 {
		return Const{}
	}
	defer func() { _ = recover() }()
	var value values.JavaValue
	if d.ConstantPoolLiteralGetter != nil {
		value = d.ConstantPoolLiteralGetter(index)
	} else {
		// Standalone MethodIR builders may supply a unified original pool.
		value = d.GetValueFromPool(index)
	}
	switch value := value.(type) {
	case *values.JavaLiteral:
		return literalConst(value)
	case *values.JavaClassValue:
		if value != nil && value.JavaType != nil {
			if name := typeInternal(value.JavaType); name != "" {
				return Const{Kind: ConstClass, Class: name}
			}
		}
	}
	return Const{}
}

func literalConst(t *values.JavaLiteral) Const {
	if t == nil || t.JavaType == nil {
		return Const{}
	}
	category, ok := t.JavaType.RawType().(*types.JavaPrimer)
	if !ok || category == nil {
		return Const{}
	}
	// Declared CP kind determines computational width. Go payload convenience
	// types do not: an int payload tagged long remains category two, while a
	// malformed int/float payload must not silently change its JVM category.
	switch category.Name {
	case types.JavaInteger:
		if n, ok := asInt64(t.Data); ok && n >= math.MinInt32 && n <= math.MaxInt32 {
			return Const{Kind: ConstInt, Int: int32(n)}
		}
	case types.JavaLong:
		if n, ok := asInt64(t.Data); ok {
			return Const{Kind: ConstLong, Long: n}
		}
	case types.JavaFloat:
		if f, ok := t.Data.(float32); ok {
			return Const{Kind: ConstFloat, FloatBits: math.Float32bits(f)}
		}
	case types.JavaDouble:
		if f, ok := t.Data.(float64); ok {
			return Const{Kind: ConstDouble, DoubleBits: math.Float64bits(f)}
		}
	case types.JavaString:
		if text, ok := t.Data.(string); ok {
			return Const{Kind: ConstString, String: text}
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
	return classConstantIdentity(t, 0)
}

func classConstantIdentity(t types.JavaType, depth int) string {
	if t == nil || depth > 8 {
		return ""
	}
	// Source display names omit java.lang/imported packages and cannot identify
	// classfile types. Read original class identity and array descriptors instead.
	switch raw := t.RawType().(type) {
	case *types.JavaClass:
		if raw != nil {
			return slashName(raw.Name)
		}
	case *types.JavaArrayType:
		if raw == nil || raw.Dimension <= 0 || raw.Dimension > 255 || raw.JavaType == nil {
			return ""
		}
		element := classConstantIdentity(raw.JavaType, depth+1)
		if element != "" {
			if !strings.HasPrefix(element, "[") {
				element = "L" + element + ";"
			}
		} else if primitive, ok := raw.JavaType.RawType().(*types.JavaPrimer); ok && primitive != nil {
			switch primitive.Name {
			case types.JavaBoolean:
				element = "Z"
			case types.JavaByte:
				element = "B"
			case types.JavaChar:
				element = "C"
			case types.JavaShort:
				element = "S"
			case types.JavaInteger:
				element = "I"
			case types.JavaLong:
				element = "J"
			case types.JavaFloat:
				element = "F"
			case types.JavaDouble:
				element = "D"
			}
		}
		if element == "" || strings.Count(element, "[")+raw.Dimension > 255 {
			return ""
		}
		return strings.Repeat("[", raw.Dimension) + element
	}
	return ""
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
