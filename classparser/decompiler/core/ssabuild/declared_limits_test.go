package ssabuild

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"testing"
)

func TestDeclaredCodeLimitsSurviveIRAndConstrainSSA(t *testing.T) {
	code := []byte{core.OP_ICONST_1, core.OP_IRETURN}
	for _, tc := range []struct {
		name, desc    string
		locals, stack int
		bad           bool
	}{
		{"zero_stack", "()I", 0, 0, true}, {"valid", "()I", 0, 1, false}, {"unused_parameter_exceeds_locals", "(I)I", 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ir, e := methodir.BuildFromBytes(code, nil, methodir.MethodMeta{ClassName: "T", Name: "f", Descriptor: tc.desc, Bytecode: code, IsStatic: true, Limits: core.CodeLimits{Present: true, MaxLocals: tc.locals, MaxStack: tc.stack, DirectSuperClass: "java/lang/Object"}}, nil)
			if e != nil {
				t.Fatal(e)
			}
			_, e = Build(ir, Options{})
			if (e != nil) != tc.bad {
				t.Fatalf("bad=%v error=%v", tc.bad, e)
			}
			before := ir.Hash()
			ir.Limits.MaxStack++
			if before == ir.Hash() {
				t.Fatal("declared limits missing from snapshot identity")
			}
		})
	}
}
