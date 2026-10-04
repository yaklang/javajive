package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNativeAnonymousInitializerArrayAllocationRequiresOriginalTypeAndShape(t *testing.T) {
	// Standard primitive array tags have one meaning each. Source rank and
	// element identity must match the original opcode, independent of spelling.
	for _, tag := range []byte{4, 5, 6, 7, 8, 9, 10, 11} {
		t.Run(types.GetPrimerArrayType(int(tag)).String(nil), func(t *testing.T) {
			for _, variant := range []string{"original", "wrong element", "wrong rank", "no length", "extra length", "initializer", "constructor", "opaque arguments", "bad tag", "truncated opcode"} {
				t.Run(variant, func(t *testing.T) {
					op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_NEWARRAY}, Data: []byte{tag}}
					typ := types.NewJavaArrayType(types.GetPrimerArrayType(int(tag)))
					n := values.NewNewArrayExpression(typ, values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger)))
					switch variant {
					case "wrong element":
						next := byte(4 + (int(tag)-4+1)%8)
						n.JavaType = types.NewJavaArrayType(types.GetPrimerArrayType(int(next)))
					case "wrong rank":
						n.JavaType = types.NewJavaArrayType(typ)
					case "no length":
						n.Length = nil
					case "extra length":
						n.Length = append(n.Length, n.Length[0])
					case "initializer":
						n.Initializer = []values.JavaValue{n.Length[0]}
					case "constructor":
						n.ConstructorCall = &values.FunctionCallExpression{}
					case "opaque arguments":
						n.ArgumentsGetter = func() string { panic("must not execute opaque text") }
					case "bad tag":
						op.Data[0] = 0
					case "truncated opcode":
						op.Data = nil
					}
					if known := nativeAnonymousInitializerArrayAllocation(nil, op, n); known != (variant == "original") {
						t.Fatalf("accepted=%v", known)
					}
				})
			}
		})
	}
}
func TestNativeAnonymousInitializerArrayTypeKeepsBinaryIdentityAndRank(t *testing.T) {
	for _, variant := range []string{"original", "slash spelling", "different package", "different rank", "different element", "missing type", "scalar", "zero rank", "excess rank"} {
		t.Run(variant, func(t *testing.T) {
			expected := types.NewJavaArrayType(types.NewJavaClass("original/Node"))
			actual := expected.Copy()
			switch variant {
			case "slash spelling":
				actual = types.NewJavaArrayType(types.NewJavaClass("original.Node"))
			case "different package":
				actual = types.NewJavaArrayType(types.NewJavaClass("other/Node"))
			case "different rank":
				actual = types.NewJavaArrayType(expected)
			case "different element":
				actual = types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
			case "missing type":
				actual = nil
			case "scalar":
				actual = types.NewJavaClass("original/Node")
			case "zero rank":
				actual.RawType().(*types.JavaArrayType).Dimension = 0
			case "excess rank":
				actual.RawType().(*types.JavaArrayType).Dimension = 256
			}
			want := variant == "original" || variant == "slash spelling"
			if got := nativeAnonymousInitializerSameArrayType(actual, expected); got != want {
				t.Fatalf("identity match=%v", got)
			}
		})
	}
}

func TestNativeAnonymousInitializerMultiArrayRequiresOriginalDimensions(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousArrayInitializerFixture)
	for _, variant := range []string{"original", "partial dimensions", "zero dimensions", "too many dimensions", "missing length", "extra length", "rank mismatch", "wrong element", "truncated opcode", "nonarray class"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(append([]byte(nil), files["ArrayInitOwner$1.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			index := obj.ConstantPoolManager.AddNewClassInfo("[[[I")
			op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_MULTIANEWARRAY}, Data: []byte{byte(index >> 8), byte(index), 3}}
			typ, e := types.ParseDescriptor("[[[I")
			if e != nil {
				t.Fatal(e)
			}
			length := values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))
			value := values.NewNewArrayExpression(typ, length, length, length)
			switch variant {
			case "partial dimensions":
				op.Data[2] = 2
				value.Length = value.Length[:2]
			case "zero dimensions":
				op.Data[2] = 0
				value.Length = nil
			case "too many dimensions":
				op.Data[2] = 4
				value.Length = append(value.Length, length)
			case "missing length":
				value.Length = value.Length[:2]
			case "extra length":
				value.Length = append(value.Length, length)
			case "rank mismatch":
				value.JavaType = types.NewJavaArrayType(typ)
			case "wrong element":
				value.JavaType, _ = types.ParseDescriptor("[[[J")
			case "truncated opcode":
				op.Data = op.Data[:2]
			case "nonarray class":
				index = obj.ConstantPoolManager.AddNewClassInfo("java/lang/Object")
				op.Data[0] = byte(index >> 8)
				op.Data[1] = byte(index)
			}
			want := variant == "original" || variant == "partial dimensions"
			if got := nativeAnonymousInitializerArrayAllocation(obj, op, value); got != want {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
