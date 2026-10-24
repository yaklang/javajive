package javaclassparser

import (
	"encoding/binary"
	"math"
	"reflect"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Literal payloads are part of the ordered delegation tree, independent of CP
// indices and instruction widths. Preserve JVM category, floating bits and
// UTF-16 words; class/handle/dynamic constants need separate resolution proofs.
type constructorLoweringLiteral struct {
	word  uint64
	units []uint16
}

func constructorLoweringReadLiteral(object *ClassObject, op *core.OpCode, work *workbudget.Budget) (*constructorLoweringLiteral, string, bool) {
	if object == nil || op == nil || op.Instr == nil || op.IsWide || !nativeProofWork(work, 1) {
		return nil, "", false
	}
	n := op.Instr.OpCode
	v := &constructorLoweringLiteral{}
	desc := ""
	switch {
	case n >= core.OP_ICONST_M1 && n <= core.OP_ICONST_5 && len(op.Data) == 0:
		desc = "I"
		v.word = uint64(uint32(int32(n - core.OP_ICONST_0)))
	case n == core.OP_BIPUSH && len(op.Data) == 1:
		desc = "I"
		v.word = uint64(uint32(int32(int8(op.Data[0]))))
	case n == core.OP_SIPUSH && len(op.Data) == 2:
		desc = "I"
		v.word = uint64(uint32(int32(int16(binary.BigEndian.Uint16(op.Data)))))
	case n >= core.OP_LCONST_0 && n <= core.OP_LCONST_1 && len(op.Data) == 0:
		desc = "J"
		v.word = uint64(n - core.OP_LCONST_0)
	case n >= core.OP_FCONST_0 && n <= core.OP_FCONST_2 && len(op.Data) == 0:
		desc = "F"
		v.word = uint64(math.Float32bits(float32(n - core.OP_FCONST_0)))
	case n >= core.OP_DCONST_0 && n <= core.OP_DCONST_1 && len(op.Data) == 0:
		desc = "D"
		v.word = math.Float64bits(float64(n - core.OP_DCONST_0))
	default:
		index := 0
		if n == core.OP_LDC && len(op.Data) == 1 {
			index = int(op.Data[0])
		} else if (n == core.OP_LDC_W || n == core.OP_LDC2_W) && len(op.Data) == 2 {
			index = int(binary.BigEndian.Uint16(op.Data))
		} else {
			return nil, "", false
		}
		if index <= 0 || index > len(object.ConstantPool) {
			return nil, "", false
		}
		switch c := object.ConstantPool[index-1].(type) {
		case *ConstantIntegerInfo:
			if c == nil {
				return nil, "", false
			}
			desc = "I"
			v.word = uint64(uint32(c.Value))
		case *ConstantLongInfo:
			if c == nil {
				return nil, "", false
			}
			desc = "J"
			v.word = uint64(c.Value)
		case *ConstantFloatInfo:
			if c == nil {
				return nil, "", false
			}
			desc = "F"
			v.word = uint64(math.Float32bits(c.Value))
		case *ConstantDoubleInfo:
			if c == nil {
				return nil, "", false
			}
			desc = "D"
			v.word = math.Float64bits(c.Value)
		case *ConstantStringInfo:
			if c == nil || c.StringIndex == 0 || int(c.StringIndex) > len(object.ConstantPool) {
				return nil, "", false
			}
			u, ok := object.ConstantPool[c.StringIndex-1].(*ConstantUtf8Info)
			if !ok || u == nil || u.DecodeErr != nil || !nativeProofWork(work, int64(len(u.Value)+len(u.Units))+1) || work.CheckAlloc(int64(len(u.Value)+len(u.Units))*4+16) != nil {
				return nil, "", false
			}
			desc = "Ljava/lang/String;"
			v.units = append([]uint16{}, u.CodeUnits()...)
		default:
			return nil, "", false
		}
		if (n == core.OP_LDC2_W) != (desc == "J" || desc == "D") {
			return nil, "", false
		}
	}
	return v, desc, true
}

func constructorLoweringSourceLiteral(argument constructorLoweringArgument, value values.JavaValue) bool {
	x, ok := value.(*values.JavaLiteral)
	if !ok || x == nil || x.Type() == nil || argument.literal == nil || argument.null || len(argument.nullCasts) != 0 || argument.invocation != nil || argument.slot != 0 || argument.owner != "" || argument.name != "" {
		return false
	}
	want := argument.literal
	if argument.descriptor == "Ljava/lang/String;" {
		name, known := types.RawClassFQN(x.Type())
		p, primer := x.Type().RawType().(*types.JavaPrimer)
		if !(known && name == "java.lang.String" || primer && p != nil && p.Name == types.JavaString) || want.word != 0 {
			return false
		}
		if x.Units != nil {
			return reflect.DeepEqual(want.units, x.Units)
		}
		text, ok := x.Data.(string)
		return ok && utf8.ValidString(text) && reflect.DeepEqual(want.units, append([]uint16{}, utf16.Encode([]rune(text))...))
	}
	if x.Units != nil || want.units != nil {
		return false
	}
	p, ok := x.Type().RawType().(*types.JavaPrimer)
	if !ok || p == nil {
		return false
	}
	var word uint64
	switch argument.descriptor {
	case "I":
		if p.Name != types.JavaInteger {
			return false
		}
		switch data := x.Data.(type) {
		case int:
			if int64(data) < math.MinInt32 || int64(data) > math.MaxInt32 {
				return false
			}
			word = uint64(uint32(int32(data)))
		case int32:
			word = uint64(uint32(data))
		default:
			return false
		}
	case "J":
		data, ok := x.Data.(int64)
		if !ok || p.Name != types.JavaLong {
			return false
		}
		word = uint64(data)
	case "F":
		data, ok := x.Data.(float32)
		if !ok || p.Name != types.JavaFloat {
			return false
		}
		word = uint64(math.Float32bits(data))
	case "D":
		data, ok := x.Data.(float64)
		if !ok || p.Name != types.JavaDouble {
			return false
		}
		word = math.Float64bits(data)
	default:
		return false
	}
	return word == want.word
}
