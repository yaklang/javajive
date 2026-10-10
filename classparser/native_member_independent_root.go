package javaclassparser

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A static declaration cuts the enclosing-instance and free-type-variable
// paths. It can therefore retain its existing independent binary declaration
// while owning its own named children. Its original outer remains metadata,
// never a member of this source/private scope.
type nativeMemberIndependentRoot struct {
	object       *ClassObject
	lexicalOwner string
	declaration  *nativeMemberClass // completed binding view; not family membership
}

func (c *ClassObjectDumper) originalNativeMemberIndependentRoot() *nativeMemberIndependentRoot {
	if c == nil || c.obj == nil || c.foldSiblingResolver == nil {
		return nil
	}
	obj := c.obj
	owner, name, flags, known := originalMemberOwner(obj)
	if !known || owner == obj.GetClassName() || flags&8 == 0 || flags&6 != 0 || obj.MinorVersion != 0 || obj.MajorVersion < 49 || obj.MajorVersion > 52 {
		return nil
	}
	// Ordinary class or proved static interface headers only. Do not transfer
	// enum, synthetic, assertion-status, modern-nest or private-root protocols.
	header := obj.AccessFlags
	ordinary := header & ^uint16(0x0431) == 0 && header&0x20 != 0 && header&0x410 != 0x410
	staticInterface := flags&0x600 == 0x600 && nativeMemberDeclarationKindRepresentable(obj, flags, c.Work)
	if !(ordinary || staticInterface) || flags != (header & ^uint16(0x20))|8 || !nativeMemberVersionMetadata(obj, c.Work) || !nativeSourceBinaryName(obj.GetClassName()) || !c.nativeMemberAnnotationTablesRepresentable() || !nativeMemberTypeScope(obj, nil, c.Work) {
		return nil
	}
	packet, valid := nativeMemberAssertionProof(obj, owner, c.Work)
	if !valid || packet != nil || !nativeMemberIndependentAssertionReferencesClosed(obj, c.Work) {
		return nil
	}
	for _, fields := range [][]*MemberInfo{obj.Fields, obj.Methods} {
		for _, field := range fields {
			if field == nil || !nativeProofWork(c.Work, 1) || field.AccessFlags&2 != 0 {
				return nil
			}
		}
	}
	raw, found := c.foldSiblingResolver(owner)
	if !found {
		return nil
	}
	outer, err := c.parseResolved(raw)
	if err != nil || outer.GetClassName() != owner {
		return nil
	}
	rows := 0
	for _, attr := range outer.Attributes {
		table, ok := attr.(*InnerClassesAttribute)
		if !ok {
			continue
		}
		if table == nil {
			return nil
		}
		for _, row := range table.Classes {
			if row == nil || !nativeProofWork(c.Work, 1) {
				return nil
			}
			binary, known := sourceBridgeClassName(outer, row.InnerClassInfoIndex)
			if !known {
				return nil
			}
			if binary != obj.GetClassName() {
				continue
			}
			parent, pk := sourceBridgeClassName(outer, row.OuterClassInfoIndex)
			simple, nk := sourceBridgeUTF8(outer, row.InnerNameIndex)
			if !pk || !nk || parent != owner || simple != name || row.InnerClassAccessFlags != flags {
				return nil
			}
			rows++
		}
	}
	if rows != 1 {
		return nil
	}
	declaration := nativeMemberProofWithDeclarations(obj, outer, c.Work, c.originalNativeConstructorAccessBridges(), nil, c.nativeAnnotationDeclarationResolver(), c.buildInvocationMetadata())
	if declaration == nil || !declaration.static || declaration.owner != owner || declaration.name != name || declaration.flags != flags {
		return nil
	}
	declaration.sourceName = strings.ReplaceAll(obj.GetClassName(), "/", ".")
	return &nativeMemberIndependentRoot{object: obj, lexicalOwner: owner, declaration: declaration}
}

func (p *nativeMemberIndependentRoot) validFor(c *ClassObjectDumper) bool {
	if p == nil || p.object == nil || p.declaration == nil || c == nil || c.obj != p.object || p.declaration.object != p.object || p.declaration.owner != p.lexicalOwner || !p.declaration.static || p.declaration.sourceName != strings.ReplaceAll(p.object.GetClassName(), "/", ".") {
		return false
	}
	fresh := c.originalNativeMemberIndependentRoot()
	return fresh != nil && fresh.lexicalOwner == p.lexicalOwner && fresh.declaration.flags == p.declaration.flags && fresh.declaration.name == p.declaration.name
}

