package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The caller proves that Java's compiler regenerates this field and producer.
// This packet only proves ownership of their decoded source occurrences; it
// grants no regeneration role to another field with a similar source spelling.
type RegeneratedStaticArray struct {
	Owner, Field, Descriptor, Helper string
	AllocationPC, StorePC            int
	Constants                        []string
}

func (d *Decompiler) ProjectRegeneratedStaticArray(root *[]statements.Statement, source RegeneratedStaticArray) bool {
	if d == nil || root == nil || d.FunctionContext == nil || d.FunctionContext.FunctionName != "<clinit>" ||
		strings.ReplaceAll(d.FunctionContext.ClassName, ".", "/") != source.Owner || source.Owner == "" ||
		source.Descriptor != "[L"+source.Owner+";" || source.StorePC <= source.AllocationPC ||
		d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(*root))+1) != nil || d.Work.CheckAlloc(int64(len(d.opCodes)+len(*root)+1)*64) != nil) {
		return false
	}
	store := d.opcodeAtOffset(source.StorePC)
	if store == nil || store.Instr == nil || store.Instr.OpCode != OP_PUTSTATIC || len(store.Data) != 2 || len(store.stackConsumed) != 1 {
		return false
	}
	member, known := d.constantPoolGetter(int(Convert2bytesToInt(store.Data))).(*values.JavaClassMember)
	if !known || member == nil || strings.ReplaceAll(member.Name, ".", "/") != source.Owner || member.Member != source.Field || member.Description != source.Descriptor {
		return false
	}
	index := -1
	var sink *statements.AssignStatement
	for i, statement := range *root {
		assignment, ok := statement.(*statements.AssignStatement)
		if !ok || assignment == nil || !assignment.HasOriginPC || assignment.OriginPC != source.StorePC {
			continue
		}
		field, ok := values.UnpackSoltValue(assignment.LeftValue).(*values.JavaClassMember)
		if !ok || field == nil || assignment.ArrayMember != nil || assignment.IsDeclare ||
			strings.ReplaceAll(field.Name, ".", "/") != source.Owner || field.Member != source.Field || field.Description != source.Descriptor || sink != nil {
			return false
		}
		index, sink = i, assignment
	}
	if sink == nil {
		return false
	}
	if source.Helper != "" {
		call, ok := values.UnpackSoltValue(sink.JavaValue).(*values.FunctionCallExpression)
		invoke := d.opcodeAtOffset(source.AllocationPC)
		if !ok || call == nil || invoke == nil || invoke.Instr == nil || invoke.Instr.OpCode != OP_INVOKESTATIC ||
			!call.IsStatic || call.Kind != values.InvokeStatic || len(call.Arguments) != 0 ||
			strings.ReplaceAll(call.ClassName, ".", "/") != source.Owner || call.FunctionName != source.Helper || call.Descriptor != "()"+source.Descriptor ||
			!sameBranchArrayInvocation(d.invokeFuncCall[invoke], call) || len(invoke.stackProduced) != 1 ||
			values.UnpackSoltValue(invoke.stackProduced[0]) != values.UnpackSoltValue(store.stackConsumed[0]) {
			return false
		}
		*root = append((*root)[:index:index], (*root)[index+1:]...)
		return true
	}
	var array *values.NewExpression
	var ref *values.JavaRef
	var producer *statements.AssignStatement
	if len(source.Constants) == 0 {
		array, _ = values.UnpackSoltValue(sink.JavaValue).(*values.NewExpression)
	} else if index > 0 {
		producer, _ = (*root)[index-1].(*statements.AssignStatement)
		if producer != nil && producer.ArrayMember == nil {
			ref, _ = values.UnpackSoltValue(producer.LeftValue).(*values.JavaRef)
			array, _ = values.UnpackSoltValue(producer.JavaValue).(*values.NewExpression)
		}
		if ref == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || !delegationArraySameRef(sink.JavaValue, ref) || statementTreeReferencesLocal(*root, ref, producer, sink) {
			return false
		}
	}
	if !d.regeneratedConstantArrayPacket(array, ref, source, store) {
		return false
	}
	if producer == nil {
		*root = append((*root)[:index:index], (*root)[index+1:]...)
	} else {
		*root = append((*root)[:index-1:index-1], (*root)[index+1:]...)
	}
	return true
}
