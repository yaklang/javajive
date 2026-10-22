package core

import (
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A canonical compiler factory has a linear, pure packet: one allocation,
// ordered reads of this initializing class's own constants, sequential stores,
// and its one backing-field sink. It needs neither the nested-operand DAG nor
// that more general algorithm's small-expression bound.
func (d *Decompiler) regeneratedConstantArrayPacket(array *values.NewExpression, ref *values.JavaRef, source RegeneratedStaticArray, sink *OpCode) bool {
	if array == nil || !array.IsArray() || !array.HasOriginPC || array.OriginPC != source.AllocationPC ||
		array.ConstructorCall != nil || array.ArgumentsGetter != nil || len(array.Length) != 1 || len(array.Initializer) != len(source.Constants) {
		return false
	}
	length, known := values.UnpackSoltValue(array.Length[0]).(*values.JavaLiteral)
	erasure, erased := values.SourceTypeErasure(array.Type(), d.FunctionContext)
	if !known || length.Data != len(source.Constants) || !erased || erasure != source.Descriptor {
		return false
	}
	var packet []*OpCode
	for _, op := range d.opCodes {
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		if op != nil && op.Instr != nil && op.Instr.OpCode != OP_NOP && op.Instr.OpCode != OP_START && int(op.CurrentOffset) >= source.AllocationPC && int(op.CurrentOffset) <= source.StorePC {
			packet = append(packet, op)
		}
	}
	if len(packet) != 4*len(source.Constants)+2 || packet[len(packet)-1] != sink {
		return false
	}
	allocation := packet[0]
	if allocation.Instr.OpCode != OP_ANEWARRAY || len(allocation.Data) != 2 || len(allocation.stackProduced) != 1 || values.UnpackSoltValue(allocation.stackProduced[0]) != array {
		return false
	}
	component, known := d.constantPoolGetter(int(Convert2bytesToInt(allocation.Data))).(*values.JavaClassValue)
	if !known || component == nil {
		return false
	}
	actual, known := values.SourceTypeErasure(component.Type(), d.FunctionContext)
	if !known || actual != "L"+source.Owner+";" {
		return false
	}
	allowed := map[*OpCode]int{sink: 0}
	for i, constant := range source.Constants {
		dup, index, read, store := packet[1+4*i], packet[2+4*i], packet[3+4*i], packet[4+4*i]
		if ref == nil || dup.Instr.OpCode != OP_DUP ||
			!d.regeneratedArrayIndex(index, i) || read.Instr.OpCode != OP_GETSTATIC || len(read.Data) != 2 ||
			store.Instr.OpCode != OP_AASTORE || len(store.stackConsumed) != 3 || !delegationArraySameRef(store.stackConsumed[2], ref) {
			return false
		}
		// The first DUP materializes the allocation as a private source local:
		// the simulator pops the expression and pushes that local twice. Later
		// DUPs only peek at the existing local and record their one extra push.
		if i == 0 {
			if len(dup.stackConsumed) != 1 || values.UnpackSoltValue(dup.stackConsumed[0]) != array || len(dup.stackProduced) != 2 ||
				!delegationArraySameRef(dup.stackProduced[0], ref) || !delegationArraySameRef(dup.stackProduced[1], ref) {
				return false
			}
		} else if len(dup.stackConsumed) != 0 || len(dup.stackProduced) != 1 || !delegationArraySameRef(dup.stackProduced[0], ref) {
			return false
		}
		storedIndex, indexKnown := values.UnpackSoltValue(store.stackConsumed[1]).(*values.JavaLiteral)
		field, fieldKnown := d.constantPoolGetter(int(Convert2bytesToInt(read.Data))).(*values.JavaClassMember)
		element, elementKnown := values.UnpackSoltValue(array.Initializer[i]).(*values.JavaClassMember)
		if !indexKnown || storedIndex.Data != i || !fieldKnown || field == nil ||
			strings.ReplaceAll(field.Name, ".", "/") != source.Owner || field.Member != constant || field.Description != "L"+source.Owner+";" ||
			!elementKnown || element == nil || strings.ReplaceAll(element.Name, ".", "/") != source.Owner || element.Member != constant || element.Description != field.Description ||
			!element.OriginalStaticFieldRead(int(read.CurrentOffset), source.Owner, constant, field.Description) ||
			values.UnpackSoltValue(store.stackConsumed[0]) != values.UnpackSoltValue(array.Initializer[i]) {
			return false
		}
		allowed[dup], allowed[store] = 0, 2
	}
	if len(source.Constants) != 0 && (!array.HasEvaluationEndPC || array.EvaluationEndPC != int(packet[len(packet)-2].CurrentOffset)) {
		return false
	}
	if ref == nil {
		if values.UnpackSoltValue(sink.stackConsumed[0]) != array {
			return false
		}
	} else if !delegationArraySameRef(sink.stackConsumed[0], ref) {
		return false
	}
	// No publication, extra original use or hidden local store can acquire this
	// source occurrence just by reusing its type or allocation PC.
	for _, op := range d.opCodes {
		if op == nil || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		for index, value := range op.stackConsumed {
			original := values.UnpackSoltValue(value)
			if original != array && (ref == nil || !delegationArraySameRef(value, ref)) {
				continue
			}
			position, owns := allowed[op]
			if !owns || position != index {
				return false
			}
		}
	}
	return true
}

// Literal pushes do not all populate stackProduced. Reopen their encoded
// integer operand instead; the AASTORE separately proves the consumed index.
func (d *Decompiler) regeneratedArrayIndex(op *OpCode, index int) bool {
	if op == nil || op.Instr == nil {
		return false
	}
	switch code := op.Instr.OpCode; {
	case code >= OP_ICONST_M1 && code <= OP_ICONST_5:
		return int(code)-int(OP_ICONST_0) == index
	case code == OP_BIPUSH:
		return len(op.Data) == 1 && int(int8(op.Data[0])) == index
	case code == OP_SIPUSH:
		return len(op.Data) == 2 && int(int16(binary.BigEndian.Uint16(op.Data))) == index
	case code == OP_LDC || code == OP_LDC_W:
		pool := 0
		if code == OP_LDC && len(op.Data) == 1 {
			pool = int(op.Data[0])
		} else if code == OP_LDC_W && len(op.Data) == 2 {
			pool = int(binary.BigEndian.Uint16(op.Data))
		}
		if pool == 0 || d.ConstantPoolLiteralGetter == nil {
			return false
		}
		literal, known := d.ConstantPoolLiteralGetter(pool).(*values.JavaLiteral)
		return known && literal != nil && literal.Data == index
	default:
		return false
	}
}
