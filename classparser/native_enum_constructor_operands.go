package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The general expression getter assumes well-formed CP references. Before the
// new constructor-frame analysis calls it, close the physical operand tags and
// nested references. A malformed receipt must refuse admission, not panic while
// constructing an otherwise useful name/ordinal identity proof.
func nativeEnumConstructorFrameOperands(obj *ClassObject, ops []*core.OpCode, retained int64, work *workbudget.Budget) bool {
	validated := map[uint16]bool{}
	var expanded int64
	consume := func(characters int64) bool {
		// The snapshot's expression getter materializes metadata at each use.
		// Descriptor grammar can be memoized, but a repeated huge descriptor
		// must not bypass either traversal or the combined frame high-water.
		bytes := characters*32 + 128
		if bytes > (64<<20)-expanded || !nativeProofWork(work, characters+1) || work != nil && work.CheckAlloc(retained+expanded+bytes) != nil {
			return false
		}
		expanded += bytes
		return true
	}
	for _, op := range ops {
		if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
			return false
		}
		code := op.Instr.OpCode
		switch code {
		case core.OP_GETSTATIC, core.OP_PUTSTATIC, core.OP_GETFIELD, core.OP_PUTFIELD,
			core.OP_INVOKEVIRTUAL, core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE:
			member := constructorMotionMember(obj, op, code)
			if member == nil {
				return false
			}
			if !consume(int64(len(member.Name)) + int64(len(member.Description))) {
				return false
			}
			index := core.Convert2bytesToInt(op.Data[:2])
			if validated[index] {
				continue
			}
			if len(member.Description) > 65535 || !nativeProofWork(work, int64(len(member.Description))) || work != nil && work.CheckAlloc(int64(len(member.Description))*32+int64(len(validated)+1)*24+128) != nil {
				return false
			}
			desc := member.Description
			field := code >= core.OP_GETSTATIC && code <= core.OP_PUTFIELD
			if field {
				desc = "(" + desc + ")V"
			}
			params, _, err := callbinding.Descriptor(desc)
			if err != nil || field && len(params) != 1 {
				return false
			}
			validated[index] = true
			continue
		case core.OP_LDC, core.OP_LDC_W, core.OP_LDC2_W,
			core.OP_NEW, core.OP_ANEWARRAY, core.OP_CHECKCAST, core.OP_INSTANCEOF, core.OP_MULTIANEWARRAY, core.OP_INVOKEDYNAMIC:
		default:
			continue
		}
		index := uint16(0)
		if code == core.OP_LDC {
			if len(op.Data) != 1 {
				return false
			}
			index = uint16(op.Data[0])
		} else {
			if len(op.Data) < 2 {
				return false
			}
			index = core.Convert2bytesToInt(op.Data[:2])
		}
		if index == 0 || int(index) > len(obj.ConstantPool) {
			return false
		}
		constant := obj.ConstantPool[index-1]
		if code == core.OP_INVOKEDYNAMIC {
			cp, ok := constant.(*ConstantInvokeDynamicInfo)
			if !ok || cp == nil || obj.MajorVersion < 51 || len(op.Data) != 4 || op.Data[2] != 0 || op.Data[3] != 0 || cp.NameAndTypeIndex == 0 || int(cp.NameAndTypeIndex) > len(obj.ConstantPool) {
				return false
			}
			nt, ok := obj.ConstantPool[cp.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			if !ok || nt == nil {
				return false
			}
			name, nk := sourceBridgeUTF8(obj, nt.NameIndex)
			desc, dk := sourceBridgeUTF8(obj, nt.DescriptorIndex)
			if !nk || name == "" || !dk || len(desc) > 65535 || !nativeProofWork(work, int64(len(desc))) || work != nil && work.CheckAlloc(int64(len(desc))*32+128) != nil {
				return false
			}
			if !consume(int64(len(name)) + int64(len(desc))) {
				return false
			}
			if _, _, err := callbinding.Descriptor(desc); err != nil {
				return false
			}
			continue
		}
		if code == core.OP_LDC2_W {
			if !consume(0) {
				return false
			}
			switch cp := constant.(type) {
			case *ConstantLongInfo:
				if cp == nil {
					return false
				}
			case *ConstantDoubleInfo:
				if cp == nil {
					return false
				}
			default:
				return false
			}
			continue
		}
		if code == core.OP_LDC || code == core.OP_LDC_W {
			switch cp := constant.(type) {
			case *ConstantIntegerInfo:
				if cp == nil || !consume(0) {
					return false
				}
				continue
			case *ConstantFloatInfo:
				if cp == nil || !consume(0) {
					return false
				}
				continue
			case *ConstantStringInfo:
				if cp == nil {
					return false
				}
				if text, known := sourceBridgeUTF8(obj, cp.StringIndex); !known || !consume(int64(len(text))) {
					return false
				}
				continue
			}
		}
		if name, known := sourceBridgeClassName(obj, index); !known || name == "" || !consume(int64(len(name))) {
			return false
		}
	}
	return true
}
