package javaclassparser

import (
	"bytes"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func resultAdapterFixture(t *testing.T, files map[string][]byte, owner string) (*ClassObject, *MemberInfo, *CodeAttribute) {
	t.Helper()
	object, err := Parse(bytes.Clone(files[owner+".class"]))
	if err != nil {
		t.Fatal(err)
	}
	var implementation *MemberInfo
	var code *CodeAttribute
	for _, method := range object.Methods {
		if method.AccessFlags != 0x100a {
			continue
		}
		if implementation != nil {
			t.Fatal("ambiguous original authored adapter", owner)
		}
		implementation = method
		for _, a := range method.Attributes {
			if c, ok := a.(*CodeAttribute); ok {
				if code != nil {
					t.Fatal("duplicate original Code")
				}
				code = c
			}
		}
	}
	if implementation == nil || code == nil {
		t.Fatal("missing original authored adapter", owner)
	}
	return object, implementation, code
}

// These mutants retain the original factory/member schema, but change the
// actual executable result tail. Type compatibility alone must never grant
// ownership of that changed body to a compiler-generated method reference.
func TestNativeVarargsResultConversionRequiresOriginalExecutableTail(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required")
	}
	fixture := varargsResultBank + `
interface ResultByteShort {short apply(byte value);}
class ResultByteShortOwner {static ResultByteShort make(ResultKindsTarget target){return target::bytes;}}
`
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, fixture, "ResultKindsTarget", "none", true)
	owners := []string{"ResultBoolOwner", "ResultByteOwner", "ResultCharOwner", "ResultShortOwner", "ResultIntOwner", "ResultLongOwner", "ResultFloatOwner", "ResultDoubleOwner", "ResultIntLongOwner", "ResultIntDoubleOwner", "ResultLongDoubleOwner", "ResultFloatDoubleOwner", "ResultStringObjectOwner", "ResultArrayObjectOwner", "ResultIntNumberOwner", "ResultLongDiscardOwner", "ResultDoubleDiscardOwner", "ResultByteIntOwner", "ResultShortLongOwner", "ResultCharDoubleOwner", "ResultIntFloatOwner", "ResultLongFloatOwner", "ResultByteShortOwner"}
	for _, owner := range owners {
		t.Run("original/"+owner, func(t *testing.T) {
			object, method, code := resultAdapterFixture(t, files, owner)
			before := bytes.Clone(code.Code)
			d := NewClassObjectDumper(object)
			if _, known := d.originalVarargsAdapterShape(method); !known {
				t.Fatal("original compiler result conversion was not recognized", owner)
			}
			if !bytes.Equal(before, code.Code) {
				t.Fatal("original result proof mutated Code")
			}
		})
	}
	for _, owner := range owners[:8] {
		for _, kind := range []string{"missing boxing", "wrong invocation opcode", "wrong CP kind", "wrong boxing owner", "wrong boxing name", "wrong boxing descriptor", "widen before boxing", "extra executable tail"} {
			t.Run(owner+"/"+kind, func(t *testing.T) {
				object, method, code := resultAdapterFixture(t, files, owner)
				pc := -1
				// Bytecode parsing supplies instruction boundaries; scanning raw operand
				// bytes could accidentally mutate a CP index that resembles an opcode.
				parser := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
				if err := parser.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				for _, op := range parser.Opcodes() {
					if op == nil || op.Instr == nil {
						continue
					}
					m := constructorMotionMember(object, op, core.OP_INVOKESTATIC)
					if m != nil && m.Member == "valueOf" {
						if pc >= 0 {
							t.Fatal("ambiguous original boxing")
						}
						pc = int(op.CurrentOffset)
					}
				}
				if pc < 0 {
					t.Fatal("missing original boxing", owner)
				}
				index := core.Convert2bytesToInt(code.Code[pc+1 : pc+3])
				ref, ok := object.ConstantPool[index-1].(*ConstantMethodrefInfo)
				if !ok {
					t.Fatal("original boxing CP kind")
				}
				nameType, ok := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !ok {
					t.Fatal("original boxing NameAndType")
				}
				appendUTF := func(text string) uint16 {
					object.ConstantPool = append(object.ConstantPool, &ConstantUtf8Info{Value: text})
					return uint16(len(object.ConstantPool))
				}
				switch kind {
				case "missing boxing":
					code.Code = append(bytes.Clone(code.Code[:pc]), code.Code[pc+3:]...)
				case "wrong invocation opcode":
					code.Code[pc] = core.OP_INVOKEVIRTUAL
				case "wrong CP kind":
					object.ConstantPool[index-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
				case "wrong boxing owner":
					name := appendUTF("java/lang/String")
					object.ConstantPool = append(object.ConstantPool, &ConstantClassInfo{NameIndex: name})
					ref.ClassIndex = uint16(len(object.ConstantPool))
				case "wrong boxing name":
					nameType.NameIndex = appendUTF("differentValue")
				case "wrong boxing descriptor":
					nameType.DescriptorIndex = appendUTF("(Ljava/lang/Object;)Ljava/lang/Object;")
				case "widen before boxing":
					code.Code = append(append(bytes.Clone(code.Code[:pc]), core.OP_I2L), code.Code[pc:]...)
				case "extra executable tail":
					code.Code = append(code.Code, core.OP_ACONST_NULL, core.OP_ARETURN)
				}
				before := bytes.Clone(code.Code)
				d := NewClassObjectDumper(object)
				if _, known := d.originalVarargsAdapterShape(method); known {
					t.Fatal("changed result tail borrowed compiler ownership", kind)
				}
				if !bytes.Equal(before, code.Code) {
					t.Fatal("rejected proof mutated original Code")
				}
			})
		}
	}
	for _, owner := range []string{"ResultIntLongOwner", "ResultIntDoubleOwner", "ResultLongDoubleOwner", "ResultFloatDoubleOwner", "ResultByteIntOwner", "ResultShortLongOwner", "ResultCharDoubleOwner", "ResultIntFloatOwner", "ResultLongFloatOwner", "ResultLongDiscardOwner", "ResultDoubleDiscardOwner", "ResultByteShortOwner"} {
		t.Run(owner+"/changed executable conversion", func(t *testing.T) {
			object, method, code := resultAdapterFixture(t, files, owner)
			parser := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
			if err := parser.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			pc := -1
			for _, op := range parser.Opcodes() {
				if op == nil || op.Instr == nil {
					continue
				}
				m := constructorMotionMember(object, op, core.OP_INVOKEVIRTUAL)
				if m != nil && m.Name == "ResultKindsTarget" {
					pc = int(op.CurrentOffset) + 3
				}
			}
			if pc < 0 || pc >= len(code.Code) {
				t.Fatal("missing original target return", owner)
			}
			// Adding a real negation after invocation is observably different even
			// for an adapter whose small primitive widening needs no JVM opcode.
			code.Code = append(append(bytes.Clone(code.Code[:pc]), core.OP_INEG), code.Code[pc:]...)
			before := bytes.Clone(code.Code)
			d := NewClassObjectDumper(object)
			if _, known := d.originalVarargsAdapterShape(method); known {
				t.Fatal("extra result computation borrowed compiler ownership", owner)
			}
			if !bytes.Equal(before, code.Code) {
				t.Fatal("rejection mutated Code")
			}
		})
	}
}
