package javaclassparser

import (
	"crypto/sha256"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A consumer-specific certificate. An anonymous lexical THIS is still not a
// Java expression: only this exact member allocation can omit it. Ordinary
// arguments keep their original producers and the selected constructor binding.
type nativeAnonymousMemberEnclosing struct {
	caller                     *ClassObject
	method                     *MemberInfo
	code                       *CodeAttribute
	read                       *nativeMemberLexicalRead
	child                      *nativeMemberClass
	methodKey, descriptor      string
	newPC, invokePC            int
	sourceName, enclosingOwner string
	codeHash                   [32]byte
}

func nativeAnonymousMemberFormalFree(o *ClassObject, attrs []AttributeInfo, name string, work *workbudget.Budget) bool {
	if o == nil {
		return false
	}
	found := false
	for _, a := range attrs {
		if !nativeProofWork(work, 1) {
			return false
		}
		if s, ok := a.(*SignatureAttribute); ok {
			if found || s == nil {
				return false
			}
			found = true
			sig, known := sourceBridgeUTF8(o, s.SignatureIndex)
			if !known || len(sig) > 4096 || !nativeProofWork(work, int64(len(sig))+1) {
				return false
			}
			own, _, valid := types.SignatureTypeVariableReferences(sig)
			if !valid {
				return false
			}
			for _, n := range own {
				if n == name {
					return false
				}
			}
		}
	}
	return true
}

// Only the immediate original anonymous parent owns the selected member. An
// inherited/member/local/type-formal shadow in the inner source scope refuses.
func nativeAnonymousMemberTypeLookup(f *nativeAnonymousForest, caller *ClassObject, method *MemberInfo, target string, child *nativeMemberClass, work *workbudget.Budget) bool {
	if f == nil || caller == nil || method == nil || child == nil || f.units[target] == nil || f.objects[target] == nil {
		return false
	}
	localSafe := func() bool {
		parent, lexical, known := originalAnonymousOwner(caller)
		if !known || parent != target || !nativeAnonymousMemberFormalFree(caller, caller.Attributes, child.name, work) || !nativeAnonymousMemberFormalFree(caller, method.Attributes, child.name, work) {
			return false
		}
		parentObject := f.objects[target]
		if f.units[target].object != parentObject || parentObject.GetClassName() != target {
			return false
		}
		if !nativeAnonymousMemberFormalFree(parentObject, parentObject.Attributes, child.name, work) {
			return false
		}
		matches := 0
		for _, m := range parentObject.Methods {
			if m == nil || !nativeProofWork(work, 1) {
				return false
			}
			n, nk := sourceBridgeUTF8(parentObject, m.NameIndex)
			d, dk := sourceBridgeUTF8(parentObject, m.DescriptorIndex)
			if !nk || !dk {
				return false
			}
			if n+d == lexical {
				matches++
				if !nativeAnonymousMemberFormalFree(parentObject, m.Attributes, child.name, work) {
					return false
				}
			}
		}
		if matches != 1 {
			return false
		}
		// Same-spelled method-local types require a declaration-site scope proof.
		for _, scope := range []*ClassObject{caller, parentObject} {
			for _, a := range scope.Attributes {
				if !nativeProofWork(work, 1) {
					return false
				}
				if table, ok := a.(*InnerClassesAttribute); ok {
					if table == nil {
						return false
					}
					for _, row := range table.Classes {
						if row == nil || !nativeProofWork(work, 1) {
							return false
						}
						if row.OuterClassInfoIndex == 0 && row.InnerNameIndex != 0 {
							n, ok := sourceBridgeUTF8(scope, row.InnerNameIndex)
							if !ok || n == child.name {
								return false
							}
						}
					}
				}
			}
		}

		return true
	}
	if !localSafe() {
		return false
	}
	unshadowed := func(o *ClassObject) bool {
		n := o.GetClassName()
		for _, a := range o.Attributes {
			if !nativeProofWork(work, 1) {
				return false
			}
			if table, ok := a.(*InnerClassesAttribute); ok {
				if table == nil {
					return false
				}
				for _, row := range table.Classes {
					if row == nil || !nativeProofWork(work, 1) {
						return false
					}
					if row.OuterClassInfoIndex != 0 && row.InnerNameIndex != 0 {
						owner, ok := sourceBridgeClassName(o, row.OuterClassInfoIndex)
						if !ok {
							return false
						}
						if owner == n {
							simple, ok := sourceBridgeUTF8(o, row.InnerNameIndex)
							if !ok || simple == child.name {
								return false
							}
						}
					}
				}
			}
		}

		return true
	}
	objects := map[string]*ClassObject{}
	edges := map[string][]string{}
	seen, active := map[string]bool{}, map[string]bool{}
	var visit func(*ClassObject, int) bool
	visit = func(o *ClassObject, depth int) bool {
		if o == nil || depth >= 32 || active[o.GetClassName()] || !nativeProofWork(work, 1) {
			return false
		}
		n := o.GetClassName()
		if seen[n] {
			return true
		}
		if len(seen)+len(active) >= 128 || work != nil && work.CheckAlloc(int64(len(seen)+len(active)+1)*128) != nil {
			return false
		}
		active[n] = true
		if !unshadowed(o) {
			return false
		}
		parents := append([]string{}, o.GetInterfacesName()...)
		if p := o.GetSupperClassName(); p != "" {
			parents = append(parents, p)
		}
		objects[n] = o
		edges[n] = append([]string(nil), parents...)
		for _, p := range parents {
			next := f.objects[p]
			if next == nil {
				var ok bool
				if f.resolve == nil {
					return false
				}
				next, ok = f.resolve(p)
				if !ok || next == nil || next.GetClassName() != p {
					return false
				}
			}
			if !visit(next, depth+1) {
				return false
			}
		}
		delete(active, n)
		seen[n] = true
		return true
	}
	if !visit(caller, 0) || !localSafe() {
		return false
	}
	// The last provider callback may have changed an earlier declaration.
	// Reopen all local absence/edge facts without calling another provider.
	for n, o := range objects {
		if o == nil || o.GetClassName() != n || !unshadowed(o) {
			return false
		}
		now := append([]string{}, o.GetInterfacesName()...)
		if parent := o.GetSupperClassName(); parent != "" {
			now = append(now, parent)
		}
		if len(now) != len(edges[n]) {
			return false
		}
		for i, p := range now {
			if p != edges[n][i] || !nativeProofWork(work, 1) {
				return false
			}
		}
	}
	return true
}

// Resolve the source type between two original packet checks. Provider reads
// cannot revoke an earlier mutable field, method, code or declaration witness.
func nativeAnonymousMemberEnclosingProof(f *nativeAnonymousForest, caller *ClassObject, method *MemberInfo, code *CodeAttribute, ops []*core.OpCode, read *nativeMemberLexicalRead, target string, work *workbudget.Budget) *nativeAnonymousMemberEnclosing {
	before := nativeAnonymousMemberEnclosingPacket(f, caller, method, code, ops, read, target, work)
	if before == nil || !nativeAnonymousMemberTypeLookup(f, caller, method, target, before.child, work) || sha256.Sum256(code.Code) != before.codeHash {
		return nil
	}
	after := nativeAnonymousMemberEnclosingPacket(f, caller, method, code, ops, read, target, work)
	if after == nil || after.child != before.child || after.methodKey != before.methodKey || after.descriptor != before.descriptor || after.newPC != before.newPC || after.invokePC != before.invokePC || after.sourceName != before.sourceName || after.enclosingOwner != before.enclosingOwner || after.codeHash != before.codeHash {
		return nil
	}
	return after
}

// Prove NEW/DUP/THIS/enclosing-GETFIELD, its exclusive constructor use and the
// remaining argument evaluation. The latter can contain effectful producers;
// none may consume or duplicate the unnameable enclosing receiver.
// The original verifier frame must contain initialized THIS and this exact NEW;
// no branch or handler can enter its operands. The source name is resolved in
// the original lexical hierarchy, separately from the JVM descriptor.
func nativeAnonymousMemberEnclosingPacket(f *nativeAnonymousForest, caller *ClassObject, method *MemberInfo, code *CodeAttribute, ops []*core.OpCode, read *nativeMemberLexicalRead, target string, work *workbudget.Budget) *nativeAnonymousMemberEnclosing {
	if f == nil || f.members == nil || caller == nil || method == nil || code == nil || read == nil || read.prior != nil || read.parameterOwner != "" || read.parameterDescriptor != "" || len(code.Code) == 0 || len(code.Code) > 65535 || len(ops) > len(code.Code) || len(code.ExceptionTable) != 0 || method.AccessFlags&8 != 0 || !nativeProofWork(work, 1) {
		return nil
	}
	unit := f.units[caller.GetClassName()]
	if unit == nil || unit.object != caller || !unit.parentAnonymous || read.owner != caller.GetClassName() || read.field != unit.enclosingField || read.descriptor != "L"+target+";" {
		return nil
	}
	parameter, captured := unit.fields[read.field]
	if !captured || parameter != 0 {
		return nil
	}
	name, nok := sourceBridgeUTF8(caller, method.NameIndex)
	desc, dok := sourceBridgeUTF8(caller, method.DescriptorIndex)
	if !nok || !dok || name == "<clinit>" {
		return nil
	}
	found := 0
	for _, m := range caller.Methods {
		if !nativeProofWork(work, 1) {
			return nil
		}
		if m == method {
			found++
		}
	}
	if found != 1 {
		return nil
	}
	codeFound := 0
	for _, a := range method.Attributes {
		if !nativeProofWork(work, 1) {
			return nil
		}
		if c, ok := a.(*CodeAttribute); ok {
			if c != code {
				return nil
			}
			codeFound++
		}
	}
	if codeFound != 1 {
		return nil
	}
	for _, op := range ops {
		if op == nil || op.Instr == nil || core.GetStoreIdx(op) == 0 || !nativeProofWork(work, 1) {
			return nil
		}
	}
	fieldCount := 0
	for _, field := range caller.Fields {
		if field == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(caller, field.NameIndex)
		d, dk := sourceBridgeUTF8(caller, field.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if n == read.field {
			flags, only, valid := nativeMemberEffectiveFieldFlags(field, work)
			if !valid || !only || flags != 0x1010 || d != read.descriptor {
				return nil
			}
			fieldCount++
		}
	}
	if fieldCount != 1 {
		return nil
	}
	at := -1
	for i, op := range ops {
		if int(op.CurrentOffset) == read.pc {
			at = i
		}
	}
	if at < 3 || at+1 >= len(ops) || ops[at-3].Instr.OpCode != core.OP_NEW || ops[at-2].Instr.OpCode != core.OP_DUP || !constructorMotionLoad(ops[at-1], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[at-1]) != 0 {
		return nil
	}
	if len(ops[at].Data) != 2 || len(ops[at-3].Data) != 2 {
		return nil
	}
	fieldIndex := int(core.Convert2bytesToInt(ops[at].Data))
	if fieldIndex <= 0 || fieldIndex > len(caller.ConstantPool) {
		return nil
	}
	fieldRef, fieldKind := caller.ConstantPool[fieldIndex-1].(*ConstantFieldrefInfo)
	if !fieldKind || fieldRef == nil {
		return nil
	}
	field := constructorMotionMember(caller, ops[at], core.OP_GETFIELD)
	if field == nil || field.Name != read.owner || field.Member != read.field || field.Description != read.descriptor {
		return nil
	}
	allocated, valid := sourceBridgeClassName(caller, uint16(core.Convert2bytesToInt(ops[at-3].Data)))
	if !valid {
		return nil
	}
	reader := NewClassObjectDumper(caller)
	reader.Work = work
	// Use the shared frame-allocation and analysis-update bounds. An unrelated
	// 512-byte packet cutoff rejected a small constructor expression merely
	// because its containing method had more ordinary argument/body work.
	_, fn, known := reader.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return nil
	}
	records, known := nativeAnonymousArgumentRecords(fn, work)
	if !known {
		return nil
	}
	newPC := int(ops[at-3].CurrentOffset)
	invokeAt := -1
	newOrigin := ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(newPC)}
	for i := at + 1; i < len(ops); i++ {
		if !nativeProofWork(work, 1) {
			return nil
		}
		call := constructorMotionMember(caller, ops[i], core.OP_INVOKESPECIAL)
		record := records[int(ops[i].CurrentOffset)]
		if call == nil || call.Member != "<init>" || record == nil || len(record.Uses) == 0 || record.Uses[0] != newOrigin {
			continue
		}
		// Same-owner nested allocations are different NEW identities. One
		// allocation with alternative initializers has no single source call.
		if invokeAt >= 0 || call.Name != allocated {
			return nil
		}
		invokeAt = i
	}
	if invokeAt < 0 || len(ops[invokeAt].Data) != 2 {
		return nil
	}
	methodIndex := int(core.Convert2bytesToInt(ops[invokeAt].Data))
	if methodIndex <= 0 || methodIndex > len(caller.ConstantPool) {
		return nil
	}
	methodRef, methodKind := caller.ConstantPool[methodIndex-1].(*ConstantMethodrefInfo)
	invoke := constructorMotionMember(caller, ops[invokeAt], core.OP_INVOKESPECIAL)
	if !methodKind || methodRef == nil || invoke == nil {
		return nil
	}
	parameters, result, err := callbinding.Descriptor(invoke.Description)
	if err != nil || result != "V" || len(parameters) == 0 || parameters[0] != "L"+target+";" {
		return nil
	}
	child := f.members.children[allocated]
	if child == nil || child.static || child.owner != target || child.formalCount != 0 || child.object == nil || child.object.GetClassName() != allocated || f.objects[allocated] != child.object || child.object.AccessFlags&0x600 != 0 {
		return nil
	}
	ctor := nativeMemberConstructorForAllocation(child, invoke.Description)
	if ctor == nil {
		return nil
	}
	sourceParameters, sourceResult, err := callbinding.Descriptor(ctor.sourceDescriptor)
	if err != nil || sourceResult != "V" || len(sourceParameters) != len(parameters)-1 {
		return nil
	}
	for i, parameter := range sourceParameters {
		if parameter != parameters[i+1] || !nativeProofWork(work, 1) {
			return nil
		}
	}
	originalOwner, originalName, flags, known := originalMemberOwner(child.object)
	if !known || originalOwner != target || originalName != child.name || flags != child.flags {
		return nil
	}
	ctorFound := false
	for _, m := range child.object.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(child.object, m.NameIndex)
		d, dk := sourceBridgeUTF8(child.object, m.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if n == "<init>" && d == invoke.Description {
			if ctorFound || m.AccessFlags&(8|0x1000|0x400|0x100) != 0 {
				return nil
			}
			ctorFound = true
		}
	}
	if !ctorFound {
		return nil
	}
	get, call := records[read.pc], records[int(ops[invokeAt].CurrentOffset)]
	if get == nil || call == nil || len(get.Uses) != 1 || get.Uses[0] != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 0}) || len(get.Before.Stack) == 0 || get.Before.Stack[len(get.Before.Stack)-1].Kind != frametransfer.Ref || get.Before.Stack[len(get.Before.Stack)-1].Class != caller.GetClassName() || len(call.Uses) != len(parameters)+1 || call.Uses[0] != newOrigin || call.Uses[1] != (ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(read.pc)}) {
		return nil
	}
	receiverIndex := len(call.Before.Stack) - nativeMemberParameterWidth(parameters) - 1
	if receiverIndex < 0 || call.Before.Stack[receiverIndex].Kind != frametransfer.UninitNew || call.Before.Stack[receiverIndex].NewPC != uint16(newPC) || call.Before.Stack[receiverIndex].Class != allocated {
		return nil
	}
	// Grant an omission only for the one hidden argument. A DUP, local store,
	// ordinary argument, field publication or other consumer of this GETFIELD
	// needs its own source expression and cannot borrow this certificate.
	if !nativeAnonymousMemberEnclosingExclusiveUse(fn, read.pc, int(ops[invokeAt].CurrentOffset), work) {
		return nil
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil
	}
	entries, valid := nativeMemberLexicalControlEntries(decoder, code, work)
	if !valid {
		return nil
	}
	for _, pc := range entries {
		if pc > newPC && pc <= int(ops[invokeAt].CurrentOffset) {
			return nil
		}
	}
	return &nativeAnonymousMemberEnclosing{caller: caller, method: method, code: code, read: read, child: child, methodKey: name + desc, descriptor: invoke.Description, newPC: newPC, invokePC: int(ops[invokeAt].CurrentOffset), codeHash: sha256.Sum256(code.Code), sourceName: child.name, enclosingOwner: child.owner}
}

