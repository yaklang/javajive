package javaclassparser

import (
	"crypto/sha256"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A field user is not a lexical receiver proof. This initial domain accepts
// only a complete original anonymous THIS/enclosing-field/local-capture chain.
// It retains every original code/PC edge and refuses all writes/foreign bases.
func (c *ClassObjectDumper) nativeMethodLocalAnonymousCaptureUser(p *nativeMemberFamily, local *nativeMethodLocalClass, user, field string, work *workbudget.Budget) bool {
	if c == nil || local == nil || local.object == nil || local.constructor == nil || field == local.constructor.enclosingField || !local.declaredAnonymous[user] {
		return false
	}
	if _, ok := nativeMemberJointMethodLocalOwner(p, local.object, work); !ok {
		return false
	}
	group := p.anonymousUnits[user]
	if group == nil || group.failed || group.owner != local.object.GetClassName() || p.memberAnonymous[group.owner] != group {
		return false
	}
	child := group.children[user]
	if child == nil || child.object == nil || child.object.GetClassName() != user || child.enclosingField == "" {
		return false
	}
	index, exists := child.fields[child.enclosingField]
	if !exists || index != 0 {
		return false
	}
	ps, _, e := callbinding.Descriptor(child.descriptor)
	if e != nil || len(ps) == 0 || ps[0] != "L"+group.owner+";" {
		return false
	}
	owner, method, anon := originalAnonymousOwner(child.object)
	if !anon || owner != group.owner || method != child.method {
		return false
	}
	// Reopen the reciprocal parent row as well as the child's self row.
	// A cached declaration list cannot replace either original ownership edge.
	rows := 0
	for _, attribute := range local.object.Attributes {
		if table, ok := attribute.(*InnerClassesAttribute); ok {
			if table == nil {
				return false
			}
			for _, row := range table.Classes {
				if row == nil || !nativeProofWork(work, 1) {
					return false
				}
				name, known := sourceBridgeClassName(local.object, row.InnerClassInfoIndex)
				if !known {
					return false
				}
				if name == user {
					rows++
					if row.InnerNameIndex != 0 || row.OuterClassInfoIndex != 0 || !nativeAnonymousSelfAccessFlags(child.object, row.InnerClassAccessFlags, work) {
						return false
					}
				}
			}
		}
	}
	if rows != 1 {
		return false
	}
	captureIndex, ok := local.constructor.captures[field]
	if !ok {
		return false
	}
	caps, _, e := callbinding.Descriptor(local.constructor.descriptor)
	if e != nil || captureIndex < 0 || captureIndex >= len(caps) {
		return false
	}
	descriptor := caps[captureIndex]
	// Reopen the current physical packet with the selected compiler profile;
	// a retained map entry does not certify a changed constructor or field role.
	reader := NewClassObjectDumper(local.object)
	reader.Work = work
	reader.options = c.options
	reader.foldSiblingResolver = c.foldSiblingResolver
	reader.declarationResolver = c.declarationResolver
	actual := reader.nativeAnonymousConstructorForCompiler(child.object, owner, method, "", p, nil, reader.buildInvocationMetadata(), reader.originalNativeConstructorAccessBridges())
	if actual == nil || actual.descriptor != child.descriptor || actual.enclosingField != child.enclosingField || actual.superDescriptor != child.superDescriptor || actual.superPC != child.superPC || actual.sourceSuperDescriptor != child.sourceSuperDescriptor || actual.sharedParameterRoles != child.sharedParameterRoles || actual.unusedEnclosing != child.unusedEnclosing || actual.parentAnonymous != child.parentAnonymous || len(actual.superParams) != len(child.superParams) || len(actual.fields) != len(child.fields) || len(actual.capturePCs) != len(child.capturePCs) {
		return false
	}
	for i, param := range actual.superParams {
		if !nativeProofWork(work, 1) || param != child.superParams[i] {
			return false
		}
	}
	for f, i := range actual.fields {
		j, known := child.fields[f]
		if !known || j != i || actual.capturePCs[f] != child.capturePCs[f] {
			return false
		}
	}
	if len(child.object.Methods) > 1024 {
		return false
	}
	result := map[string]map[int]*nativeMemberLexicalRead{}
	seenMethods := map[string]bool{}
	seen := 0
	for _, m := range child.object.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, nk := sourceBridgeUTF8(child.object, m.NameIndex)
		desc, dk := sourceBridgeUTF8(child.object, m.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		key := name + desc
		if seenMethods[key] {
			return false
		}
		seenMethods[key] = true
		codeSeen := false
		for _, a := range m.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			if codeSeen {
				return false
			}
			codeSeen = true
			if code == nil || len(code.Code) > 65535 || !nativeProofWork(work, int64(len(code.Code))) {
				return false
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(child.object.ConstantPool, i) })
			d.Work = work
			if d.ParseOpcode() != nil {
				return false
			}
			entries, closed := nativeMemberLexicalControlEntries(d, code, work)
			if !closed {
				return false
			}
			ops := constructorMotionOps(d)
			stable := true
			for _, op := range ops {
				if core.GetStoreIdx(op) == 0 {
					stable = false
				}
			}
			reads := map[int]*nativeMemberLexicalRead{}
			for i, op := range ops {
				if !nativeProofWork(work, 1) {
					return false
				}
				for _, opcode := range []int{core.OP_GETFIELD, core.OP_PUTFIELD, core.OP_GETSTATIC, core.OP_PUTSTATIC} {
					f := constructorMotionMember(child.object, op, opcode)
					if f == nil || f.Name != group.owner || f.Member != field {
						continue
					}
					if opcode != core.OP_GETFIELD || f.Description != descriptor || m.AccessFlags&8 != 0 || !stable || i < 2 {
						return false
					}
					first := constructorMotionMember(child.object, ops[i-1], core.OP_GETFIELD)
					if core.GetRetrieveIdx(ops[i-2]) != 0 || !constructorMotionLoad(ops[i-2], "Ljava/lang/Object;") || first == nil || first.Name != user || first.Member != child.enclosingField || first.Description != "L"+group.owner+";" {
						return false
					}
					base, pc := int(ops[i-2].CurrentOffset), int(op.CurrentOffset)
					for _, entry := range entries {
						if entry > base && entry <= pc {
							return false
						}
					}
					prior := &nativeMemberLexicalRead{owner: user, field: child.enclosingField, descriptor: first.Description, pc: int(ops[i-1].CurrentOffset), basePC: base}
					if seen >= 8192 || work != nil && work.CheckAlloc(int64(seen+1)*256) != nil {
						return false
					}
					reads[pc] = &nativeMemberLexicalRead{owner: group.owner, field: field, descriptor: descriptor, pc: pc, basePC: base, prior: prior}
					seen++
				}
			}
			result[key] = reads
		}
	}
	if seen == 0 {
		return false
	}
	localBytes, anonymousBytes := local.object.Bytes(), child.object.Bytes()
	if len(localBytes) > 1<<20 || len(anonymousBytes) > 1<<20 || !nativeProofWork(work, int64(len(localBytes)+len(anonymousBytes))) || work != nil && work.CheckAlloc(int64(len(localBytes)+len(anonymousBytes))) != nil {
		return false
	}
	if local.anonymousCaptureSeals == nil {
		local.anonymousCaptureSeals = map[string]nativeMethodLocalAnonymousSeal{}
	}
	local.anonymousCaptureSeals[user] = nativeMethodLocalAnonymousSeal{sha256.Sum256(localBytes), sha256.Sum256(anonymousBytes)}
	if local.anonymousCaptureReads == nil {
		local.anonymousCaptureReads = map[string]map[string]map[int]*nativeMemberLexicalRead{}
	}
	if local.anonymousCaptureReads[user] == nil {
		local.anonymousCaptureReads[user] = map[string]map[int]*nativeMemberLexicalRead{}
	}
	for method, reads := range result {
		if local.anonymousCaptureReads[user][method] == nil {
			local.anonymousCaptureReads[user][method] = map[int]*nativeMemberLexicalRead{}
		}
		for pc, r := range reads {
			local.anonymousCaptureReads[user][method][pc] = r
		}
	}
	return true
}
func nativeMethodLocalAnonymousOwner(p *nativeMemberFamily, group *nativeAnonymousFamily, work *workbudget.Budget) bool {
	if p == nil || group == nil || group.failed || p.memberAnonymous[group.owner] != group {
		return false
	}
	local := p.methodLocals[group.owner]
	if local == nil || local.object == nil {
		return false
	}
	_, known := nativeMemberJointMethodLocalOwner(p, local.object, work)
	return known
}
func (c *ClassObjectDumper) wireNativeMethodLocalAnonymousCaptures() {
	p, ctx := c.nativeMemberRoot, c.FuncCtx
	if p == nil || ctx == nil || c.obj == nil {
		return
	}
	group := p.anonymousUnits[c.obj.GetClassName()]
	if !nativeMethodLocalAnonymousOwner(p, group, c.Work) {
		return
	}
	local := p.methodLocals[group.owner]
	child := group.children[c.obj.GetClassName()]
	if child == nil || child.object != c.obj {
		return
	}
	// A lexical name is needed only for an actually certified foreign read.
	// An unrelated outer binding must not forbid a child's legitimate own field.
	needed := map[string]bool{}
	for _, reads := range local.anonymousCaptureReads[c.obj.GetClassName()] {
		for _, read := range reads {
			if !nativeProofWork(c.Work, 1) || read == nil {
				c.nativeCaptureFailed = true
				return
			}
			needed[read.field] = true
		}
	}
	if len(needed) == 0 {
		return
	}
	if !c.nativeMethodLocalAnonymousInputsUnchanged() {
		c.nativeCaptureFailed = true
		return
	}
	// Publish only lexical namespace barriers here. Read/source permission
	// remains tied to its separate original PC and receiver-path certificate.
	if len(local.bindings) > 256 || len(c.nativeAnonymousBindings) > 4096 || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(len(local.bindings)+len(c.nativeAnonymousBindings)+1)*128) != nil {
		c.nativeCaptureFailed = true
		return
	}
	if c.nativeAnonymousBindings == nil {
		c.nativeAnonymousBindings = map[string]string{}
	}
	for field := range needed {
		text, known := local.bindings[field]
		if !known {
			c.nativeCaptureFailed = true
			return
		}
		if !nativeProofWork(c.Work, int64(len(field)+len(text))+1) {
			c.nativeCaptureFailed = true
			return
		}
		if field == local.constructor.enclosingField {
			continue
		}
		if !class_context.PreservesJava8Identifier(text) {
			c.nativeCaptureFailed = true
			return
		}
		key := nativeMemberCaptureIndexKey(group.owner, field)
		if prior, ok := c.nativeAnonymousBindings[key]; ok && prior != text {
			c.nativeCaptureFailed = true
			return
		}
		c.nativeAnonymousBindings[key] = text
	}
	reserved := map[string]bool{}
	for _, name := range c.nativeAnonymousBindings {
		if !nativeProofWork(c.Work, 1) {
			c.nativeCaptureFailed = true
			return
		}
		reserved[name] = true
	}
	// A field shadows a lexical variable throughout its class. Renaming
	// a generated local cannot repair that conflict without changing field ABI.
	for _, field := range c.obj.Fields {
		if field == nil || !nativeProofWork(c.Work, 1) {
			c.nativeCaptureFailed = true
			return
		}
		name, known := sourceBridgeUTF8(c.obj, field.NameIndex)
		if !known {
			c.nativeCaptureFailed = true
			return
		}
		if _, own := child.fields[name]; own {
			continue
		}
		if reserved[name] {
			c.nativeCaptureFailed = true
			return
		}
	}
	priorType, priorSource := ctx.SourceLexicalCapturedFieldType, ctx.SourceLexicalCapturedField
	readSource := func(value any, pc int, name string) (string, bool) {
		read := local.anonymousCaptureReads[c.obj.GetClassName()][ctx.FunctionName+ctx.CurrentMethodDesc][pc]
		if read == nil || read.field != name || !nativeMethodLocalAnonymousOriginalOperand(value, read, c.Work) || !nativeMemberLexicalReadOperand(value, read, c.Work, ctx) {
			return "", false
		}
		text, ok := local.bindings[name]
		ref := local.sourceRefs[name]
		originalName, compilerName := strings.CutPrefix(name, "val$")
		if !ok || text == "" || !compilerName || text != originalName || ref == nil || ref.Id == nil || local.captureIDs[name] != ref.Id {
			return "", false
		}
		current := false
		for _, method := range c.obj.Methods {
			if !nativeProofWork(c.Work, 1) {
				return "", false
			}
			if method == c.CurrentMethod {
				n, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
				d, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
				current = nk && dk && n == ctx.FunctionName && d == ctx.CurrentMethodDesc
				break
			}
		}
		if !current {
			return "", false
		}
		return "(/*jdec-owned-local-capture:" + group.owner + ":" + name + "*/" + text + ")", true
	}
	ctx.SourceLexicalCapturedFieldType = func(value any, pc int, name string) bool {
		if local.anonymousCaptureReads[c.obj.GetClassName()][ctx.FunctionName+ctx.CurrentMethodDesc][pc] != nil {
			_, ok := readSource(value, pc, name)
			return ok
		}
		return priorType != nil && priorType(value, pc, name)
	}
	ctx.SourceLexicalCapturedField = func(value any, pc int, name string) (string, bool) {
		if local.anonymousCaptureReads[c.obj.GetClassName()][ctx.FunctionName+ctx.CurrentMethodDesc][pc] != nil {
			text, ok := readSource(value, pc, name)
			if !ok {
				c.nativeCaptureFailed = true
			}
			return text, ok
		}
		if field, ok := value.(*values.RefMember); ok && field != nil && !sourceProofNil(field.Object) {
			erasure, known := values.SourceTypeErasure(field.Object.Type(), ctx)
			if known && erasure == "L"+group.owner+";" {
				if _, captured := local.constructor.captures[name]; captured {
					c.nativeCaptureFailed = true
					return "", false
				}
			}
		}
		if priorSource != nil {
			return priorSource(value, pc, name)
		}
		return "", false
	}
}

