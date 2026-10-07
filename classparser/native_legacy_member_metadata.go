package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Legacy named ownership still has authoritative InnerClasses self rows.
// Do not infer local/anonymous ownership without EnclosingMethod, or reinterpret
// later classfile features that a pre-49 JVM may ignore/reject.
func nativeMemberVersionMetadata(obj *ClassObject, work *workbudget.Budget) bool {
	if obj == nil || obj.MajorVersion < 45 || obj.MajorVersion > 54 {
		return false
	}
	if obj.MajorVersion >= 53 {
		return nativeAccessorVersion(obj, work)
	}
	if obj.MajorVersion >= 49 {
		return true
	}
	if obj.MajorVersion == 45 && obj.MinorVersion > 3 || obj.MajorVersion > 45 && obj.MinorVersion != 0 {
		return false
	}
	for _, constant := range obj.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		switch constant.(type) {
		case nil, *ConstantUtf8Info, *ConstantIntegerInfo, *ConstantFloatInfo, *ConstantLongInfo, *ConstantDoubleInfo, *ConstantClassInfo, *ConstantStringInfo, *ConstantFieldrefInfo, *ConstantMethodrefInfo, *ConstantInterfaceMethodrefInfo, *ConstantNameAndTypeInfo:
		default:
			return false
		}
	}
	for _, attribute := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		switch attribute.(type) {
		case *SourceFileAttribute, *InnerClassesAttribute:
		default:
			return false
		}
	}
	for _, field := range obj.Fields {
		if field == nil || !nativeProofWork(work, 1) {
			return false
		}
		for _, attribute := range field.Attributes {
			if !nativeProofWork(work, 1) {
				return false
			}
			switch attribute.(type) {
			case *SyntheticAttribute, *ConstantValueAttribute:
			default:
				return false
			}
		}
	}
	for _, method := range obj.Methods {
		if method == nil || !nativeProofWork(work, 1) || method.AccessFlags&(0x0040|0x0080|0x1000) != 0 {
			return false
		}
		for _, attribute := range method.Attributes {
			if !nativeProofWork(work, 1) {
				return false
			}
			switch a := attribute.(type) {
			case *ExceptionsAttribute:
			case *CodeAttribute:
				if a == nil || !nativeLegacyMemberCode(obj, a, work) {
					return false
				}
				for _, debug := range a.Attributes {
					if !nativeProofWork(work, 1) {
						return false
					}
					switch debug.(type) {
					case *LineNumberTableAttribute:
					case *UnparsedAttribute:
						// LocalVariableTable is retained as raw debug metadata by
						// the parser. Do not mistake another opaque attribute for it.
						a := debug.(*UnparsedAttribute)
						if a == nil || a.Name != "LocalVariableTable" || len(a.Info) < 2 || a.Length != uint32(len(a.Info)) || len(a.Info) != 2+10*int(binary.BigEndian.Uint16(a.Info)) || !nativeProofWork(work, int64(len(a.Info))) {
							return false
						}
					default:
						return false
					}
				}
			default:
				return false
			}
		}
	}
	return true
}

// A version number cannot turn modern bytecode into legacy bytecode. In
// particular pre-49 ldc may not resolve a class literal, and pre-52 static or
// special calls may not name an InterfaceMethodref. Instruction boundaries
// come from the bounded decoder, rather than a byte scan through operands.
func nativeLegacyMemberCode(obj *ClassObject, code *CodeAttribute, work *workbudget.Budget) bool {
	if !nativeProofWork(work, int64(len(code.Code))) {
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
			return false
		case core.OP_LDC, core.OP_LDC_W, core.OP_INVOKESTATIC, core.OP_INVOKESPECIAL:
			index := 0
			if len(op.Data) == 1 && op.Instr.OpCode == core.OP_LDC {
				index = int(op.Data[0])
			} else if len(op.Data) == 2 {
				index = int(binary.BigEndian.Uint16(op.Data))
			}
			if index < 1 || index > len(obj.ConstantPool) {
				return false
			}
			constant := obj.ConstantPool[index-1]
			if op.Instr.OpCode == core.OP_LDC || op.Instr.OpCode == core.OP_LDC_W {
				switch constant.(type) {
				case *ConstantIntegerInfo, *ConstantFloatInfo, *ConstantStringInfo:
				default:
					return false
				}
			} else if _, ok := constant.(*ConstantMethodrefInfo); !ok {
				return false
			}
		}
	}
	return true
}

// ACC_SYNTHETIC and the zero-length Synthetic marker are original equivalent
// JVM encodings. Every additional capture-field attribute still needs its own
// source regeneration proof; unrelated ordinary field annotations are retained.
func nativeMemberEffectiveFieldFlags(field *MemberInfo, work *workbudget.Budget) (uint16, bool, bool) {
	if field == nil {
		return 0, false, false
	}
	flags := field.AccessFlags
	syntheticAttributes := 0
	onlySynthetic := true
	for _, attribute := range field.Attributes {
		if !nativeProofWork(work, 1) {
			return 0, false, false
		}
		if a, ok := attribute.(*SyntheticAttribute); ok {
			if a == nil || a.AttrLen != 0 {
				return 0, false, false
			}
			syntheticAttributes++
			flags |= 0x1000
		} else {
			onlySynthetic = false
		}
	}
	if syntheticAttributes > 1 {
		return 0, false, false
	}
	return flags, onlySynthetic, true
}