func nativeAnonymousMemberEnclosingExclusiveUse(fn *ssabuild.Function, readPC, invokePC int, work *workbudget.Budget) bool {
	if fn == nil || readPC < 0 || readPC > 65535 || invokePC <= readPC || invokePC > 65535 {
		return false
	}
	origin := ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(readPC)}
	for _, phi := range fn.Phis {
		for _, operand := range phi.Operands {
			if !nativeProofWork(work, 1) || operand.Origin == origin {
				return false
			}
		}
	}
	uses := 0
	for _, record := range fn.Instructions {
		for i, used := range record.Uses {
			if !nativeProofWork(work, 1) {
				return false
			}
			if used == origin {
				if int(record.PC) != invokePC || i != 1 {
					return false
				}
				uses++
			}
		}
	}
	return uses == 1
}

func nativeAnonymousMemberEnclosingReceipt(p *nativeMemberFamily, caller *ClassObject, method string, newPC int, child *nativeMemberClass, work *workbudget.Budget) *nativeAnonymousMemberEnclosing {
	if p == nil || p.failed || p.anonymousForest == nil || p.anonymousForest.members != p || caller == nil || child == nil || !nativeProofWork(work, 1) {
		return nil
	}
	f := p.anonymousForest
	r := f.memberEnclosing[caller.GetClassName()][method][newPC]
	if r == nil || r.caller != caller || r.child != child || r.methodKey != method || r.newPC != newPC || r.code == nil || sha256.Sum256(r.code.Code) != r.codeHash {
		return nil
	}
	current := caller.GetClassName()
	group := p.anonymousUnits[current]
	if group == nil || group.failed || group.forest != f || f.groups[group.owner] != group || f.objects[current] != caller || f.units[current] == nil || group.children[current] != f.units[current] || r.read == nil || !f.readPCs[current][method][r.read.pc] || f.lexicalThis[current][method][r.read.pc] || f.reads[current][method][r.read.pc] != nil || child.name != r.sourceName || child.owner != r.enclosingOwner {
		return nil
	}
	d := core.NewDecompiler(r.code.Code, func(i int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, i) })
	d.Work = work
	if d.ParseOpcode() != nil {
		return nil
	}
	recheck := nativeAnonymousMemberEnclosingProof(f, caller, r.method, r.code, constructorMotionOps(d), r.read, child.owner, work)
	if recheck == nil || recheck.child != r.child || recheck.newPC != r.newPC || recheck.invokePC != r.invokePC || recheck.descriptor != r.descriptor || recheck.codeHash != r.codeHash || recheck.sourceName != r.sourceName || recheck.enclosingOwner != r.enclosingOwner {
		return nil
	}
	return r
}
