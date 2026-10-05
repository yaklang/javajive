package methodir_test

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

// An indy call site has no Methodref owner. Its original NameAndType supplies
// stack categories even when the bootstrap's implementation is opaque. Never
// read it through the ordinary member getter or execute the bootstrap.
func TestInvokeDynamicOriginalDescriptorTypesImmutableOperands(t *testing.T) {
	for _, test := range []struct {
		name, descriptor, result string
		prefix                   []byte
		stack, arguments         int
	}{
		{"one-slot", "(I)Ljava/lang/Object;", "Ljava/lang/Object;", []byte{core.OP_ICONST_1}, 1, 1},
		{"wide-and-array", "(JD)[Ljava/lang/String;", "[Ljava/lang/String;", []byte{core.OP_LCONST_0, core.OP_DCONST_0}, 4, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			code := append(append([]byte(nil), test.prefix...), core.OP_INVOKEDYNAMIC, 0, 7, 0, 0, core.OP_ARETURN)
			d := core.NewDecompiler(code, func(int) values.JavaValue { t.Fatal("indy must not use a Methodref getter"); return nil })
			calls := 0
			d.ConstantPoolInvokeDynamicInfo = func(index int) (uint16, string, string) {
				calls++
				if index != 7 {
					t.Fatalf("original CP index=%d", index)
				}
				return 3, "factory", test.descriptor
			}
			ir, err := methodir.BuildFromBytes(code, nil, methodir.MethodMeta{ClassName: "CallSiteOwner", Name: "run", Descriptor: "()" + test.result, IsStatic: true, Bytecode: code, Limits: core.CodeLimits{Present: true, MaxStack: test.stack}}, d)
			if err != nil {
				t.Fatal(err)
			}
			pc := uint16(len(test.prefix))
			invoke, ok := ir.InstrByID(methodir.InstrID(pc))
			if !ok || invoke.Class != "" || invoke.Member != "factory" || invoke.Desc != test.descriptor || invoke.CPIndex != 7 || calls != 1 {
				t.Fatalf("original call-site identity:%+v calls=%d", invoke, calls)
			}
			before := ir.Canonical()
			fn, err := ssabuild.Build(ir, ssabuild.Options{MaxUpdates: 1000})
			if err != nil {
				t.Fatal(err)
			}
			if before != ir.Canonical() {
				t.Fatal("typed proof mutated MethodIR")
			}
			found := false
			for _, record := range fn.Instructions {
				if record.PC == pc {
					found = true
					if len(record.Uses) != test.arguments || len(record.Results) != 1 || len(record.Before.Stack) != test.stack {
						t.Fatalf("computational argument categories:%+v", record)
					}
				}
			}
			if !found {
				t.Fatal("typed call-site snapshot absent")
			}
		})
	}
}

func TestInvokeDynamicUnclosedDescriptorCannotCertifyTypedFrames(t *testing.T) {
	for _, test := range []string{"no provider", "empty", "malformed", "panicking provider"} {
		t.Run(test, func(t *testing.T) {
			code := []byte{core.OP_ICONST_1, core.OP_INVOKEDYNAMIC, 0, 7, 0, 0, core.OP_ARETURN}
			d := core.NewDecompiler(code, func(int) values.JavaValue { return nil })
			if test != "no provider" {
				d.ConstantPoolInvokeDynamicInfo = func(int) (uint16, string, string) {
					if test == "panicking provider" {
						panic("unavailable original metadata")
					}
					if test == "malformed" {
						return 3, "factory", "I"
					}
					return 3, "factory", ""
				}
			}
			ir, err := methodir.BuildFromBytes(code, nil, methodir.MethodMeta{ClassName: "CallSiteOwner", Name: "run", Descriptor: "()Ljava/lang/Object;", IsStatic: true, Bytecode: code, Limits: core.CodeLimits{Present: true, MaxStack: 1}}, d)
			if err != nil {
				return
			}
			if _, err := ssabuild.Build(ir, ssabuild.Options{MaxUpdates: 1000}); err == nil {
				t.Fatal("unknown original stack effect certified")
			}
		})
	}
}
