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
