package javaclassparser

import (
	"bytes"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Receiver type views require the complete original instruction stream,
// including the loads and aliases that produced THIS. Matching only the cast
// would let a rewritten operand or omitted branch manufacture a proof. This
// certificate is local to one body analysis and spends its existing chain
// budget; it grants neither a subtype fact nor a constant instanceof result.
func (c *ClassObjectDumper) constructorReceiverTypeBodyOriginal(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, remaining *int) bool {
	if c == nil || obj == nil || code == nil || len(code.Code) == 0 || len(code.Code) > 65535 || remaining == nil || *remaining <= 0 || len(ops) >= *remaining {
		return false
	}
	*remaining -= len(ops) + 1
	if !nativeProofWork(c.Work, int64(len(code.Code)+len(ops))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(code.Code))*8+int64(len(ops))*96) != nil {
		return false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return false
	}
	original := constructorMotionOps(decoder)
	if len(original) != len(ops) {
		return false
	}
	for i, op := range ops {
		physical := original[i]
		if op == nil || op.Instr == nil || physical == nil || physical.Instr == nil || op.CurrentOffset != physical.CurrentOffset || op.IsWide != physical.IsWide || op.Instr.OpCode != physical.Instr.OpCode || !bytes.Equal(op.Data, physical.Data) {
			return false
		}
	}
	return true
}
