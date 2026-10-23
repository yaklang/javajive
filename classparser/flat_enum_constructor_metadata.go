package javaclassparser

import (
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func flatEnumConstructorParameter(object *ClassObject, row []byte, expected byte, work *workbudget.Budget) bool {
	if len(row) != 4 || row[2] != expected || row[3]&^byte(0x10) != 0 {
		return false
	}
	index := binary.BigEndian.Uint16(row)
	if index == 0 {
		return true
	}
	name, known := sourceBridgeUTF8(object, index)
	return known && nativeProofWork(work, int64(len(name))+1) && class_context.PreservesJava8Identifier(name)
}

// A forwarding constructor has no local storage beyond this and its original
// parameters. Debug rows may describe any subset, including generic Class<?>
// parameters; their scope and erased type must still match the physical slot.
func flatEnumConstructorDebugTable(object *ClassObject, code *CodeAttribute, table *UnparsedAttribute, params []string, work *workbudget.Budget) bool {
	if table == nil || code == nil || table.Length != uint32(len(table.Info)) || len(table.Info) < 2 || !nativeProofWork(work, int64(len(table.Info))) {
		return false
	}
	count := int(binary.BigEndian.Uint16(table.Info))
	if len(table.Info) != 2+10*count {
		return false
	}
	slots := map[int]string{0: "L" + object.GetClassName() + ";"}
	slot := 1
	for _, descriptor := range params {
		slots[slot] = descriptor
		slot += constructorEffectType(descriptor).width()
	}
	seen := map[int]bool{}
	for i := 0; i < count; i++ {
		row := table.Info[2+10*i : 12+10*i]
		start, length := int(binary.BigEndian.Uint16(row)), int(binary.BigEndian.Uint16(row[2:]))
		name, named := sourceBridgeUTF8(object, binary.BigEndian.Uint16(row[4:]))
		typ, typed := sourceBridgeUTF8(object, binary.BigEndian.Uint16(row[6:]))
		slot := int(binary.BigEndian.Uint16(row[8:]))
		expected, parameter := slots[slot]
		if start != 0 || length != len(code.Code) || !named || !typed || !parameter || seen[slot] || (name == "" || strings.ContainsAny(name, ".;[/")) {
			return false
		}
		seen[slot] = true
		if table.Name == "LocalVariableTypeTable" {
			erased, throws, valid := types.EraseConcreteMethodSignatureWithThrows("()" + typ)
			if !valid || len(throws) != 0 || erased != "()"+expected {
				return false
			}
		} else if table.Name != "LocalVariableTable" || typ != expected {
			return false
		}
	}
	return true
}

// The table belongs to retained method bodies, not to the erased forwarding
// constructor (whose full opcode sequence is checked separately). Validate its
// encoding without treating any bootstrap as pure or suppressing its methods.
func flatEnumBodyBootstrapTable(object *ClassObject, table *BootstrapMethodsAttribute, work *workbudget.Budget) bool {
	if object == nil || table == nil || object.MajorVersion < 51 || int(table.NumBootstrapMethods) != len(table.BootstrapMethods) || !nativeProofWork(work, int64(table.NumBootstrapMethods)+1) {
		return false
	}
	length := uint64(2)
	for _, method := range table.BootstrapMethods {
		if method == nil || int(method.NumBootstrapArguments) != len(method.BootstrapArguments) || !nativeProofWork(work, int64(method.NumBootstrapArguments)+1) || object.checkCPIndex(method.BootstrapMethodRef, false, "enum body bootstrap", CONSTANT_MethodHandle) != nil {
			return false
		}
		handle := object.ConstantPool[method.BootstrapMethodRef-1].(*ConstantMethodHandleInfo)
		if handle == nil || handle.ReferenceKind != 6 && handle.ReferenceKind != 8 {
			return false
		}
		target, known := nativeLambdaOriginalHandleTarget(object, handle)
		if !known {
			return false
		}
		if _, result, err := callbinding.Descriptor(target.descriptor); err != nil || handle.ReferenceKind == 8 && result != "V" {
			return false
		}
		for _, argument := range method.BootstrapArguments {
			if object.checkCPIndex(argument, false, "enum body bootstrap argument", CONSTANT_String, CONSTANT_Class, CONSTANT_Integer, CONSTANT_Long, CONSTANT_Float, CONSTANT_Double, CONSTANT_MethodHandle, CONSTANT_MethodType) != nil {
				return false
			}
		}
		length += 4 + 2*uint64(method.NumBootstrapArguments)
	}
	return length == uint64(table.AttributeLength)
}