func (p *nativeMemberIndependentRoot) familyClosed(f *nativeMemberFamily, work *workbudget.Budget) bool {
	if p == nil || p.object == nil || f == nil || f.independentRoot != p || f.owner != p.object.GetClassName() || f.lexicalObjects[f.owner] != p.object || f.lexicalObjects[p.lexicalOwner] != nil || len(f.modernNestObjects) != 0 || len(f.enumConstants) != 0 || len(f.enumSwitchTables) != 0 || len(f.methodLocals) != 0 || len(f.anonymousUnits) != 0 || f.anonymousForest != nil || f.anonymous != nil {
		return false
	}
	for _, group := range f.memberAnonymous {
		if group != nil {
			return false
		}
	}
	for _, child := range f.children {
		if child == nil || child.enumSynthesis != nil || child.assertions != nil || child.object == nil || child.object.MinorVersion != 0 || child.object.MajorVersion < 49 || child.object.MajorVersion > 52 || !nativeProofWork(work, 1) {
			return false
		}
		packet, known := nativeMemberAssertionProof(child.object, p.lexicalOwner, work)
		if !known || packet != nil || !nativeMemberIndependentAssertionReferencesClosed(child.object, work) {
			return false
		}
	}
	return true
}

// A helper-owned assertion field also carries lexical status, even when the
// current interface/class has no local synthetic field. Its status owner needs
// a separate compiler proof before an independent source scope may use it.
func nativeMemberIndependentAssertionReferencesClosed(obj *ClassObject, work *workbudget.Budget) bool {
	if obj == nil {
		return false
	}
	for _, constant := range obj.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		ref, ok := constant.(*ConstantFieldrefInfo)
		if !ok {
			continue
		}
		if ref == nil || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(obj.ConstantPool) {
			return false
		}
		symbol, ok := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || symbol == nil {
			return false
		}
		name, known := sourceBridgeUTF8(obj, symbol.NameIndex)
		if !known || name == nativeAssertionField {
			return false
		}
	}
	return true
}

// Find a static boundary using original self rows. Binary dollar spelling is
// never evidence of ownership. Choose the outermost static cut on that path:
// a nearer static child must not be independently emitted while the parent
// source family regenerates it. The existing depth and cycle bounds apply.
func (z *JarFS) nativeMemberIndependentObject(obj *ClassObject) *ClassObject {
	reader := z.nativeMemberReader(obj)
	seen := map[string]bool{}
	var boundary *ClassObject
	for depth := 0; depth < 64 && obj != nil; depth++ {
		binary := obj.GetClassName()
		if seen[binary] || !nativeProofWork(reader.Work, 1) {
			return nil
		}
		seen[binary] = true
		owner, _, flags, known := originalMemberOwner(obj)
		if !known {
			if nativeMemberTopLevelEvidence(obj, reader.Work) {
				return boundary
			}
			return nil
		}
		if flags&8 != 0 {
			boundary = obj
		}
		raw, found := z.enumSiblingResolver()(owner)
		if !found {
			return nil
		}
		var err error
		obj, err = reader.parseResolved(raw)
		if err != nil || obj.GetClassName() != owner {
			return nil
		}
	}
	return nil
}

func (z *JarFS) nativeMemberIndependentEntry(obj *ClassObject) *nativeMemberCacheEntry {
	root := z.nativeMemberIndependentObject(obj)
	if root == nil {
		return nil
	}
	snap, bound := jdecenv.Current()
	if !bound || snap == nil {
		snap = snapshotJDECEnv()
	}
	if snap["JDEC_NATIVE_MEMBER_OFF"] != "" {
		return nil
	}
	reader := z.nativeMemberReader(root)
	reader.options.EnvSnapshot = snap
	certificate := reader.originalNativeMemberIndependentRoot()
	if certificate == nil {
		return nil
	}
	policy, _ := json.Marshal(snap)
	entry := z.nativeMemberPolicyEntry("independent-static\x00" + root.GetClassName() + "\x00" + string(policy))
	if entry == nil {
		return nil
	}
	entry.once.Do(func() {
		prepared := z.prepareNativeMemberFamilyFromRoot(root, snap, certificate)
		// This source scope cannot recursively claim another independent scope or
		// import metadata-only ancestors through a dependency transaction.
		result := z.finishNativeMemberFamily(prepared, z.nativeMemberStandardLookup, false)
		if result == nil {
			return
		}
		src := result.source
		if reader.Work != nil && reader.Work.CheckAlloc(int64(len(src))) != nil {
			return
		}
		if !z.reserveOwnershipSource(int64(len(src))) {
			return
		}
		z.nativeMembersMu.Lock()
		if int64(len(src)) > (16<<20)-z.nativeMembersBytes {
			z.nativeMembersMu.Unlock()
			return
		}
		z.nativeMembersBytes += int64(len(src))
		z.nativeMembersMu.Unlock()
		entry.family = result.family
		entry.source = src
	})
	return entry
}

// A proved independent static boundary retains its original ClassFile
// visibility. Its physical outer remains metadata, so the broad flat-unit
// visibility policy must not widen this separately proved source scope.
func (c *ClassObjectDumper) nativeMemberIndependentSourceHeader() bool {
	if c == nil || c.obj == nil || c.nativeMemberCurrent != nil || c.nativeMemberRoot == nil {
		return false
	}
	p := c.nativeMemberRoot
	return p.owner == c.obj.GetClassName() && p.independentRoot != nil && p.independentRoot.validFor(c) && p.independentRoot.familyClosed(p, c.Work)
}
