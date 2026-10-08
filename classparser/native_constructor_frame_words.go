package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Verifier int is a computational category, not evidence of a source boolean,
// byte, char or short. Retain exact narrow inputs and explicit truncations;
// otherwise every reaching word must be representable without a new conversion.
type nativeConstructorFrameWords struct {
	ir        *methodir.MethodIR
	params    map[int]string
	phis      map[ssabuild.Origin][]ssabuild.Origin
	active    map[ssabuild.Origin]bool
	remaining int
	work      *workbudget.Budget
}

func newNativeConstructorFrameWords(obj *ClassObject, method *MemberInfo, ir *methodir.MethodIR, frames *ssabuild.Function, work *workbudget.Budget) *nativeConstructorFrameWords {
	descriptor, known := sourceBridgeUTF8(obj, method.DescriptorIndex)
	params, result, err := callbinding.Descriptor(descriptor)
	if !known || err != nil || result != "V" || !nativeProofWork(work, int64(len(frames.Phis)+len(params))) || work != nil && work.CheckAlloc(int64(len(frames.Phis)+len(params)+1)*128) != nil {
		return nil
	}
	p := &nativeConstructorFrameWords{ir: ir, params: map[int]string{}, phis: map[ssabuild.Origin][]ssabuild.Origin{}, active: map[ssabuild.Origin]bool{}, remaining: 512, work: work}
	for slot, i := 1, 0; i < len(params); i++ {
		p.params[slot] = params[i]
		if params[i] == "J" || params[i] == "D" {
			slot += 2
		} else {
			slot++
		}
	}
	for _, phi := range frames.Phis {
		if int(phi.Block) >= len(frames.Blocks) {
			return nil
		}
		block := frames.Blocks[phi.Block]
		slot := phi.Slot.Index
		if !phi.Slot.Local {
			slot += len(block.In.Locals)
		}
		origin := ssabuild.Origin{Kind: ssabuild.OriginPhi, PC: block.First, Slot: slot, Aux: int(phi.Block)}
		if _, duplicate := p.phis[origin]; duplicate || len(phi.Operands) == 0 || len(phi.Operands) > p.remaining || !nativeProofWork(work, int64(len(phi.Operands))) {
			return nil
		}
		p.remaining -= len(phi.Operands)
		for _, operand := range phi.Operands {
			p.phis[origin] = append(p.phis[origin], operand.Origin)
		}
	}
	return p
}

func nativeConstructorNarrowWord(formal string, word int32) bool {
	switch formal {
	case "Z":
		return word == 0 || word == 1
	case "B":
		return word >= -128 && word <= 127
	case "C":
		return word >= 0 && word <= 65535
	case "S":
		return word >= -32768 && word <= 32767
	}
	return false
}

func (p *nativeConstructorFrameWords) argument(value frametransfer.Type, origin ssabuild.Origin, formal string) bool {
	if value.Kind != frametransfer.Int {
		return false
	}
	if value.HasInt {
		return nativeConstructorNarrowWord(formal, value.Int)
	}
	return p.origin(origin, formal)
}

func (p *nativeConstructorFrameWords) origin(origin ssabuild.Origin, formal string) bool {
	p.remaining--
	if p.remaining < 0 || p.active[origin] || !nativeProofWork(p.work, 1) {
		return false
	}
	p.active[origin] = true
	defer delete(p.active, origin)
	if origin.Kind == ssabuild.OriginParam {
		return p.params[origin.Slot] == formal
	}
	if origin.Kind == ssabuild.OriginPhi {
		operands := p.phis[origin]
		if len(operands) == 0 {
			return false
		}
		for _, operand := range operands {
			if !p.origin(operand, formal) {
				return false
			}
		}
		return true
	}
	if origin.Kind != ssabuild.OriginInstr {
		return false
	}
	ins, known := p.ir.InstrByID(methodir.InstrID(origin.PC))
	if !known {
		return false
	}
	if ins.Const.Kind == methodir.ConstInt {
		return nativeConstructorNarrowWord(formal, ins.Const.Int)
	}
	// Immediate integer constants do not use the constant-pool Const field.
	// A merged frame drops its known value, so inspect the original producer
	// of every phi operand rather than confusing computational int with Z/B/C/S.
	if ins.Opcode >= core.OP_ICONST_M1 && ins.Opcode <= core.OP_ICONST_5 {
		return nativeConstructorNarrowWord(formal, int32(ins.Opcode-core.OP_ICONST_0))
	}
	switch ins.Opcode {
	case core.OP_BIPUSH:
		return len(ins.Data) == 1 && nativeConstructorNarrowWord(formal, int32(int8(ins.Data[0])))
	case core.OP_SIPUSH:
		return len(ins.Data) == 2 && nativeConstructorNarrowWord(formal, int32(int16(binary.BigEndian.Uint16(ins.Data))))
	case core.OP_I2B:
		return formal == "B"
	case core.OP_I2C:
		return formal == "C"
	case core.OP_I2S:
		return formal == "S"
	case core.OP_GETFIELD, core.OP_GETSTATIC:
		return ins.Desc == formal
	case core.OP_INVOKEVIRTUAL, core.OP_INVOKEINTERFACE, core.OP_INVOKESTATIC:
		_, result, err := callbinding.Descriptor(ins.Desc)
		return err == nil && result == formal
	}
	return false
}
