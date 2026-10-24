package javaclassparser

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestAdversarialConstructorLoweringNullRequiresExactOriginalOperand(t *testing.T) {
	for _, variant := range []string{"control", "different pc", "manufactured null", "copied literal", "changed data", "changed source type", "parameter role", "field role", "primitive port"} {
		t.Run(variant, func(t *testing.T) {
			arg := constructorLoweringArgument{null: true, descriptor: "Ljava/lang/Object;"}
			literal := values.NewOriginalNullLiteral(7)
			switch variant {
			case "different pc":
				literal = values.NewOriginalNullLiteral(9)
			case "manufactured null":
				literal = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			case "copied literal":
				copy := *literal
				literal = &copy
			case "changed data":
				literal.Data = "fake"
			case "changed source type":
				literal.JavaType = types.NewJavaClass("java.lang.String")
			case "parameter role":
				arg.slot = 1
			case "field role":
				arg.owner = "Other"
			case "primitive port":
				arg.descriptor = "I"
			}
			if constructorLoweringSourceArgument(arg, 7, literal, nil, nil) != (variant == "control") {
				t.Fatal("null literal lost original producer proof")
			}
		})
	}
}

func TestAdversarialConstructorLoweringNullCastChainRetainsOriginalInstructions(t *testing.T) {
	for _, variant := range []string{"one cast", "two casts", "wrong origin", "wrong target", "missing inner cast", "reordered casts", "synthetic cast", "binding cast", "copied unchanged cast", "copied changed leaf", "missing null origin", "wrong leaf", "wrong final descriptor"} {
		t.Run(variant, func(t *testing.T) {
			arg := constructorLoweringArgument{null: true, descriptor: "Ljava/lang/String;", nullCasts: []string{"Ljava/lang/String;"}}
			var leaf values.JavaValue = values.NewOriginalNullLiteral(7)
			if variant == "missing null origin" {
				leaf = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			}
			if variant == "wrong leaf" {
				leaf = values.NewOriginalNullLiteral(8)
			}
			cast := values.NewOriginalCheckCast(leaf, types.NewJavaClass("java.lang.String"), 8)
			var value values.JavaValue = cast
			want := variant == "one cast" || variant == "two casts" || variant == "copied unchanged cast"
			switch variant {
			case "two casts", "missing inner cast", "reordered casts":
				arg.nullCasts = []string{"Ljava/lang/String;", "Ljava/lang/Object;"}
				arg.descriptor = "Ljava/lang/Object;"
				value = values.NewOriginalCheckCast(cast, types.NewJavaClass("java.lang.Object"), 11)
				if variant == "missing inner cast" {
					value = values.NewOriginalCheckCast(leaf, types.NewJavaClass("java.lang.Object"), 11)
				}
				if variant == "reordered casts" {
					value = values.NewOriginalCheckCast(values.NewOriginalCheckCast(leaf, types.NewJavaClass("java.lang.Object"), 8), types.NewJavaClass("java.lang.String"), 11)
				}
			case "wrong origin":
				value = values.NewOriginalCheckCast(leaf, types.NewJavaClass("java.lang.String"), 9)
			case "wrong target":
				cast.TargetType = types.NewJavaClass("java.lang.Integer")
			case "synthetic cast":
				value = &values.CastExpression{Value: leaf, TargetType: types.NewJavaClass("java.lang.String")}
			case "binding cast":
				cast.Binding = true
			case "copied unchanged cast", "copied changed leaf":
				// An immutable cast view may be copied; its producer is still the
				// identical original null. A newly manufactured leaf is not.
				copied := *cast
				if variant == "copied changed leaf" {
					copied.Value = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
				}
				value = &copied
			case "wrong final descriptor":
				arg.descriptor = "Ljava/lang/Object;"
			}
			if got := constructorLoweringSourceArgument(arg, 7, value, nil, nil); got != want {
				t.Fatalf("original null cast packet=%v want=%v", got, want)
			}
		})
	}
}

func TestAdversarialConstructorLoweringNullRejectsFreshOperandAndOverloadChanges(t *testing.T) {
	original, fresh, a := constructorSuffixRoundTrip(t, strings.ReplaceAll(constructorNullLoweringFixture, "NULLTYPE", "String"), "none", "SuffixOwner$Member", "72:constructor:null:binding:original:oracle\n")
	for _, variant := range []string{"control", "parameter instead of null", "primitive instead of null", "different selected constructor"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh))
			var packet *constructorLoweringPacket
			for _, m := range obj.Methods {
				if p := constructorLoweringPacketFor(obj, m, false, false, nil, 1); p != nil {
					if packet != nil {
						t.Fatal("multiple fresh packets")
					}
					packet = p
				}
			}
			if packet == nil {
				t.Fatal("fresh null packet missing")
			}
			nullPC := -1
			for _, op := range packet.ops {
				if op.Instr.OpCode == core.OP_ACONST_NULL {
					if nullPC >= 0 {
						t.Fatal("ambiguous null")
					}
					nullPC = int(op.CurrentOffset)
				}
			}
			if nullPC < 0 {
				t.Fatal("missing literal")
			}
			switch variant {
			case "parameter instead of null":
				packet.code.Code[nullPC] = core.OP_ALOAD_2
			case "primitive instead of null":
				packet.code.Code[nullPC] = core.OP_ICONST_0
			case "different selected constructor":
				pool := NewConstantPoolWithConstant(&obj.ConstantPool)
				for _, op := range packet.ops {
					if m := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL); m != nil && m.Name == "SuffixBase" {
						index := uint16(core.Convert2bytesToInt(op.Data))
						ref := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
						nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
						nt.DescriptorIndex = uint16(pool.AddUtf8Info("(Ljava/lang/Object;Ljava/lang/Integer;)V"))
					}
				}
			}
			input := obj.Bytes()
			saved := bytes.Clone(input)
			result, err := a.source.jar.sourceConstructorLowering.lower(original, input, nil)
			if !bytes.Equal(input, saved) {
				t.Fatal("input mutated")
			}
			if variant == "control" {
				if err != nil || len(result) == 0 {
					t.Fatal(err)
				}
			} else if err == nil || result != nil {
				t.Fatal("changed fresh initialization accepted", err)
			}
		})
	}
}
