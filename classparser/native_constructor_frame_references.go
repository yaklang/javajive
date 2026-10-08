package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

// The verifier join deliberately has no dependency hierarchy oracle. When it
// widens a reference phi to Object, a source assignment can still be proved by
// the exact declared descriptors of *all* original reaching producers. Reuse
// the constructor operand graph's shared work limit and cycle guard. These
// descriptors never select an overload or insert an operand conversion.
func (p *nativeConstructorFrameWords) reference(origin ssabuild.Origin, formal string, widening *constructorWideningQuery, frames *ssabuild.Function) bool {
	p.remaining--
	if p.remaining < 0 || p.active[origin] || widening == nil || !nativeProofWork(p.work, int64(len(formal)+1)) {
		return false
	}
	params, result, err := callbinding.Descriptor("(" + formal + ")V")
	if err != nil || result != "V" || len(params) != 1 || !callbinding.Reference(formal) {
		return false
	}
	p.active[origin] = true
	defer delete(p.active, origin)
	if origin.Kind == ssabuild.OriginParam {
		return callbinding.Reference(p.params[origin.Slot]) && widening.assignable(p.params[origin.Slot], formal)
	}
	if origin.Kind == ssabuild.OriginPhi {
		operands := p.phis[origin]
		if len(operands) == 0 {
			return false
		}
		for _, operand := range operands {
			if !p.reference(operand, formal, widening, frames) {
				return false
			}
		}
		return true
	}
	if origin.Kind != ssabuild.OriginInstr || origin.Slot != 0 || origin.Aux != 0 {
		return false
	}
	ins, known := p.ir.InstrByID(methodir.InstrID(origin.PC))
	if !known {
		return false
	}
	actual := ""
	switch ins.Opcode {
	case core.OP_ACONST_NULL:
		actual = "null"
	case core.OP_NEW, core.OP_CHECKCAST:
		if ins.Class != "" {
			actual = ins.Class
			if actual[0] != '[' {
				actual = "L" + actual + ";"
			}
		}
	case core.OP_ANEWARRAY:
		if ins.Class != "" {
			actual = ins.Class
			if actual[0] != '[' {
				actual = "L" + actual + ";"
			}
			actual = "[" + actual
		}
	case core.OP_GETFIELD, core.OP_GETSTATIC:
		actual = ins.Desc
	case core.OP_INVOKEVIRTUAL, core.OP_INVOKEINTERFACE, core.OP_INVOKESTATIC:
		_, ret, err := callbinding.Descriptor(ins.Desc)
		if err == nil {
			actual = ret
		}
	case core.OP_AALOAD:
		if frames == nil {
			return false
		}
		// Reading an element removes one array rank. Prove the array's
		// reaching declarations recursively rather than guessing the
		// component of a computational Object at a merged array input.
		var array ssabuild.Origin
		found := false
		for _, record := range frames.Instructions {
			if !nativeProofWork(p.work, 1) {
				return false
			}
			if record.PC == ins.PC {
				if found || len(record.Uses) != 2 {
					return false
				}
				found, array = true, record.Uses[0]
			}
		}
		return found && p.reference(array, "["+formal, widening, frames)
	case core.OP_LDC, core.OP_LDC_W:
		switch ins.Const.Kind {
		case methodir.ConstString:
			actual = "Ljava/lang/String;"
		case methodir.ConstClass:
			actual = "Ljava/lang/Class;"
		}
	}
	if actual == "null" {
		return true
	}
	params, result, err = callbinding.Descriptor("(" + actual + ")V")
	return err == nil && result == "V" && len(params) == 1 && callbinding.Reference(actual) && widening.assignable(actual, formal)
}
