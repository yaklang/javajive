package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

type staticInitializerFieldKey struct{ name, descriptor string }

// A field table is not an execution schedule. Moving an initial PUTSTATIC
// sequence to declarations is order-preserving only while declaration positions
// increase. At the first inversion retain that store and the remaining suffix
// in <clinit>; already lifted prefix stores still execute before that block.
// This is an additional restriction on hoisting, not permission to move a call,
// change constant-variable status, or leave an original exception handler.
func (c *ClassObjectDumper) orderedStaticInitializerPrefix(code *CodeAttribute, body []statements.Statement) map[*statements.AssignStatement]bool {
	if c == nil || c.obj == nil || code == nil || len(c.obj.Fields) > 4096 || len(code.Code) > 65535 || !nativeProofWork(c.Work, int64(len(code.Code)+len(c.obj.Fields))) || c.Work != nil && c.Work.CheckAlloc(int64(len(code.Code)+len(c.obj.Fields))*64) != nil {
		return nil
	}
	declarations := map[staticInitializerFieldKey]int{}
	for i, f := range c.obj.Fields {
		if f == nil {
			return nil
		}
		if f.AccessFlags&8 == 0 {
			continue
		}
		name, named := sourceBridgeUTF8(c.obj, f.NameIndex)
		desc, typed := sourceBridgeUTF8(c.obj, f.DescriptorIndex)
		key := staticInitializerFieldKey{name, desc}
		if _, duplicate := declarations[key]; duplicate || !named || !typed {
			return nil
		}
		declarations[key] = i
	}
	writes, err := c.decodeInterfaceWrites(code, false)
	if err != nil {
		return nil
	}
	return orderedStaticStorePrefix(body, c.obj.GetClassName(), declarations, writes, code.ExceptionTable, c.Work)
}

func orderedStaticStorePrefix(body []statements.Statement, owner string, declarations map[staticInitializerFieldKey]int, writes []interfaceFieldWrite, handlers []*ExceptionTableEntry, work *workbudget.Budget) map[*statements.AssignStatement]bool {
	if len(body) > 4096 || len(writes) > 4096 || len(handlers) > 4096 || !nativeProofWork(work, int64(len(body)+len(writes)+len(handlers))) || work != nil && work.CheckAlloc(int64(len(body)+len(writes))*128) != nil {
		return nil
	}
	physical := map[int]interfaceFieldWrite{}
	for _, write := range writes {
		if _, duplicate := physical[write.pc]; duplicate || write.pc < 0 {
			return nil
		}
		physical[write.pc] = write
	}
	result := map[*statements.AssignStatement]bool{}
	previousDeclaration, previousPC := -1, -1
	for _, st := range body {
		if middle, ok := st.(*statements.MiddleStatement); ok && middle != nil && middle.Data == nil && (middle.Flag == "start" || middle.Flag == "end") {
			continue
		}
		assign, ok := st.(*statements.AssignStatement)
		if !ok || assign == nil || assign.IsDeclare || assign.ArrayMember != nil || assign.JavaValue == nil || !assign.HasOriginPC || assign.OriginPC <= previousPC {
			break
		}
		field, ok := assign.LeftValue.(*values.JavaClassMember)
		if !ok || field == nil || normalizedInitializerOwner(field.Name) != normalizedInitializerOwner(owner) {
			break
		}
		write, witnessed := physical[assign.OriginPC]
		position, declared := declarations[staticInitializerFieldKey{field.Member, field.Description}]
		if !witnessed || write.name != field.Member || write.descriptor != field.Description || !declared || position <= previousDeclaration {
			break
		}
		for _, handler := range handlers {
			if !nativeProofWork(work, 1) || handler == nil || handler.StartPc >= handler.EndPc || assign.OriginPC >= int(handler.StartPc) && assign.OriginPC < int(handler.EndPc) {
				return result
			}
		}
		result[assign] = true
		previousDeclaration, previousPC = position, assign.OriginPC
	}
	return result
}