type nativeMethodLocalAnonymousSeal struct{ local, anonymous [32]byte }

func nativeMethodLocalAnonymousOriginalOperand(value any, read *nativeMemberLexicalRead, work *workbudget.Budget) bool {
	v, ok := value.(values.JavaValue)
	if !ok || read == nil {
		return false
	}
	depth := 0
	for r := read; r != nil; r = r.prior {
		depth++
		if depth > 32 || !nativeProofWork(work, 1) {
			return false
		}
		v, ok = nativeMemberEnclosingUnpack(v, work)
		if !ok {
			return false
		}
		f, known := v.(*values.RefMember)
		if !known || !f.OriginalInstanceFieldRead(r.pc, r.owner, r.field, r.descriptor) {
			return false
		}
		v = f.Object
	}
	v, ok = nativeMemberEnclosingUnpack(v, work)
	if !ok {
		return false
	}
	ref, known := v.(*values.JavaRef)
	if !known || ref == nil {
		return false
	}
	slot, sealed := ref.OriginalReceiverSlot()
	return sealed && slot == 0
}

// Original inputs are immutable during a source transaction. Reopen their
// fingerprints at the stage boundary and again before publishing the complete
// body, rather than serializing entire classes for each expression/type query.
func (c *ClassObjectDumper) nativeMethodLocalAnonymousInputsUnchanged() bool {
	if c == nil || c.obj == nil || c.nativeMemberRoot == nil {
		return true
	}
	p := c.nativeMemberRoot
	user := c.obj.GetClassName()
	group := p.anonymousUnits[user]
	if group == nil {
		return true
	}
	local := p.methodLocals[group.owner]
	if local == nil {
		return true
	}
	child := group.children[user]
	if p.failed || group.failed || p.memberAnonymous[group.owner] != group || child == nil || child.object != c.obj {
		return false
	}
	seal, known := local.anonymousCaptureSeals[user]
	if !known {
		return len(local.anonymousCaptureReads[user]) == 0
	}
	original, parent := c.obj.Bytes(), local.object.Bytes()
	return len(original) <= 1<<20 && len(parent) <= 1<<20 && nativeProofWork(c.Work, int64(len(original)+len(parent))) && (c.Work == nil || c.Work.CheckAlloc(int64(len(original)+len(parent))) == nil) && sha256.Sum256(original) == seal.anonymous && sha256.Sum256(parent) == seal.local
}
