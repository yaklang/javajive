package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
)

func nativeAnonymousLegacySourceFeatures(c *ClassObjectDumper, child *nativeAnonymousClass) bool {
	if child == nil {
		return false
	}
	if child.legacyInputMajor == 0 {
		return true
	}
	if c == nil || c.obj == nil || child.object == nil || c.options.SourceCompiler != NativeJavac8 || c.options.TargetSourceVersion != 8 || c.obj.MajorVersion != child.legacyInputMajor || child.object.MajorVersion != child.legacyInputMajor {
		return false
	}
	return nativeAnonymousLegacyInputFeatures(c.obj, c.Work) && nativeAnonymousLegacyInputFeatures(child.object, c.Work)
}

// This closes only the Java5/6 input feature domain, never source ownership or
// constructor/capture permission. Keep it separate from pre-Java5 validation:
// ldc Class became legal in Java5 and is not a future constant in this domain.
func nativeAnonymousLegacyInputFeatures(obj *ClassObject, work *workbudget.Budget) bool {
	if obj == nil || obj.MajorVersion < 49 || obj.MajorVersion > 50 || obj.MinorVersion != 0 || len(obj.ConstantPool) > 65535 || len(obj.Methods) > 65535 || !nativeProofWork(work, int64(len(obj.ConstantPool)+len(obj.Methods))+1) {
		return false
	}
	for i, cp := range obj.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if cp == nil {
			if i == 0 {
				return false
			}
			switch obj.ConstantPool[i-1].(type) {
			case *ConstantLongInfo, *ConstantDoubleInfo:
				continue
			default:
				return false
			}
		}
		switch cp.(type) {
		case *ConstantUtf8Info, *ConstantIntegerInfo, *ConstantFloatInfo, *ConstantClassInfo, *ConstantStringInfo, *ConstantFieldrefInfo, *ConstantMethodrefInfo, *ConstantInterfaceMethodrefInfo, *ConstantNameAndTypeInfo:
		case *ConstantLongInfo, *ConstantDoubleInfo:
			if i+1 >= len(obj.ConstantPool) || obj.ConstantPool[i+1] != nil {
				return false
			}
		default:
			return false
		}
		if reflect.ValueOf(cp).IsNil() {
			return false
		} // All admitted constant categories are pointers.
	}
	for _, method := range obj.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, known := sourceBridgeUTF8(obj, method.NameIndex)
		if !known {
			return false
		}
		codes := 0
		for _, attr := range method.Attributes {
			if !nativeProofWork(work, 1) {
				return false
			}
			if code, ok := attr.(*CodeAttribute); ok {
				codes++
				if codes > 1 || !nativeAnonymousLegacyCodeFeatures(obj, code, work) {
					return false
				}
			}
		}
		if obj.AccessFlags&0x0200 != 0 {
			if name == "<clinit>" {
				if method.AccessFlags != 8 || codes != 1 {
					return false
				}
			} else if method.AccessFlags&^uint16(0x0080) != 0x0401 || codes != 0 {
				// Interface default/static/private implementations did not exist
				// in this domain, even if their Code uses only older opcodes.
				return false
			}
		} else if method.AccessFlags&(0x0100|0x0400) != 0 {
			if codes != 0 {
				return false
			}
		} else if codes != 1 {
			return false
		}
	}
	return true
}

func nativeAnonymousLegacyCodeFeatures(obj *ClassObject, code *CodeAttribute, work *workbudget.Budget) bool {
	if obj == nil || code == nil || len(code.Code) == 0 || len(code.Code) > 65535 || !nativeProofWork(work, int64(len(code.Code))) || work != nil && work.CheckAlloc(int64(len(code.Code))*96+128) != nil {
		return false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return false
	}
	for _, op := range decoder.Opcodes() {
		if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
			return false
		}
		opcode := op.Instr.OpCode
		if opcode == core.OP_INVOKEDYNAMIC || opcode == core.OP_JSR || opcode == core.OP_RET || opcode == core.OP_JSR_W {
			// The selected javac8 profile cannot certify a return-address
			// subroutine through the ordinary Java-value constructor proof.
			return false
		}
		cpKind := byte(0)
		switch opcode {
		case core.OP_LDC, core.OP_LDC_W:
			cpKind = 'l'
		case core.OP_LDC2_W:
			cpKind = 'w'
		case core.OP_GETSTATIC, core.OP_PUTSTATIC, core.OP_GETFIELD, core.OP_PUTFIELD:
			cpKind = 'f'
		case core.OP_INVOKEVIRTUAL, core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC:
			cpKind = 'm'
		case core.OP_INVOKEINTERFACE:
			cpKind = 'i'
		case core.OP_NEW, core.OP_ANEWARRAY, core.OP_CHECKCAST, core.OP_INSTANCEOF, core.OP_MULTIANEWARRAY:
			cpKind = 'c'
		}
		if cpKind == 0 {
			continue
		}
		index := 0
		if opcode == core.OP_LDC {
			if len(op.Data) != 1 {
				return false
			}
			index = int(op.Data[0])
		} else {
			if len(op.Data) < 2 {
				return false
			}
			index = int(core.Convert2bytesToInt(op.Data[:2]))
		}
		if index <= 0 || index > len(obj.ConstantPool) {
			return false
		}
		cp, valid := obj.ConstantPool[index-1], false
		switch cpKind {
		case 'l':
			switch c := cp.(type) {
			case *ConstantIntegerInfo:
				valid = c != nil
			case *ConstantFloatInfo:
				valid = c != nil
			case *ConstantStringInfo:
				valid = c != nil
			case *ConstantClassInfo:
				valid = c != nil
			}
		case 'w':
			switch c := cp.(type) {
			case *ConstantLongInfo:
				valid = c != nil
			case *ConstantDoubleInfo:
				valid = c != nil
			}
		case 'f':
			c, ok := cp.(*ConstantFieldrefInfo)
			valid = ok && c != nil
		case 'm':
			c, ok := cp.(*ConstantMethodrefInfo)
			valid = ok && c != nil
		case 'i':
			c, ok := cp.(*ConstantInterfaceMethodrefInfo)
			valid = ok && c != nil
		case 'c':
			c, ok := cp.(*ConstantClassInfo)
			valid = ok && c != nil
		}
		if !valid {
			return false
		}
	}
	return true
}
