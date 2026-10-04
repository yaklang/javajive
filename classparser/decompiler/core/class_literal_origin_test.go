package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestClassLiteralLoadKeepsUseOriginWithoutChangingPoolSymbol(t *testing.T) {
	original := values.NewJavaClassValue(types.NewJavaClass("example.SharedLiteral"))
	var uses []*values.JavaClassValue
	for _, code := range [][]byte{{OP_NOP, OP_LDC, 1, OP_RETURN}, {OP_NOP, OP_NOP, OP_LDC_W, 0, 1, OP_RETURN}} {
		d := NewDecompiler(code, func(int) values.JavaValue { return original })
		d.ConstantPoolLiteralGetter = func(int) values.JavaValue { return original }
		if err := d.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		sim := NewStackSimulation(NewEmptyStackEntry(), map[int]*values.JavaRef{}, utils.NewRootVariableId())
		for _, op := range d.Opcodes() {
			if op.Instr.OpCode != OP_LDC && op.Instr.OpCode != OP_LDC_W {
				continue
			}
			if err := d.calcOpcodeStackInfo(sim, op); err != nil {
				t.Fatal(err)
			}
			v, ok := values.UnpackSoltValue(sim.Pop()).(*values.JavaClassValue)
			if !ok || v == original || !v.HasOriginPC || v.OriginPC != int(op.CurrentOffset) {
				t.Fatal("lost use-site class resolution identity")
			}
			uses = append(uses, v)
		}
	}
	if len(uses) != 2 || uses[0] == uses[1] || uses[0].OriginPC == uses[1].OriginPC || original.HasOriginPC {
		t.Fatal("shared pool class acquired one use's origin")
	}
}
