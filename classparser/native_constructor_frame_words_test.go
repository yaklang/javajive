package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Even legal JVM int words need not be denotable as source narrow arguments.
// The external SDK producer forces the frame path, so the small packet proof
// cannot accidentally conceal a missing source-word admission guard.
func TestNativeConstructorFrameWordsKeepNarrowSourceArguments(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"FrameWordOwner.java": `
class FrameWordParent {FrameWordParent(Class type,boolean z,byte b,char c,short s){}}
class FrameWordOwner {
 class Literal extends FrameWordParent {Literal(java.lang.reflect.Field f){super(f.getType(),true,(byte)1,(char)1,(short)1);}}
 class Parameters extends FrameWordParent {Parameters(java.lang.reflect.Field f,boolean z,byte b,char c,short s){super(f.getType(),z,b,c,s);}}
 class Conversions extends FrameWordParent {Conversions(java.lang.reflect.Field f,int n){super(f.getType(),n!=0,(byte)n,(char)n,(short)n);}}
}`}, "none", "8")
	for _, variant := range []string{"literal", "parameters", "conversions", "boolean false", "boolean two", "boolean minus one", "byte two", "char two", "short two", "byte minus one", "char minus one", "short minus one"} {
		t.Run(variant, func(t *testing.T) {
			kind := "Literal"
			if variant == "parameters" {
				kind = "Parameters"
			} else if variant == "conversions" {
				kind = "Conversions"
			}
			obj, err := Parse(append([]byte(nil), files["FrameWordOwner$"+kind+".class"]...))
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name != "<init>" {
					continue
				}
				method = m
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			if code == nil {
				t.Fatal("missing original Code")
			}
			decoder := func() []*core.OpCode {
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if err := d.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				return constructorMotionOps(d)
			}
			ops := decoder()
			want := true
			if kind == "Literal" && variant != "literal" {
				which, opcode := 0, core.OP_ICONST_2
				switch variant {
				case "boolean false":
					opcode = core.OP_ICONST_0
				case "boolean minus one":
					opcode = core.OP_ICONST_M1
				case "byte two", "byte minus one":
					which = 1
				case "char two", "char minus one":
					which = 2
				case "short two", "short minus one":
					which = 3
				}
				if variant == "byte minus one" || variant == "char minus one" || variant == "short minus one" {
					opcode = core.OP_ICONST_M1
				}
				count := 0
				for _, op := range ops {
					if op.Instr.OpCode == core.OP_ICONST_1 {
						if count == which {
							code.Code[op.CurrentOffset] = byte(opcode)
							break
						}
						count++
					}
				}
				ops = decoder()
				want = variant != "boolean two" && variant != "boolean minus one" && variant != "char minus one"
			}
			next, call := nativeMemberFrameDelegation(obj, method, code, ops, 3, nil)
			if (next > 0 && call != nil) != want {
				t.Fatalf("source narrow admission=%v wanted=%v", next > 0 && call != nil, want)
			}
		})
	}
}
