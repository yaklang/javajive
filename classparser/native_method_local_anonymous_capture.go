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
	if c == nil || p == nil || local == nil || local.object == nil || local.constructor == nil || field == local.constructor.enclosingField {
		return false
	}
	links, actualLocal, known := c.nativeMethodLocalAnonymousOriginalPath(p, user, work)
	if !known || actualLocal != local || len(links) == 0 {
		return false
	}
	child := links[0]
	captureIndex, ok := local.constructor.captures[field]
	if !ok {
		return false
	}
	caps, _, e := callbinding.Descriptor(local.constructor.descriptor)
	if e != nil || captureIndex < 0 || captureIndex >= len(caps) {
		return false
	}
	descriptor := caps[captureIndex]
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
					if f == nil || f.Name != local.object.GetClassName() || f.Member != field {
						continue
					}
					if opcode != core.OP_GETFIELD || f.Description != descriptor || m.AccessFlags&8 != 0 || !stable || i < len(links)+1 {
						return false
					}
					start := i - len(links) - 1
					if core.GetRetrieveIdx(ops[start]) != 0 || !constructorMotionLoad(ops[start], "Ljava/lang/Object;") {
						return false
					}
					base, pc := int(ops[start].CurrentOffset), int(op.CurrentOffset)
					var prior *nativeMemberLexicalRead
					for offset, link := range links {
						path := constructorMotionMember(child.object, ops[start+1+offset], core.OP_GETFIELD)
						parent, _, ok := originalAnonymousOwner(link.object)
						if !ok || path == nil || path.Name != link.object.GetClassName() || path.Member != link.enclosingField || path.Description != "L"+parent+";" {
							return false
						}
						prior = &nativeMemberLexicalRead{owner: path.Name, field: path.Member, descriptor: path.Description, pc: int(ops[start+1+offset].CurrentOffset), basePC: base, prior: prior}
					}
					for _, entry := range entries {
						if entry > base && entry <= pc {
							return false
						}
					}
					if seen >= 8192 || work != nil && work.CheckAlloc(int64(seen+1)*256) != nil {
						return false
					}
					reads[pc] = &nativeMemberLexicalRead{owner: local.object.GetClassName(), field: field, descriptor: descriptor, pc: pc, basePC: base, prior: prior}
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
	pathSeals := map[*ClassObject][32]byte{}
	for _, link := range links {
		b := link.object.Bytes()
		if len(b) > 1<<20 || !nativeProofWork(work, int64(len(b))) || work != nil && work.CheckAlloc(int64(len(b))) != nil {
			return false
		}
		pathSeals[link.object] = sha256.Sum256(b)
	}
	for current, depth := local.owner.owner, 0; ; depth++ {
		if depth >= 64 || !nativeProofWork(work, 1) {
			return false
		}
		object := p.lexicalObjects[current]
		if object == nil {
			return false
		}
		b := object.Bytes()
		if len(b) > 1<<20 || !nativeProofWork(work, int64(len(b))) {
			return false
		}
		pathSeals[object] = sha256.Sum256(b)
		if current == p.owner {
			break
		}
		node := p.children[current]
		if node == nil {
			return false
		}
		current = node.owner
	}
	var total int64
	for object := range pathSeals {
		total += int64(len(object.Bytes()))
		if work != nil && work.CheckAlloc(total) != nil {
			return false
		}
	}
	if local.anonymousCaptureSeals == nil {
		local.anonymousCaptureSeals = map[string]nativeMethodLocalAnonymousSeal{}
	}
	local.anonymousCaptureSeals[user] = nativeMethodLocalAnonymousSeal{local: sha256.Sum256(localBytes), anonymous: sha256.Sum256(anonymousBytes), path: pathSeals}
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
	if p == nil || group == nil || group.failed {
		return false
	}
	if local := p.methodLocals[group.owner]; local != nil {
		_, ok := nativeMemberJointMethodLocalOwner(p, local.object, work)
		return ok && p.memberAnonymous[group.owner] == group
	}
	if group.forest == nil || group.forest != p.anonymousForest || group.forest.members != p || group.forest.groups[group.owner] != group {
		return false
	}
	local, known := nativeMethodLocalAnonymousLocal(p, group.owner, work)
	return known && local != nil
}
func nativeMethodLocalAnonymousLocal(p *nativeMemberFamily, user string, work *workbudget.Budget) (*nativeMethodLocalClass, bool) {
	if p == nil {
		return nil, false
	}
	seen := map[string]bool{}
	for current := user; len(seen) < 64; {
		if seen[current] || !nativeProofWork(work, 1) {
			return nil, false
		}
		seen[current] = true
		if local := p.methodLocals[current]; local != nil {
			if _, ok := nativeMemberJointMethodLocalOwner(p, local.object, work); ok {
				return local, true
			}
			return nil, false
		}
		if current == p.owner || p.children[current] != nil {
			return nil, true
		}
		group := p.anonymousUnits[current]
		if group == nil || group.failed {
			return nil, false
		}
		child := group.children[current]
		if child == nil || child.object == nil || (group.forest != nil && p.lexicalObjects[current] != child.object) {
			return nil, false
		}
		parent, method, ok := originalAnonymousOwner(child.object)
		if !ok || parent != group.owner || method != child.method {
			return nil, false
		}
		if group.forest != nil && (group.forest != p.anonymousForest || group.forest.members != p || group.forest.groups[parent] != group || group.forest.units[current] != child) {
			return nil, false
		}
		current = parent
	}
	return nil, false
}
func (c *ClassObjectDumper) wireNativeMethodLocalAnonymousCaptures() {
	p, ctx := c.nativeMemberRoot, c.FuncCtx
	if p == nil || ctx == nil || c.obj == nil {
		return
	}
	group := p.anonymousUnits[c.obj.GetClassName()]
	if group == nil {
		return
	}
	local, knownLocal := nativeMethodLocalAnonymousLocal(p, c.obj.GetClassName(), c.Work)
	if !knownLocal {
		c.nativeCaptureFailed = true
		return
	}
	if local == nil {
		return
	}
	if !nativeMethodLocalAnonymousOwner(p, group, c.Work) {
		c.nativeCaptureFailed = true
		return
	}
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
		key := nativeMemberCaptureIndexKey(local.object.GetClassName(), field)
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
		return c.nativeMethodLocalAnonymousCaptureSource(local, value, pc, name, ctx)
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
			if known && erasure == "L"+local.object.GetClassName()+";" {
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

type nativeMethodLocalAnonymousSeal struct {
	local, anonymous [32]byte
	path             map[*ClassObject][32]byte
}

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
	local, knownLocal := nativeMethodLocalAnonymousLocal(p, c.obj.GetClassName(), c.Work)
	if !knownLocal {
		return false
	}
	if local == nil {
		return true
	}
	child := group.children[user]
	if p.failed || group.failed || (group.forest == nil && p.memberAnonymous[group.owner] != group || group.forest != nil && (group.forest != p.anonymousForest || group.forest.groups[group.owner] != group)) || child == nil || child.object != c.obj {
		return false
	}
	seal, known := local.anonymousCaptureSeals[user]
	if !known {
		return len(local.anonymousCaptureReads[user]) == 0
	}
	if reader := NewClassObjectDumper(c.obj); reader != nil {
		reader.options = c.options
		reader.Work = c.Work
		reader.foldSiblingResolver = c.foldSiblingResolver
		reader.declarationResolver = c.declarationResolver
		_, actual, ok := reader.nativeMethodLocalAnonymousOriginalPath(p, user, c.Work)
		if !ok || actual != local {
			return false
		}
	}
	var total int64
	for object, want := range seal.path {
		if object == nil || !nativeMethodLocalAnonymousSealedObject(p, object) {
			return false
		}
		b := object.Bytes()
		total += int64(len(b))
		if len(b) > 1<<20 || !nativeProofWork(c.Work, int64(len(b))) || c.Work != nil && c.Work.CheckAlloc(total) != nil || sha256.Sum256(b) != want {
			return false
		}
	}
	original, parent := c.obj.Bytes(), local.object.Bytes()
	return len(original) <= 1<<20 && len(parent) <= 1<<20 && nativeProofWork(c.Work, int64(len(original)+len(parent))) && (c.Work == nil || c.Work.CheckAlloc(int64(len(original)+len(parent))) == nil) && sha256.Sum256(original) == seal.anonymous && sha256.Sum256(parent) == seal.local
}

// Reopen each original lexical edge and physical constructor packet. The
// result orders the real THIS/enclosing fields from the caller to its local
// ancestor; discovering this path alone grants no source name or access.
func (c *ClassObjectDumper) nativeMethodLocalAnonymousOriginalPath(p *nativeMemberFamily, user string, work *workbudget.Budget) ([]*nativeAnonymousClass, *nativeMethodLocalClass, bool) {
	if c == nil || p == nil || p.failed || work != nil && work.CheckAlloc(64*512) != nil {
		return nil, nil, false
	}
	links := []*nativeAnonymousClass{}
	seen := map[string]bool{}
	for current := user; len(seen) < 64; {
		if seen[current] || !nativeProofWork(work, 1) {
			return nil, nil, false
		}
		seen[current] = true
		if local := p.methodLocals[current]; local != nil {
			if _, ok := nativeMemberJointMethodLocalOwner(p, local.object, work); !ok || len(links) == 0 {
				return nil, nil, false
			}
			return links, local, true
		}
		group := p.anonymousUnits[current]
		if group == nil || group.failed {
			return nil, nil, false
		}
		child := group.children[current]
		if child == nil || child.object == nil || child.object.GetClassName() != current || (group.forest != nil && p.lexicalObjects[current] != child.object) || child.enclosingField == "" {
			return nil, nil, false
		}
		owner, method, ok := originalAnonymousOwner(child.object)
		if !ok || owner != group.owner || method != child.method {
			return nil, nil, false
		}
		if group.forest != nil {
			f := group.forest
			if f != p.anonymousForest || f.members != p || f.groups[owner] != group || f.units[current] != child || f.objects[current] != child.object {
				return nil, nil, false
			}
		} else if p.memberAnonymous[owner] != group || p.methodLocals[owner] == nil || !p.methodLocals[owner].declaredAnonymous[current] {
			return nil, nil, false
		}
		parent := p.lexicalObjects[owner]
		if parent == nil || parent.GetClassName() != owner {
			return nil, nil, false
		}
		rows := 0
		for _, a := range parent.Attributes {
			if !nativeProofWork(work, 1) {
				return nil, nil, false
			}
			if table, ok := a.(*InnerClassesAttribute); ok {
				if table == nil {
					return nil, nil, false
				}
				for _, row := range table.Classes {
					if row == nil || !nativeProofWork(work, 1) {
						return nil, nil, false
					}
					name, ok := sourceBridgeClassName(parent, row.InnerClassInfoIndex)
					if !ok {
						return nil, nil, false
					}
					if name == current {
						rows++
						if row.InnerNameIndex != 0 || row.OuterClassInfoIndex != 0 || !nativeAnonymousSelfAccessFlags(child.object, row.InnerClassAccessFlags, work) {
							return nil, nil, false
						}
					}
				}
			}
		}
		if rows != 1 {
			return nil, nil, false
		}
		parameter, ok := child.fields[child.enclosingField]
		ps, _, bad := callbinding.Descriptor(child.descriptor)
		if !ok || parameter != 0 || bad != nil || len(ps) == 0 || ps[0] != "L"+owner+";" {
			return nil, nil, false
		}
		reader := NewClassObjectDumper(parent)
		reader.Work = work
		reader.options = c.options
		reader.foldSiblingResolver = c.foldSiblingResolver
		reader.declarationResolver = c.declarationResolver
		actual := reader.nativeAnonymousConstructorForCompiler(child.object, owner, method, "", p, group.forest, reader.buildInvocationMetadata(), reader.originalNativeConstructorAccessBridges())
		if actual == nil || actual.descriptor != child.descriptor || actual.enclosingField != child.enclosingField || actual.superDescriptor != child.superDescriptor || actual.superPC != child.superPC || actual.sourceSuperDescriptor != child.sourceSuperDescriptor || actual.sharedParameterRoles != child.sharedParameterRoles || actual.unusedEnclosing != child.unusedEnclosing || actual.parentAnonymous != child.parentAnonymous || len(actual.superParams) != len(child.superParams) || len(actual.fields) != len(child.fields) || len(actual.capturePCs) != len(child.capturePCs) {
			return nil, nil, false
		}
		for i, x := range actual.superParams {
			if !nativeProofWork(work, 1) || x != child.superParams[i] {
				return nil, nil, false
			}
		}
		for field, x := range actual.fields {
			if !nativeProofWork(work, 1) {
				return nil, nil, false
			}
			y, ok := child.fields[field]
			if !ok || x != y || actual.capturePCs[field] != child.capturePCs[field] {
				return nil, nil, false
			}
		}
		links = append(links, child)
		current = owner
	}
	return nil, nil, false
}

func nativeMethodLocalAnonymousSealedObject(p *nativeMemberFamily, object *ClassObject) bool {
	if p == nil || object == nil {
		return false
	}
	if p.lexicalObjects[object.GetClassName()] == object {
		return true
	}
	group := p.anonymousUnits[object.GetClassName()]
	if group == nil || group.failed || group.forest != nil || p.memberAnonymous[group.owner] != group || p.methodLocals[group.owner] == nil {
		return false
	}
	child := group.children[object.GetClassName()]
	return child != nil && child.object == object
}

// Source permission comes from the original read and declaration identities,
// including when the generic forest renderer reaches this local capture.
func (c *ClassObjectDumper) nativeMethodLocalAnonymousCaptureSource(local *nativeMethodLocalClass, value any, pc int, name string, ctx *class_context.ClassContext) (string, bool) {
	if c == nil || c.obj == nil || ctx == nil || c.nativeCaptureFailed || local == nil || local.object == nil || c.nativeMemberRoot == nil || c.nativeMemberRoot.failed || c.nativeMemberRoot.methodLocals[local.object.GetClassName()] != local {
		return "", false
	}
	read := local.anonymousCaptureReads[c.obj.GetClassName()][ctx.FunctionName+ctx.CurrentMethodDesc][pc]
	if read == nil || read.field != name || !nativeMethodLocalAnonymousOriginalOperand(value, read, c.Work) || !nativeMemberLexicalReadOperand(value, read, c.Work, ctx) {
		return "", false
	}
	text, ok := local.bindings[name]
	ref := local.sourceRefs[name]
	originalName, compilerName := strings.CutPrefix(name, "val$")
	if !ok || text == "" || !compilerName || text != originalName || ref == nil || ref.Id == nil || local.captureIDs[name] != ref.Id || c.nativeAnonymousBindings[nativeMemberCaptureIndexKey(local.object.GetClassName(), name)] != text {
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
	return "(/*jdec-owned-local-capture:" + local.object.GetClassName() + ":" + name + "*/" + text + ")", true
}
