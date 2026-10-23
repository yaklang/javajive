package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
)

// A dead local destination can be omitted, but its producer must execute once
// before SUPER. Reopen the original STORE and adjacent invocation, then prove
// that no JVM read (including wide overlap, IINC or RET) observes the slot.
// This is a source view for the existing ordered delegation carrier; it neither
// moves the call behind initialization nor changes the original statement list.
func (c *ClassObjectDumper) constructorDiscardedPrefixEffect(st statements.Statement, decoder *core.Decompiler, method *MemberInfo, delegatePC int) *statements.ExpressionStatement {
	assign, ok := st.(*statements.AssignStatement)
	if !ok || assign == nil || decoder == nil || method == nil || c == nil || c.obj == nil {
		return nil
	}
	pc, slot, known := assign.OriginalLocalStore()
	if !known && assign.HasOriginPC && assign.ArrayMember == nil {
		// A source clone may carry the first declaration's sealed seed rather
		// than a later STORE-web receipt. Reopen that original definition,
		// including the unchanged RHS and exact assignment PC.
		if ref, ok := assign.LeftValue.(*values.JavaRef); ok && ref != nil {
			pc, slot, known = ref.OriginalLocalDeclaration(assign.JavaValue)
			known = known && pc == assign.OriginPC
		}
	}
	if !known || pc >= delegatePC {
		return nil
	}
	call, ok := values.UnpackSoltValue(assign.JavaValue).(*values.FunctionCallExpression)
	if !ok || call == nil || !call.HasOriginPC || call.OriginPC < 0 || call.OriginPC >= pc {
		return nil
	}
	invoke := decoder.OpcodeByPC(uint16(call.OriginPC))
	store := decoder.OpcodeByPC(uint16(pc))
	if invoke == nil || store == nil || invoke.Instr == nil || store.Instr == nil || invoke.IsWide || call.OriginPC+1+len(invoke.Data) != pc || core.GetStoreIdx(store) != slot {
		return nil
	}
	_, result, err := callbinding.Descriptor(call.Descriptor)
	if err != nil || result == "V" {
		return nil
	}
	category := strings.IndexByte("IJFDL", constructorEffectType(result).kind)
	opcode := store.Instr.OpCode
	if category < 0 || !(opcode == core.OP_ISTORE+category || opcode >= core.OP_ISTORE_0+category*4 && opcode < core.OP_ISTORE_0+category*4+4) {
		return nil
	}
	access := core.LocalAccessOf(store.Instr.OpCode)
	if !access.Write || access.Read || access.Width != constructorEffectType(result).width() {
		return nil
	}
	desc, known := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	params, _, err := callbinding.Descriptor(desc)
	if !known || err != nil {
		return nil
	}
	words := 1
	for _, param := range params {
		words += constructorEffectType(param).width()
	}
	if slot < words {
		return nil
	}
	ops := decoder.Opcodes()
	if len(ops) > 65536 || !nativeProofWork(c.Work, int64(len(ops))+1) {
		return nil
	}
	for _, op := range ops {
		if op == nil || op.Instr == nil {
			return nil
		}
		// Legacy subroutine control requires its own live-local analysis.
		if op.Instr.OpCode == core.OP_JSR || op.Instr.OpCode == core.OP_JSR_W || op.Instr.OpCode == core.OP_RET {
			return nil
		}
		read := core.LocalAccessOf(op.Instr.OpCode)
		if read.Read {
			at := core.GetRetrieveIdx(op)
			if at < 0 || at < slot+access.Width && slot < at+read.Width {
				return nil
			}
		}
	}
	return statements.NewExpressionStatement(call)
}

func (p *constructorSourceBoundary) sourcePrefixEffects() []statements.Statement {
	if p.prefixEffects != nil {
		return p.prefixEffects
	}
	return p.prefix
}
