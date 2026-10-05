package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func nativeLegacyForestCode(obj *ClassObject, code *CodeAttribute, work *workbudget.Budget) bool {
	if obj == nil || code == nil || len(code.Code) > 65535 || !nativeProofWork(work, int64(len(code.Code))) || work != nil && work.CheckAlloc(int64(len(code.Code))*512+256) != nil {
		return false
	}
	d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	d.Work = work
	if d.ParseOpcode() != nil {
		return false
	}
	for _, op := range d.Opcodes() {
		if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
			return false
		}
		switch op.Instr.OpCode {
		case core.OP_INVOKEDYNAMIC:
			if obj.MajorVersion < 51 {
				return false
			}
		case core.OP_INVOKESTATIC, core.OP_INVOKESPECIAL:
			if len(op.Data) != 2 {
				return false
			}
			index := int(binary.BigEndian.Uint16(op.Data))
			if index < 1 || index > len(obj.ConstantPool) {
				return false
			}
			m, valid := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
			if !valid || m == nil {
				return false
			}
		}
	}
	return true
}
