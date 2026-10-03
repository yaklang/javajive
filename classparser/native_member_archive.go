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

type nativeMemberIndex struct {
	once         sync.Once
	valid        bool
	constructors map[string]map[string]bool
	captureUsers map[string]map[string]bool
	typeUsers    map[string]map[string]bool
	handles      map[string]bool
}
type nativeMemberCacheEntry struct {
	once   sync.Once
	family *nativeMemberFamily
	source string
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
				case *ConstantMethodrefInfo:
					if name == "<init>" {
						if !record(idx.constructors, owner) {
							return fmt.Errorf("member index edge limit")
						}
					}
				case *ConstantFieldrefInfo:
					if name == "this$0" {
						if !record(idx.captureUsers, owner) {
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
			}
			return nil
		})
		idx.valid = e == nil
	})
	return idx
}
func (z *JarFS) nativeMemberEntry(obj *ClassObject) *nativeMemberCacheEntry {
	owner, _, _, member := originalMemberOwner(obj)
	if !member {
		owner = obj.GetClassName()
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
	z.nativeMembersMu.Lock()
	entry := z.nativeMembersCache[key]
	if entry == nil {
		if len(z.nativeMembersCache) >= 512 {
			z.nativeMembersMu.Unlock()
			return nil
		}
		if z.nativeMembersCache == nil {
			z.nativeMembersCache = map[string]*nativeMemberCacheEntry{}
		}
		entry = &nativeMemberCacheEntry{}
		z.nativeMembersCache[key] = entry
	}
	z.nativeMembersMu.Unlock()
	entry.once.Do(func() {
		root := obj
		if member {
			raw, known := z.enumSiblingResolver()(owner)
			if !known {
				return
			}
			var e error
			root, e = z.nativeMemberReader(obj).parseResolved(raw)
			if e != nil || root.GetClassName() != owner {
				return
			}
		}
		d := z.nativeMemberReader(root)
		d.options.EnvSnapshot = snap
		p := d.planNativeMemberFamily()
		if p == nil {
			return
		}
		index := z.originalMemberIndex()
		if !index.valid || !z.nativeMemberAccessRepresentable(p, index, d.Work) {
			return
		}
		objects := map[string]*ClassObject{owner: root}
		for n, child := range p.children {
			objects[n] = child.object
			if index.handles[n] {
				return
			}
			for user := range index.captureUsers[n] {
				if user != n {
					return
				}
			}
			for user := range index.constructors[n] {
				if objects[user] != nil {
					continue
				}
				raw, known := z.enumSiblingResolver()(user)
				if !known {
					return
				}
				other, e := d.parseResolved(raw)
				if e != nil || other.GetClassName() != user {
					return
				}
				objects[user] = other
			}
		}
		for _, object := range objects {
			reader := z.nativeMemberReader(object)
			if _, known := reader.nativeMemberAllocations(p); !known {
				return
			}
		}
		// Independent direct families have independent commits to ownership. A
		// cross-family source scope requires a joint dependency plan, so refuse
		// a root/body referring to another archive-owned nonstatic member type.
		lexicalNames := map[string]bool{}
		for _, child := range p.children {
			lexicalNames[child.name] = true
		}
		packageRoot := strings.SplitN(owner, "/", 2)[0]
		if strings.Contains(owner, "/") && lexicalNames[packageRoot] {
			return
		}
		for _, object := range append([]*ClassObject{root}, nativeMemberObjects(p)...) {
			references, known := nativeMemberDependencyNames(object, d.Work)
			if !known {
				return
			}
			for _, n := range references {
				// A default-package class has no qualified spelling to escape a
				// same-named lexical member declaration. Retain the flat scope.
				if !strings.Contains(n, "/") && lexicalNames[n] {
					return
				}
				if p.children[n] != nil || n == owner {
					continue
				}
				raw, found := z.enumSiblingResolver()(n)
				if !found {
					continue
				}
				other, e := d.parseResolved(raw)
				if e != nil {
					return
				}
				otherOwner, _, _, isMember := originalMemberOwner(other)
				if isMember && otherOwner != owner {
					return
				}
				if anonOwner, _, anon := originalAnonymousOwner(other); anon && (anonOwner == owner || p.children[anonOwner] != nil) {
					return
				}
			}
		}
		for name, object := range objects {
			if name == owner || p.children[name] != nil {
				continue
			}
			reader := z.nativeMemberReader(object)
			reader.options.EnvSnapshot = snap
			reader.nativeMemberRoot = p
			var err error
			jdecenv.Run(snap, func() error { _, err = reader.DumpClass(); return err })
			if err != nil || p.failed {
				return
			}
		}
		d.nativeMemberRoot = p
		var src string
		var e error
		jdecenv.Run(snap, func() error { src, e = d.DumpClass(); return e })
		if e != nil || p.failed || strings.Contains(src, DecompileStubMarker) {
			return
		}
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
func nativeMemberObjects(p *nativeMemberFamily) []*ClassObject {
	objects := make([]*ClassObject, 0, len(p.children))
	for _, child := range p.children {
		objects = append(objects, child.object)
	}
	return objects
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
		return nil
	}
	return entry.family.children[name]
}
func (z *JarFS) nativeMemberSource(obj *ClassObject) ([]byte, bool) {
	entry := z.nativeMemberEntry(obj)
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
		for _, attribute := range member.Attributes {
			if sig, ok := attribute.(*SignatureAttribute); ok {
				desc, known := sourceBridgeUTF8(obj, sig.SignatureIndex)
				if !known || !signature(desc) {
					return nil, false
				}
			}
		}
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
