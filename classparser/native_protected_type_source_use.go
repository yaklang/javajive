package javaclassparser

import (
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/mutf8"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Only a standalone class constant used by InnerClasses metadata may disappear
// from a source-access obligation. Keep all member owners, runtime and generic
// descriptors, annotations, bootstrap operands, hierarchy, handlers and actual
// class instructions, including unreachable code and unused symbolic members.
// This does not remove any original archive/index/dependency-validation edge.
func nativeProtectedTypeRequiresSourceAccess(object *ClassObject, target string, work *workbudget.Budget) (bool, bool) {
	if object == nil || target == "" || len(object.ConstantPool) > 65534 || !nativeMemberDeclarationMetadataBounded(object, work) || work != nil && work.CheckAlloc(int64(len(object.ConstantPool))*16) != nil {
		return false, false
	}
	references, known := nativeMemberDependencyNamesWithoutStandaloneClass(object, target, work)
	if !known {
		return false, false
	}
	for _, name := range references {
		if name == target {
			return true, true
		}
	}
	indices := map[uint16]bool{}
	for i, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false, false
		}
		if c, ok := constant.(*ConstantClassInfo); ok && c != nil {
			name, known := sourceBridgeClassName(object, uint16(i+1))
			if !known {
				return false, false
			}
			if name == target {
				indices[uint16(i+1)] = true
			}
		}
	}
	if indices[object.SuperClass] {
		return true, true
	}
	for _, index := range object.Interfaces {
		if !nativeProofWork(work, 1) {
			return false, false
		}
		if indices[index] {
			return true, true
		}
	}
	for _, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false, false
		}
		if member := nativeConstantMember(constant); member != nil && indices[member.ClassIndex] {
			return true, true
		}
	}
	for _, attribute := range object.Attributes {
		if !nativeProofWork(work, 1) {
			return false, false
		}
		switch a := attribute.(type) {
		case *BootstrapMethodsAttribute:
			if a == nil {
				return false, false
			}
			for _, method := range a.BootstrapMethods {
				if method == nil || !nativeProofWork(work, int64(len(method.BootstrapArguments))+1) {
					return false, false
				}
				for _, index := range method.BootstrapArguments {
					if indices[index] {
						return true, true
					}
				}
			}
		case *UnparsedAttribute:
			if a == nil {
				return false, false
			}
			switch a.Name {
			case "EnclosingMethod":
				if len(a.Info) != 4 {
					return false, false
				}
				if indices[binary.BigEndian.Uint16(a.Info[:2])] {
					return true, true
				}
			case "SourceDebugExtension", "Synthetic", "Deprecated":
				// These JVMS attributes contain no class operands.
			default:
				// Opaque structural metadata (for example Record, permits or
				// module tables) has not proved an ignorable class operand.
				return false, false
			}
		}
	}
	for _, method := range object.Methods {
		if method == nil || !nativeProofWork(work, int64(len(method.Attributes))+1) {
			return false, false
		}
		for _, attribute := range method.Attributes {
			switch a := attribute.(type) {
			case *ExceptionsAttribute:
				if a == nil || !nativeProofWork(work, int64(len(a.ExceptionIndexTable))) {
					return false, false
				}
				for _, index := range a.ExceptionIndexTable {
					if indices[index] {
						return true, true
					}
				}
			case *CodeAttribute:
				if a == nil || !nativeProofWork(work, int64(len(a.Code))+int64(len(a.ExceptionTable))) || work != nil && work.CheckAlloc(int64(len(a.Code))*64) != nil {
					return false, false
				}
				for _, handler := range a.ExceptionTable {
					if handler == nil {
						return false, false
					}
					if indices[handler.CatchType] {
						return true, true
					}
				}
				for _, attribute := range a.Attributes {
					raw, ok := attribute.(*UnparsedAttribute)
					if !ok {
						continue
					}
					if raw == nil || !nativeProofWork(work, int64(len(raw.Info))+1) {
						return false, false
					}
					if raw.Name == "StackMapTable" && len(indices) != 0 {
						// A verifier-only class word may still seed a source
						// consumer. Keep this obligation conservatively.
						return true, true
					}
					if raw.Name != "LocalVariableTable" && raw.Name != "LocalVariableTypeTable" {
						if raw.Name != "LineNumberTable" && raw.Name != "StackMapTable" {
							return false, false
						}
						continue
					}
					if len(raw.Info) < 2 || len(raw.Info) != 2+10*int(binary.BigEndian.Uint16(raw.Info[:2])) {
						return false, false
					}
					for offset := 2; offset < len(raw.Info); offset += 10 {
						index := binary.BigEndian.Uint16(raw.Info[offset+6 : offset+8])
						text, known := sourceBridgeUTF8(object, index)
						if !known || !nativeProofWork(work, int64(len(text))) {
							return false, false
						}
						if raw.Name == "LocalVariableTable" {
							utf := object.ConstantPool[index-1].(*ConstantUtf8Info)
							if mutf8.ValidateFieldDescriptor(utf.semanticUnits()) != nil {
								return false, false
							}
						}
						names, valid := types.SignatureClassReferences(text)
						if !valid {
							return false, false
						}
						for _, name := range names {
							if strings.ReplaceAll(name, ".", "/") == target {
								return true, true
							}
						}
					}
				}
				decoder := core.NewDecompiler(a.Code, func(index int) values.JavaValue { return GetValueFromCP(object.ConstantPool, index) })
				decoder.Work = work
				if decoder.ParseOpcode() != nil {
					return false, false
				}
				for _, op := range decoder.Opcodes() {
					if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
						return false, false
					}
					index := uint16(0)
					classOperand := false
					switch op.Instr.OpCode {
					case core.OP_LDC:
						if len(op.Data) != 1 {
							return false, false
						}
						index = uint16(op.Data[0])
					case core.OP_LDC_W, core.OP_LDC2_W:
						if len(op.Data) != 2 {
							return false, false
						}
						index = binary.BigEndian.Uint16(op.Data)
					case core.OP_NEW, core.OP_ANEWARRAY, core.OP_CHECKCAST, core.OP_INSTANCEOF:
						if len(op.Data) != 2 {
							return false, false
						}
						index, classOperand = binary.BigEndian.Uint16(op.Data), true
					case core.OP_MULTIANEWARRAY:
						if len(op.Data) != 3 {
							return false, false
						}
						index, classOperand = binary.BigEndian.Uint16(op.Data[:2]), true
					}
					if classOperand && object.checkCPIndex(index, false, "source class operand", CONSTANT_Class) != nil {
						return false, false
					}
					if op.Instr.OpCode == core.OP_LDC || op.Instr.OpCode == core.OP_LDC_W {
						if object.checkCPIndex(index, false, "source ldc operand", CONSTANT_Integer, CONSTANT_Float, CONSTANT_String, CONSTANT_Class, CONSTANT_MethodType, CONSTANT_MethodHandle, CONSTANT_Dynamic) != nil {
							return false, false
						}
					} else if op.Instr.OpCode == core.OP_LDC2_W && object.checkCPIndex(index, false, "source ldc2 operand", CONSTANT_Long, CONSTANT_Double, CONSTANT_Dynamic) != nil {
						return false, false
					}
					if indices[index] {
						return true, true
					}
				}
			}
		}
	}
	return false, true
}
