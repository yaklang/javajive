package javaclassparser

import (
	"encoding/binary"
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/mutf8"
	"github.com/yaklang/javajive/internal/workbudget"
)

// InnerClasses catalogs can retain foreign class constants with no remaining
// source use. Only source transactions may omit those standalone constants;
// the archive index still records the complete original constant pool. Scan
// all original code, including unreachable instructions, in one pass rather
// than rescanning it for each class name. Every verifier-frame class word is
// retained too; unknown structural metadata preserves every class constant.
func nativeMemberSourceBindingNames(object *ClassObject, work *workbudget.Budget) ([]string, bool) {
	if object == nil || len(object.ConstantPool) > 65534 || len(object.Fields) > 65535 || len(object.Methods) > 65535 || !nativeMemberDeclarationMetadataBounded(object, work) || work != nil && work.CheckAlloc(int64(len(object.ConstantPool))*32) != nil {
		return nil, false
	}
	mask := map[uint16]bool{}
	keepAll := false
	debugNames := map[string]bool{}
	retain := func(index uint16, optional bool) bool {
		if index == 0 && optional {
			return true
		}
		if !nativeProofWork(work, 1) || object.checkCPIndex(index, false, "source class dependency", CONSTANT_Class) != nil {
			return false
		}
		mask[index] = true
		return true
	}
	if !retain(object.ThisClass, false) || !retain(object.SuperClass, true) || len(object.Interfaces) > 65535 {
		return nil, false
	}
	for _, index := range object.Interfaces {
		if !retain(index, false) {
			return nil, false
		}
	}
	for i, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if c, isClass := constant.(*ConstantClassInfo); isClass {
			if c == nil {
				return nil, false
			}
			if _, known := sourceBridgeClassName(object, uint16(i+1)); !known {
				return nil, false
			}
		}
		// Unused symbolic member references stay conservative obligations too.
		if member := nativeConstantMember(constant); member != nil && !retain(member.ClassIndex, false) {
			return nil, false
		}
	}
	var attributes func([]AttributeInfo, int) bool
	attributes = func(list []AttributeInfo, depth int) bool {
		if depth > 2 || len(list) > 65535 || !nativeProofWork(work, int64(len(list))+1) {
			return false
		}
		for _, attribute := range list {
			switch a := attribute.(type) {
			case *BootstrapMethodsAttribute:
				if a == nil || len(a.BootstrapMethods) > 65535 {
					return false
				}
				for _, method := range a.BootstrapMethods {
					if method == nil || len(method.BootstrapArguments) > 65535 || !nativeProofWork(work, int64(len(method.BootstrapArguments))+1) {
						return false
					}
					for _, index := range method.BootstrapArguments {
						if object.checkCPIndex(index, false, "bootstrap class dependency", CONSTANT_String, CONSTANT_Class, CONSTANT_Integer, CONSTANT_Long, CONSTANT_Float, CONSTANT_Double, CONSTANT_MethodHandle, CONSTANT_MethodType, CONSTANT_Dynamic) != nil {
							return false
						}
						if _, isClass := object.ConstantPool[index-1].(*ConstantClassInfo); isClass && !retain(index, false) {
							return false
						}
					}
				}
			case *ExceptionsAttribute:
				if a == nil || len(a.ExceptionIndexTable) > 65535 {
					return false
				}
				for _, index := range a.ExceptionIndexTable {
					if !retain(index, false) {
						return false
					}
				}
			case *CodeAttribute:
				if a == nil || depth != 0 || len(a.ExceptionTable) > 65535 || !nativeProofWork(work, int64(len(a.Code))+int64(len(a.ExceptionTable))) || work != nil && work.CheckAlloc(int64(len(a.Code))*64+int64(len(object.ConstantPool))*32) != nil {
					return false
				}
				for _, handler := range a.ExceptionTable {
					if handler == nil || !retain(handler.CatchType, true) {
						return false
					}
				}
				if !attributes(a.Attributes, depth+1) {
					return false
				}
				decoder := core.NewDecompiler(a.Code, nil)
				decoder.Work = work
				if decoder.ParseOpcode() != nil {
					return false
				}
				for _, op := range decoder.Opcodes() {
					if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
						return false
					}
					index, classOperand := uint16(0), false
					switch op.Instr.OpCode {
					case core.OP_LDC:
						if len(op.Data) != 1 {
							return false
						}
						index = uint16(op.Data[0])
					case core.OP_LDC_W, core.OP_LDC2_W:
						if len(op.Data) != 2 {
							return false
						}
						index = binary.BigEndian.Uint16(op.Data)
					case core.OP_NEW, core.OP_ANEWARRAY, core.OP_CHECKCAST, core.OP_INSTANCEOF, core.OP_MULTIANEWARRAY:
						length := 2
						if op.Instr.OpCode == core.OP_MULTIANEWARRAY {
							length = 3
						}
						if len(op.Data) != length {
							return false
						}
						index, classOperand = binary.BigEndian.Uint16(op.Data[:2]), true
					default:
						continue
					}
					if classOperand {
						if !retain(index, false) {
							return false
						}
						continue
					}
					if op.Instr.OpCode == core.OP_LDC2_W {
						if object.checkCPIndex(index, false, "source ldc2 dependency", CONSTANT_Long, CONSTANT_Double, CONSTANT_Dynamic) != nil {
							return false
						}
					} else if object.checkCPIndex(index, false, "source ldc dependency", CONSTANT_Integer, CONSTANT_Float, CONSTANT_String, CONSTANT_Class, CONSTANT_MethodType, CONSTANT_MethodHandle, CONSTANT_Dynamic) != nil {
						return false
					} else if _, isClass := object.ConstantPool[index-1].(*ConstantClassInfo); isClass && !retain(index, false) {
						return false
					}
				}
			case *UnparsedAttribute:
				if a == nil || !nativeProofWork(work, int64(len(a.Info))+1) {
					return false
				}
				switch a.Name {
				case "EnclosingMethod":
					if len(a.Info) != 4 || !retain(binary.BigEndian.Uint16(a.Info[:2]), false) {
						return false
					}
				case "LocalVariableTable", "LocalVariableTypeTable":
					if len(a.Info) < 2 || len(a.Info) != 2+10*int(binary.BigEndian.Uint16(a.Info[:2])) {
						return false
					}
					for offset := 2; offset < len(a.Info); offset += 10 {
						index := binary.BigEndian.Uint16(a.Info[offset+6 : offset+8])
						text, known := sourceBridgeUTF8(object, index)
						if !known || !nativeProofWork(work, int64(len(text))) {
							return false
						}
						if a.Name == "LocalVariableTable" && mutf8.ValidateFieldDescriptor(object.ConstantPool[index-1].(*ConstantUtf8Info).semanticUnits()) != nil {
							return false
						}
						refs, known := types.SignatureClassReferences(text)
						if !known || work != nil && work.CheckAlloc(int64(len(debugNames)+len(refs))*96+int64(len(object.ConstantPool))*32) != nil {
							return false
						}
						for _, name := range refs {
							debugNames[strings.ReplaceAll(name, ".", "/")] = true
						}
					}
				case "StackMapTable":
					if !nativeStackMapClassDependencies(a.Info, func(index uint16) bool { return retain(index, false) }, work) {
						return false
					}
				case "SourceDebugExtension", "Synthetic", "Deprecated", "LineNumberTable", "MethodParameters":
					// These attributes contain no class operands. Original
					// parser/admission proofs validate their other metadata.
				default:
					// Opaque structural class operands remain obligations.
					keepAll = true
				}
			case *InnerClassesAttribute, *SignatureAttribute, *SourceFileAttribute, *SyntheticAttribute, *DeprecatedAttribute, *LineNumberTableAttribute, *ConstantValueAttribute, *RuntimeVisibleAnnotationsAttribute, *RuntimeVisibleParameterAnnotationsAttribute, *AnnotationDefaultAttribute, *TypeAnnotationsAttribute, *RuntimeVisibleTypeAnnotationsAttribute:
				// Ownership, descriptors and annotations are handled by the
				// existing original-metadata proofs and dependency collector.
			default:
				keepAll = true
			}
		}
		return true
	}
	if !attributes(object.Attributes, 0) {
		return nil, false
	}
	for _, members := range [][]*MemberInfo{object.Fields, object.Methods} {
		for _, member := range members {
			if member == nil || !nativeProofWork(work, 1) || !attributes(member.Attributes, 0) {
				return nil, false
			}
		}
	}
	if keepAll {
		mask = nil
	}
	names, known := nativeMemberDependencyNamesWithClassMask(object, "", mask, work)
	if !known {
		return nil, false
	}
	// Local debug descriptors can be UTF8-only, without a CONSTANT_Class.
	// Sort additions so source graph traversal is independent of Go map order.
	if !nativeProofWork(work, int64(len(names)+len(debugNames))) || work != nil && work.CheckAlloc(int64(len(names)+len(debugNames))*96) != nil {
		return nil, false
	}
	seen := make(map[string]bool, len(names)+len(debugNames))
	for _, name := range names {
		seen[name] = true
	}
	additions := make([]string, 0, len(debugNames))
	for name := range debugNames {
		additions = append(additions, name)
	}
	sort.Strings(additions)
	for _, name := range additions {
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	return names, true
}
