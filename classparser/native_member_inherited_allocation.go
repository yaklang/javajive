package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A subclass can allocate an inherited member using the exact current THIS
// object as its enclosing argument. No synthetic capture GETFIELD or nullable
// qualifier check is needed in the original bytecode. Require a complete
// original superclass chain and an unchanged initialized instance receiver.
func nativeMemberInheritedAllocationThis(obj *ClassObject, method *MemberInfo, ops []*core.OpCode, child *nativeMemberClass, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if obj == nil || method == nil || child == nil || child.static || child.owner == obj.GetClassName() || resolve == nil || method.AccessFlags&8 != 0 {
		return false
	}
	name, known := sourceBridgeUTF8(obj, method.NameIndex)
	if !known || name == "<init>" || name == "<clinit>" || len(ops) == 0 || len(ops) > 65535 || !nativeProofWork(work, int64(len(ops))) {
		return false
	}
	for _, op := range ops {
		if op == nil || op.Instr == nil || core.GetStoreIdx(op) == 0 {
			return false
		}
	}
	return nativeMemberOriginalClassWidening(obj.GetClassName(), child.owner, resolve, work)
}

// Source emission must independently bind the operand to the proved original
// receiver. A same-typed parameter, mutable simulator value or custom closure
// cannot borrow the nonnull THIS certificate.
func nativeMemberInheritedAllocationOperand(value any, receiver string, ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if receiver == "" || ctx == nil || ctx.IsStatic || strings.ReplaceAll(ctx.ClassName, ".", "/") != receiver {
		return false
	}
	v, ok := value.(values.JavaValue)
	if !ok || sourceProofNil(v) {
		return false
	}
	v, known := nativeMemberEnclosingUnpack(v, work)
	if !known {
		return false
	}
	ref, ok := v.(*values.JavaRef)
	if !ok || ref == nil || !ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
		return false
	}
	erasure, known := values.SourceTypeErasure(ref.Type(), ctx)
	return known && erasure == "L"+receiver+";"
}
