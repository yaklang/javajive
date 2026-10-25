package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNumericJoinProducerRequiresOriginalTypedArrayRead(t *testing.T) {
	for _, name := range []string{"char", "short", "int", "ambiguous byte", "long", "reference", "wrong pc", "missing pc", "missing array", "missing index", "unproduced"} {
		t.Run(name, func(t *testing.T) {
			code := OP_CALOAD
			kind := types.JavaChar
			switch name {
			case "short":
				code = OP_SALOAD
				kind = types.JavaShort
			case "int":
				code = OP_IALOAD
				kind = types.JavaInteger
			case "ambiguous byte":
				code = OP_BALOAD
				kind = types.JavaByte
			case "long":
				code = OP_LALOAD
				kind = types.JavaLong
			case "reference":
				code = OP_AALOAD
			}
			array := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaPrimer(kind)))
			value := values.NewJavaArrayMember(array, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
			value.OriginPC = 3
			value.HasOriginPC = true
			read := op(code, 3)
			read.stackProduced = []values.JavaValue{value}
			switch name {
			case "wrong pc":
				value.OriginPC = 4
			case "missing pc":
				value.HasOriginPC = false
			case "missing array":
				value.Object = nil
			case "missing index":
				value.Index = nil
			case "unproduced":
				read.stackProduced = nil
			}
			d := &Decompiler{opCodes: []*OpCode{read}}
			got := d.originalNumericJoinProducers()[value]
			want := name == "char" || name == "short" || name == "int"
			if got != want {
				t.Fatalf("known=%v want=%v", got, want)
			}
		})
	}
}

func TestNumericJoinProducerRequiresOriginalFieldDescriptorAndRead(t *testing.T) {
	for _, static := range []bool{false, true} {
		for _, name := range []string{"C", "B", "S", "I", "Z", "J", "[I", "wrong pc", "missing pc", "no witness", "descriptor changed", "name changed", "write opcode", "no pool", "no operand"} {
			t.Run(name+map[bool]string{true: "/static", false: "/instance"}[static], func(t *testing.T) {
				desc := name
				if len(desc) > 2 {
					desc = "C"
				}
				cp := &values.JavaClassMember{Name: "Owner", Member: "word", Description: desc, JavaType: types.NewJavaPrimer(types.JavaChar)}
				code := OP_GETFIELD
				var value values.JavaValue
				if static {
					code = OP_GETSTATIC
					field := *cp
					field.OriginPC = 3
					field.HasOriginPC = true
					if name != "no witness" {
						field.MarkOriginalFieldRead(cp, 3)
					}
					value = &field
					if name == "wrong pc" {
						field.OriginPC = 4
					}
					if name == "missing pc" {
						field.HasOriginPC = false
					}
					if name == "name changed" {
						field.Member = "other"
					}
				} else {
					field := values.NewRefMember(values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner")), "word", cp.JavaType)
					field.OriginPC = 3
					field.HasOriginPC = true
					if name != "no witness" {
						field.MarkOriginalFieldRead(cp, 3)
					}
					value = field
					if name == "wrong pc" {
						field.OriginPC = 4
					}
					if name == "missing pc" {
						field.HasOriginPC = false
					}
					if name == "name changed" {
						field.Member = "other"
					}
				}
				if name == "descriptor changed" {
					cp.Description = "I"
				}
				if name == "write opcode" {
					code = OP_PUTFIELD
				}
				read := op(code, 3)
				read.Data = []byte{0, 1}
				read.stackProduced = []values.JavaValue{value}
				d := &Decompiler{opCodes: []*OpCode{read}, constantPoolGetter: func(int) values.JavaValue { return cp }}
				if name == "no pool" {
					d.constantPoolGetter = nil
				}
				if name == "no operand" {
					read.stackProduced = nil
				}
				got := d.originalNumericJoinProducers()[value]
				want := name == "B" || name == "C" || name == "S" || name == "I"
				if got != want {
					t.Fatalf("known=%v want=%v", got, want)
				}
			})
		}
	}
}
