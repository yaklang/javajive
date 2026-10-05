package javaclassparser

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
	"io/fs"
	"strings"
	"sync"
)

type nativeMemberHandleTarget struct {
	kind             uint8
	methodRef        bool
	name, descriptor string
}

type nativeMemberIndex struct {
	once                    sync.Once
	valid                   bool
	constructors            map[string]map[string]bool
	captureUsers            map[string]map[string]bool
	typeUsers               map[string]map[string]bool
	handles                 map[string]bool
	handleTargets           map[string][]nativeMemberHandleTarget
	getterUsers             map[string]map[string]bool
	getterHandles           map[string]bool
	getterInvalidReferences map[string]bool
}
type nativeMemberCacheEntry struct {
	once                   sync.Once
	planOnce               sync.Once
	transaction            *nativeMemberTransaction // guarded by nativeMembersMu
	planKnown              bool                     // published by planOnce; independent of source completion
	dependenciesOnce       sync.Once
	dependenciesKnown      bool
	dependenciesCyclic     bool
	dependencyParticipants []string // immutable own component; no retained class objects/graph
	family                 *nativeMemberFamily
	source                 string
}

func (z *JarFS) nativeMemberReader(obj *ClassObject) *ClassObjectDumper {
	if obj == nil {
		obj = &ClassObject{}
	}
	d := NewClassObjectDumper(obj)
	d.foldSiblingResolver = z.enumSiblingResolver()
	d.declarationResolver = z.declarationResolver
	if z.archive != nil && z.archive.budget != nil {
		d.Work = z.archive.budget.Work()
		d.options.Context = z.archive.ctx
		d.options.TargetRelease = z.archive.targetRelease
	}
	return d
}
func (z *JarFS) originalMemberIndex() *nativeMemberIndex {
	idx := &z.nativeMembersIndex
	idx.once.Do(func() {
		idx.constructors = map[string]map[string]bool{}
		idx.captureUsers = map[string]map[string]bool{}
		idx.typeUsers = map[string]map[string]bool{}
		idx.handles = map[string]bool{}
		idx.handleTargets = map[string][]nativeMemberHandleTarget{}
		idx.getterUsers = map[string]map[string]bool{}
		idx.getterHandles = map[string]bool{}
		idx.getterInvalidReferences = map[string]bool{}
		total, classes, edges := int64(0), 0, 0
		seenClasses := map[string]bool{}
		e := fs.WalkDir(z.ZipFS, ".", func(path string, entry fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if entry.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".class") {
				return nil
			}
			logical := path
			if strings.HasPrefix(path, "META-INF/") {
				release, candidate, known := physicalClassNamespace(path)
				if !known || release > z.ZipFS.TargetRelease() {
					return nil
				}
				logical = candidate
			}
			if seenClasses[logical] {
				return nil
			}
			seenClasses[logical] = true
			classes++
			if classes > 16384 {
				return fmt.Errorf("member index class limit")
			}
			raw, e := z.ZipFS.ReadFile(logical)
			if e != nil {
				return e
			}
			total += int64(len(raw))
			if total > 128<<20 {
				return fmt.Errorf("member index byte limit")
			}
			reader := z.nativeMemberReader(nil)
			if reader.Work != nil && reader.Work.CheckAlloc(int64(len(raw))) != nil {
				return fmt.Errorf("member index budget")
			}
			obj, e := reader.parseResolved(raw)
			if e != nil || obj.GetClassName()+".class" != logical {
				return fmt.Errorf("member index identity")
			}
			referencer := obj.GetClassName()
			record := func(table map[string]map[string]bool, owner string) bool {
				if table[owner][referencer] {
					return true
				}
				edges++
				if edges > 1<<20 || !nativeProofWork(reader.Work, 1) || reader.Work != nil && reader.Work.CheckAlloc(int64(edges)*96) != nil {
					return false
				}
				if table[owner] == nil {
					table[owner] = map[string]bool{}
				}
				table[owner][referencer] = true
				return true
			}
			references, closed := nativeMemberDependencyNames(obj, reader.Work)
			if !closed {
				return fmt.Errorf("member index type closure")
			}
			for _, name := range references {
				if !record(idx.typeUsers, name) {
					return fmt.Errorf("member index type edge limit")
				}
			}
			for _, constant := range obj.ConstantPool {
				if !nativeProofWork(reader.Work, 1) {
					return fmt.Errorf("member index work")
				}
				member := nativeConstantMember(constant)
				if member == nil {
					continue
				}
				owner, ok := sourceBridgeClassName(obj, member.ClassIndex)
				if !ok {
					return fmt.Errorf("member index owner")
				}
				if member.NameAndTypeIndex == 0 || int(member.NameAndTypeIndex) > len(obj.ConstantPool) {
					return fmt.Errorf("member index reference")
				}
				nt, ok := obj.ConstantPool[member.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !ok || nt == nil {
					return fmt.Errorf("member index name type")
				}
				name, ok := sourceBridgeUTF8(obj, nt.NameIndex)
				if !ok {
					return fmt.Errorf("member index member")
				}
				switch constant.(type) {
				case *ConstantMethodrefInfo, *ConstantInterfaceMethodrefInfo:
					if strings.HasPrefix(name, "access$") {
						desc, known := sourceBridgeUTF8(obj, nt.DescriptorIndex)
						if _, normalMethodRef := constant.(*ConstantMethodrefInfo); !normalMethodRef {
							idx.getterInvalidReferences[nativeMemberGetterKey(owner, name, desc)] = true
						}
						if !known || !record(idx.getterUsers, nativeMemberGetterKey(owner, name, desc)) {
							return fmt.Errorf("member getter reference")
						}
					}
					if name == "<init>" {
						if !record(idx.constructors, owner) {
							return fmt.Errorf("member index edge limit")
						}
					}
				case *ConstantFieldrefInfo:
					if nativeMemberCaptureIndexName(name) {
						if !record(idx.captureUsers, nativeMemberCaptureIndexKey(owner, name)) {
							return fmt.Errorf("member index edge limit")
						}
					}
				}
			}
			for _, constant := range obj.ConstantPool {
				handle, ok := constant.(*ConstantMethodHandleInfo)
				if !ok || handle == nil {
					continue
				}
				if handle.ReferenceIndex == 0 || int(handle.ReferenceIndex) > len(obj.ConstantPool) {
					return fmt.Errorf("member index handle")
				}
				member := nativeConstantMember(obj.ConstantPool[handle.ReferenceIndex-1])
				if member == nil {
					return fmt.Errorf("member index handle kind")
				}
				owner, known := sourceBridgeClassName(obj, member.ClassIndex)
				if !known {
					return fmt.Errorf("member index handle owner")
				}
				idx.handles[owner] = true
				if member.NameAndTypeIndex == 0 || int(member.NameAndTypeIndex) > len(obj.ConstantPool) {
					return fmt.Errorf("member handle name type")
				}
				nt, valid := obj.ConstantPool[member.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !valid || nt == nil {
					return fmt.Errorf("member handle name type")
				}
				name, nok := sourceBridgeUTF8(obj, nt.NameIndex)
				desc, dok := sourceBridgeUTF8(obj, nt.DescriptorIndex)
				if !nok || !dok {
					return fmt.Errorf("member handle symbol")
				}
				_, methodRef := obj.ConstantPool[handle.ReferenceIndex-1].(*ConstantMethodrefInfo)
				edges++
				if edges > 1<<20 || len(idx.handleTargets[owner]) >= 4096 || !nativeProofWork(reader.Work, 1) || reader.Work != nil && reader.Work.CheckAlloc(int64(len(idx.handleTargets[owner])+1)*96) != nil {
					return fmt.Errorf("member handle target limit")
				}
				idx.handleTargets[owner] = append(idx.handleTargets[owner], nativeMemberHandleTarget{kind: handle.ReferenceKind, methodRef: methodRef, name: name, descriptor: desc})
				idx.getterHandles[nativeMemberGetterKey(owner, name, desc)] = true
			}
			return nil
		})
		idx.valid = e == nil
	})
	return idx
}
func (z *JarFS) nativeMemberEntry(obj *ClassObject) *nativeMemberCacheEntry {
	owner, _, _, member := originalMemberOwner(obj)
	if anonymousOwner, _, anonymous := originalAnonymousOwner(obj); anonymous {
		reader := z.nativeMemberReader(obj)
		var known bool
		owner, known = z.nativeAnonymousOutermostOwner(anonymousOwner, reader)
		if !known {
			return nil
		}
		member = true
	}

	if !member {
		owner = obj.GetClassName()
	}
	if member {
		var known bool
		owner, known = z.nativeMemberOutermostNamedOwner(owner, z.nativeMemberReader(obj).Work)
		if !known {
			return nil
		}
	}
	if !member {
		candidate := false
		for _, a := range obj.Attributes {
			if inner, ok := a.(*InnerClassesAttribute); ok && inner != nil {
				for _, row := range inner.Classes {
					if row == nil {
						return nil
					}
					outer, known := sourceBridgeClassName(obj, row.OuterClassInfoIndex)
					if known && outer == owner && row.InnerNameIndex != 0 {
						candidate = true
					}
				}
			}
		}
		if !candidate {
			return nil
		}
	}
	snap, bound := jdecenv.Current()
	if !bound || snap == nil {
		snap = snapshotJDECEnv()
	}
	if snap["JDEC_NATIVE_MEMBER_OFF"] != "" {
		return nil
	}
	policy, _ := json.Marshal(snap)
	key := owner + "\x00" + string(policy)
	entry := z.nativeMemberPolicyEntry(key)
	if entry == nil {
		return nil
	}
	entry.once.Do(func() {
		var prepared *nativeMemberPrepared
		entry.planOnce.Do(func() {
			prepared = z.nativeMemberLocalPlan(obj, owner, snap)
			entry.planKnown = prepared != nil
		})
		if !entry.planKnown {
			return
		}
		if prepared == nil {
			prepared = z.nativeMemberLocalPlan(obj, owner, snap)
		}
		result := z.finishNativeMemberFamily(prepared, z.nativeMemberLookup, false)
		if result == nil {
			return
		}
		d, p, src := prepared.reader, result.family, result.source
		if d.Work != nil && d.Work.CheckAlloc(int64(len(src))) != nil {
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
		entry.family = p
		entry.source = src
	})
	return entry
}

func (z *JarFS) nativeMemberPolicyEntry(key string) *nativeMemberCacheEntry {
	z.nativeMembersMu.Lock()
	defer z.nativeMembersMu.Unlock()
	entry := z.nativeMembersCache[key]
	if entry == nil {
		if len(z.nativeMembersCache)+len(z.nativeMemberTransactions) >= 512 {
			return nil
		}
		if z.nativeMembersCache == nil {
			z.nativeMembersCache = map[string]*nativeMemberCacheEntry{}
		}
		entry = &nativeMemberCacheEntry{}
		z.nativeMembersCache[key] = entry
	}
	return entry
}

// Admission is a pure planning phase: it neither follows source cache entries
// nor reserves/publishes source. Rendering always uses its own fresh mutable
// plan after another caller has consumed the original admission attempt.
func (z *JarFS) nativeMemberLocalPlan(obj *ClassObject, owner string, snap map[string]string) *nativeMemberPrepared {
	root := obj
	if root == nil || root.GetClassName() != owner {
		raw, known := z.enumSiblingResolver()(owner)
		if !known {
			return nil
		}
		var err error
		root, err = z.nativeMemberReader(obj).parseResolved(raw)
		if err != nil || root.GetClassName() != owner {
			return nil
		}
	}
	return z.prepareNativeMemberFamily(root, snap)
}

// Every body emitted in the joint source unit contributes binding dependencies,
// including anonymous declarations whose referenced types occur only in a
// descriptor or Signature. Reuse the already proved ownership; do not consult
// the family cache recursively or import an external class into its private nest.
func nativeMemberDependencyObjects(root *ClassObject, p *nativeMemberFamily, work *workbudget.Budget) ([]*ClassObject, bool) {
	if root == nil || p == nil || p.failed || root.GetClassName() != p.owner ||
		len(p.children) > 64 || len(p.anonymousUnits) > 64 ||
		!nativeProofWork(work, int64(len(p.children)+len(p.anonymousUnits)+1)) ||
		work != nil && work.CheckAlloc(int64(len(p.children)+len(p.anonymousUnits)+1)*128) != nil {
		return nil, false
	}
	objects := []*ClassObject{root}
	seen := map[string]bool{p.owner: true}
	add := func(name string, object *ClassObject) bool {
		if object == nil || object.GetClassName() != name || seen[name] {
			return false
		}
		seen[name] = true
		objects = append(objects, object)
		return true
	}
	for name, child := range p.children {
		if child == nil || !add(name, child.object) {
			return nil, false
		}
	}
	for name, group := range p.anonymousUnits {
		if group == nil || group.failed || group.children[name] == nil || !add(name, group.children[name].object) {
			return nil, false
		}
	}
	return objects, true
}
func (z *JarFS) nativeMemberLookup(name string) *nativeMemberClass {
	name = strings.ReplaceAll(name, ".", "/")
	if !strings.Contains(name, "$") {
		return nil
	}
	raw, known := z.enumSiblingResolver()(name)
	if !known {
		return nil
	}
	reader := z.nativeMemberReader(nil)
	obj, e := reader.parseResolved(raw)
	if e != nil || obj.GetClassName() != name {
		return nil
	}
	if _, _, _, known := originalMemberOwner(obj); !known {
		return nil
	}
	entry := z.nativeMemberEntry(obj)
	if entry == nil || entry.family == nil {
		entry = z.nativeMemberTransactionEntry(obj)
	}
	if entry == nil || entry.family == nil {
		return nil
	}
	return entry.family.children[name]
}
func (z *JarFS) nativeMemberSource(obj *ClassObject) ([]byte, bool) {
	entry := z.nativeMemberEntry(obj)
	if entry == nil || entry.family == nil {
		entry = z.nativeMemberTransactionEntry(obj)
	}
	if entry == nil || entry.family == nil {
		return nil, false
	}
	if z.archive != nil && z.archive.budget != nil {
		work := z.archive.budget.Work()
		if work.CheckAlloc(int64(len(entry.source))) != nil || work.CheckOutput(int64(len(entry.source))) != nil {
			return nil, false
		}
	}
	if obj.GetClassName() == entry.family.owner {
		return []byte(entry.source), true
	}
	if entry.family.children[obj.GetClassName()] != nil {
		return []byte("// original member body owned by " + entry.family.owner + "; javac regenerates its binary class\n"), true
	}
	if entry.family.emptyMarkers[obj.GetClassName()] != nil {
		return []byte("// original private-constructor marker body owned by " + entry.family.owner + "; javac regenerates its binary class\n"), true
	}
	if group := entry.family.anonymousUnits[obj.GetClassName()]; group != nil {
		return []byte("// original anonymous body owned by " + group.owner + "; javac regenerates its binary class\n"), true
	}
	return nil, false
}

// Scope dependencies also occur only in field/method descriptors or Signature
// bounds. A CONSTANT_Class-only closure misses those source type bindings.
func nativeMemberDependencyNames(obj *ClassObject, work *workbudget.Budget) ([]string, bool) {
	names := []string{}
	seen := map[string]bool{}
	signatures := map[string]bool{}
	add := func(n string) {
		n = strings.ReplaceAll(n, ".", "/")
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	signature := func(text string) bool {
		if signatures[text] {
			return true
		}
		signatures[text] = true
		if !nativeProofWork(work, int64(len(text))) {
			return false
		}
		refs, known := types.SignatureClassReferences(text)
		if !known {
			return false
		}
		for _, n := range refs {
			add(n)
		}
		return true
	}
	for _, constant := range obj.ConstantPool {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if cls, ok := constant.(*ConstantClassInfo); ok && cls != nil {
			n, known := sourceBridgeUTF8(obj, cls.NameIndex)
			if !known {
				return nil, false
			}
			if strings.HasPrefix(n, "[") {
				if !signature(n) {
					return nil, false
				}
			} else {
				add(n)
			}
		}
	}
	if work != nil && work.CheckAlloc(int64(len(obj.Fields)+len(obj.Methods))*8) != nil {
		return nil, false
	}
	declarations := append(append([]*MemberInfo(nil), obj.Fields...), obj.Methods...)
	for _, member := range declarations {
		if member == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		desc, known := sourceBridgeUTF8(obj, member.DescriptorIndex)
		if !known || !signature(desc) {
			return nil, false
		}
		if !nativeAnnotationDependencies(member.Attributes, work, add) {
			return nil, false
		}
		for _, attribute := range member.Attributes {
			if sig, ok := attribute.(*SignatureAttribute); ok {
				desc, known := sourceBridgeUTF8(obj, sig.SignatureIndex)
				if !known || !signature(desc) {
					return nil, false
				}
			}
		}
	}
	if !nativeAnnotationDependencies(obj.Attributes, work, add) {
		return nil, false
	}
	for _, attribute := range obj.Attributes {
		if sig, ok := attribute.(*SignatureAttribute); ok {
			desc, known := sourceBridgeUTF8(obj, sig.SignatureIndex)
			if !known || !signature(desc) {
				return nil, false
			}
		}
	}
	return names, true
}
