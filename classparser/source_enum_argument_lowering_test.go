package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestAdversarialEnumArgumentCutRejectsChangedOriginsAndResidualOperands(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	old := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, enumArgumentHelperFixture, "ArgumentOwner", "none", false)
	fresh := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, enumArgumentHelperFixture, "ArgumentOwner", "none", false)
	const owner = "ArgumentOwner$Mode"
	for _, variant := range []string{"control", "expanded argument", "hidden name", "hidden ordinal", "wrong constant store", "lost receiver", "residual operand", "wide stack capacity", "handler", "changed source tuple"} {
		t.Run(variant, func(t *testing.T) {
			original, compiled := bytes.Clone(old[owner+".class"]), bytes.Clone(fresh[owner+".class"])
			object := mustParseLowering(t, compiled)
			var initializer *MemberInfo
			for _, method := range object.Methods {
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				if name == "<clinit>" {
					initializer = method
				}
			}
			code := constructorLoweringCode(initializer)
			ops, known := nativeEnumMethodOps(object, initializer, nil)
			if !known || len(ops) < 10 || !nativeEnumOpcode(ops[0], core.OP_NEW) || !nativeEnumOpcode(ops[3], core.OP_ICONST_0) {
				t.Fatal("authored independent enum argument control")
			}
			stores := []*core.OpCode{}
			callPC := -1
			for _, op := range ops {
				if m := constructorMotionMember(object, op, core.OP_PUTSTATIC); m != nil && m.Name == owner && m.Description == "L"+owner+";" {
					stores = append(stores, op)
				}
				if m := constructorMotionMember(object, op, core.OP_INVOKESTATIC); m != nil && m.Name == owner && m.Description == "()[L"+owner+";" {
					callPC = int(op.CurrentOffset)
				}
			}
			if len(stores) != 2 || callPC < 0 {
				t.Fatal("independent complete constant stores and array call")
			}
			switch variant {
			case "expanded argument":
				// A different fresh integer encoding changes every later PC.
				// Inlining must retain these bytes, not import original Code.
				at := int(ops[4].CurrentOffset)
				if !nativeEnumOpcode(ops[4], core.OP_ICONST_3) {
					t.Fatal("authored compact source argument")
				}
				code.Code = append(append(bytes.Clone(code.Code[:at]), 0x11, 0, 3), code.Code[at+1:]...)
			case "hidden name":
				index, _ := nativeEnumCPIndex(ops[2])
				object.ConstantPool[index-1].(*ConstantStringInfo).StringIndex = uint16(object.ConstantPoolManager.AddUtf8Info("OTHER"))
			case "hidden ordinal":
				code.Code[int(ops[3].CurrentOffset)] = 0x04
			case "wrong constant store":
				first, last := int(stores[0].CurrentOffset), int(stores[1].CurrentOffset)
				copy(code.Code[first+1:first+3], code.Code[last+1:last+3])
			case "lost receiver":
				code.Code[int(ops[1].CurrentOffset)] = 0x57
			case "residual operand":
				code.Code = append(append(bytes.Clone(code.Code[:callPC]), 0x03), code.Code[callPC:]...)
			case "wide stack capacity":
				code.MaxStack = 4
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 3, HandlerPc: 3}}
			}
			code.AttrLen = uint32(12 + len(code.Code) + 8*len(code.ExceptionTable))
			if len(code.Attributes) != 0 {
				t.Fatal("debug-free authored argument fixture")
			}
			input := object.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("mutation did not reach its semantic proof", err)
			}
			origin := constructorNativeSourceOrigin{sha256.Sum256(original), sha256.Sum256([]byte("exact returned owning source"))}
			if variant == "changed source tuple" {
				origin.originalHash = sha256.Sum256([]byte("another physical original"))
			}
			authority := &sourceConstructorLoweringAuthority{enumActive: map[string]constructorNativeSourceOrigin{owner: origin}}
			before := bytes.Clone(input)
			got, err := authority.lowerEnumArrayHelper(owner, original, input, nil)
			if variant == "control" || variant == "expanded argument" {
				if err != nil || bytes.Equal(got, input) {
					t.Fatal("typed argument packet was not lowered", err)
				}
				if variant == "expanded argument" {
					result := mustParseLowering(t, got)
					for _, method := range result.Methods {
						name, _ := sourceBridgeUTF8(result, method.NameIndex)
						if name == "<clinit>" && !bytes.Contains(constructorLoweringCode(method).Code, []byte{0x11, 0, 3}) {
							t.Fatal("fresh argument encoding was not retained")
						}
					}
				}
			} else if err == nil && !bytes.Equal(got, input) || err != nil && got != nil {
				t.Fatal("unproved argument packet was partially lowered", err)
			}
			if !bytes.Equal(input, before) || !bytes.Equal(original, old[owner+".class"]) {
				t.Fatal("executable lowering mutated its inputs")
			}
		})
	}
}
