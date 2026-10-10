package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

const nativeCompoundBooleanWordFixture = `class BitUpdateOwner{private boolean value;void reset(boolean n){value=n;}class Writer{boolean andWord(BitUpdateOwner owner){return owner.value&=true;}boolean orWord(BitUpdateOwner owner){return owner.value|=false;}boolean xorWord(BitUpdateOwner owner){return owner.value^=true;}boolean get(){return value;}}Writer writer(){return new Writer();}}
class BitUpdateDriver{public static void main(String[]args){BitUpdateOwner owner=new BitUpdateOwner();BitUpdateOwner.Writer writer=owner.writer();int rows=0;for(boolean n:new boolean[]{false,true}){owner.reset(n);if(writer.andWord(owner)!=n||writer.get()!=n)throw new AssertionError("AND low bit");owner.reset(n);if(writer.orWord(owner)!=n||writer.get()!=n)throw new AssertionError("OR low bit");owner.reset(n);if(writer.xorWord(owner)==n||writer.get()==n)throw new AssertionError("XOR low bit");rows+=3;}System.out.println(rows+":boolean:compound:word:bit:zero");}}`

func TestNativeCompoundBooleanNoncanonicalWordsRoundTrip(t *testing.T) {
	testNativePrivateSetterFixtureWithMutation(t, nativeCompoundBooleanWordFixture, "BitUpdateOwner", "BitUpdateDriver", "6:boolean:compound:word:bit:zero\n", func(t *testing.T, files map[string][]byte) {
		obj, e := Parse(files["BitUpdateOwner$Writer.class"])
		if e != nil {
			t.Fatal(e)
		}
		changed := 0
		for _, m := range obj.Methods {
			name, _ := sourceBridgeUTF8(obj, m.NameIndex)
			if name != "andWord" && name != "orWord" && name != "xorWord" {
				continue
			}
			for _, a := range m.Attributes {
				code, ok := a.(*CodeAttribute)
				if !ok {
					continue
				}
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if e := decoder.ParseOpcode(); e != nil {
					t.Fatal(e)
				}
				matches := 0
				want, to := core.OP_ICONST_1, core.OP_ICONST_3
				if name == "orWord" {
					want, to = core.OP_ICONST_0, core.OP_ICONST_2
				}
				for _, op := range decoder.Opcodes() {
					if op.Instr.OpCode == want {
						code.Code[int(op.CurrentOffset)] = byte(to)
						matches++
					}
				}
				if matches != 1 {
					t.Fatalf("original %s word count=%d", name, matches)
				}
				changed++
			}
		}
		if changed != 3 {
			t.Fatal("three independent Boolean word consumers required")
		}
		files["BitUpdateOwner$Writer.class"] = obj.Bytes()
	})
}
