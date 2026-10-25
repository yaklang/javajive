package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// JVM <clinit> can assign an own final field at each terminal try/catch arm,
// while Java's definite-assignment analysis treats a protected assignment as
// possibly completed before a catch. Factor only the terminal stores through
// one private local. The RHS stays in its original arm and protected domain;
// no intervening effect moves across the field write. An exactly declared own
// primitive static final store cannot resolve another class or throw. Reject
// finally, nested control, incomplete arms and additional writes. This runs
// before lexical variable binding so the new local receives a normal identity.
func (d *Decompiler) JoinInitializerFinalStores(body *[]statements.Statement) {
	if d == nil || body == nil || d.FunctionContext == nil || d.FunctionContext.FunctionName != "<clinit>" || !d.FunctionContext.IsStatic || d.FunctionContext.OriginalFieldDeclarations == nil || d.constantPoolGetter == nil || d.OriginalOwnStaticFinalStore == nil {
		return
	}
	ctx := d.FunctionContext
	owner := strings.ReplaceAll(ctx.ClassName, ".", "/")
	decl, known := ctx.OriginalFieldDeclarations(owner)
	if !known || !decl.Complete || decl.Name != owner || len(*body) > 65535 || d.Work != nil && d.Work.CheckAlloc(int64(len(*body)+1)*64) != nil {
		return
	}
	for at := 0; at < len(*body); at++ {
		tr, ok := (*body)[at].(*statements.TryCatchStatement)
		if !ok || tr == nil || len(tr.CatchBodies) == 0 || len(tr.CatchBodies) > 64 || len(tr.Handlers) != len(tr.CatchBodies) || len(tr.Exception) != len(tr.CatchBodies) {
			continue
		}
		valid := true
		for _, h := range tr.Handlers {
			if h.CatchAll || len(h.ProtectedRanges) == 0 {
				valid = false
			}
		}
		if !valid {
			continue
		}
		arms := append([][]statements.Statement{tr.TryBody}, tr.CatchBodies...)
		if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(arms))*(int64(len(d.opcodeToSimulateStack))+1)+int64(len(decl.Fields))) != nil || d.Work.CheckAlloc(int64(len(arms))*128) != nil) {
			return
		}
		var stores []*statements.AssignStatement
		var field *values.JavaClassMember
		pcs := map[int]bool{}
		for _, arm := range arms {
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(arm))+1) != nil {
				return
			}
			if len(arm) == 0 {
				valid = false
				break
			}
			for _, s := range arm[:len(arm)-1] {
				switch s.(type) {
				case *statements.AssignStatement, *statements.ExpressionStatement:
				default:
					valid = false
				}
			}
			a, ok := arm[len(arm)-1].(*statements.AssignStatement)
			if !ok || a == nil || a.JavaValue == nil || a.ArrayMember != nil || !a.HasOriginPC || pcs[a.OriginPC] {
				valid = false
				break
			}
			f, ok := a.LeftValue.(*values.JavaClassMember)
			if !ok || f == nil || f.Type() == nil || strings.ReplaceAll(f.Name, ".", "/") != owner || len(f.Description) != 1 || !strings.Contains("ZBCSIJFD", f.Description) {
				valid = false
				break
			}
			if field != nil && (field.Member != f.Member || field.Description != f.Description) {
				valid = false
				break
			}
			if !d.OriginalOwnStaticFinalStore(a.OriginPC, owner, f.Member, f.Description) {
				valid = false
				break
			}
			op := d.opcodeAtOffset(a.OriginPC)
			if op == nil || op.Instr == nil || op.Instr.OpCode != OP_PUTSTATIC || len(op.Data) != 2 {
				valid = false
				break
			}
			original, ok := d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassMember)
			if !ok || original == nil || original.Name != f.Name || original.Member != f.Member || original.Description != f.Description {
				valid = false
				break
			}
			field = f
			pcs[a.OriginPC] = true
			stores = append(stores, a)
		}
		if !valid || field == nil {
			continue
		}
		matches := 0
		for _, f := range decl.Fields {
			if f.Name == field.Member && f.Descriptor == field.Description {
				if f.Access&0x58 != 0x18 {
					valid = false
				}
				matches++
			}
		}
		if !valid || matches != 1 {
			continue
		}
		writes := 0
		for _, op := range d.opCodes {
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
				return
			}
			if op == nil || op.Instr == nil || op.Instr.OpCode != OP_PUTSTATIC {
				continue
			}
			if len(op.Data) != 2 {
				valid = false
				break
			}
			f, ok := d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassMember)
			if !ok || f == nil {
				valid = false
				break
			}
			if strings.ReplaceAll(f.Name, ".", "/") == owner && f.Member == field.Member && f.Description == field.Description {
				writes++
				if !pcs[int(op.CurrentOffset)] {
					valid = false
				}
			}
		}
		if !valid || writes != len(stores) {
			continue
		}
		local := values.NewJavaRef(utils.NewRootVariableId(), nil, field.Type().Copy())
		// The declaration dominates every arm, and the ordinary scope binder
		// assigns its collision-free source name after this transformation.
		for _, a := range stores {
			a.LeftValue = local
			a.HasOriginPC = false
		}
		out := make([]statements.Statement, 0, len(*body)+2)
		out = append(out, (*body)[:at]...)
		out = append(out, statements.NewDeclareStatement(local), tr, &statements.AssignStatement{LeftValue: field, JavaValue: local})
		out = append(out, (*body)[at+1:]...)
		*body = out
		at += 2
	}
}
