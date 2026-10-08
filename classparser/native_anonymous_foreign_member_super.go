package javaclassparser

import (
	"bytes"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Anonymous planning precedes source-dependency publication. An original
// named SUPER declaration can supply a temporary constructor packet, but never
// joins children, lexicalObjects, access bridges or registration ownership.
func (c *ClassObjectDumper) prepareNativeAnonymousForeignMemberSuper(object *ClassObject, owner string, p *nativeMemberFamily, forest *nativeAnonymousForest) bool {
	if c == nil || c.obj == nil || object == nil || p == nil || p.failed || c.obj.GetClassName() != owner || !nativeProofWork(c.Work, 1) {
		return false
	}
	binary := object.GetSupperClassName()
	if p.children[binary] != nil {
		return true
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	if resolve == nil {
		return false
	}
	parent, found := resolve(binary)
	if !found {
		// Ordinary platform superclasses have no member projection. The
		// existing original InnerClasses refusal still rejects unknown ones.
		return true
	}
	declaring, _, flags, member := originalMemberOwner(parent)
	if !member || flags&8 != 0 {
		return true
	}
	if p.lexicalObjects[owner] != c.obj && (forest == nil || forest.members != p || forest.objects[owner] != c.obj || forest.units[owner] == nil || forest.units[owner].object != c.obj) {
		return false
	}
	originalOwner, _, anonymous := originalAnonymousOwner(object)
	if !anonymous || originalOwner != owner {
		return false
	}
	if flags&2 != 0 || !nativeMemberAncestorDeclarationDependency(c.obj, parent, resolve, c.Work) {
		return false
	}
	lexical := map[string]*ClassObject{binary: parent}
	current := declaring
	for depth := 0; ; depth++ {
		if depth >= 64 || lexical[current] != nil || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(len(lexical)+1)*128) != nil {
			return false
		}
		original, known := resolve(current)
		if !known || original == nil || original.GetClassName() != current || !nativeMemberDeclarationMetadataBounded(original, c.Work) {
			return false
		}
		lexical[current] = original
		next, _, _, nested := originalMemberOwner(original)
		if !nested {
			// Reuse the complete reciprocal named-edge proof in the
			// declaration's own package. This supplies identity only; the
			// distinct original caller ancestry supplied access above.
			if !nativeMemberOriginalDeclarationPath(original, parent, resolve, c.Work) {
				return false
			}
			break
		}
		current = next
	}
	enclosing := lexical[declaring]
	reader := NewClassObjectDumper(parent)
	reader.Work, reader.options = c.Work, c.options
	reader.foldSiblingResolver, reader.declarationResolver = c.foldSiblingResolver, c.declarationResolver
	bridges := reader.originalNativeConstructorAccessBridges()
	if len(bridges) != 0 {
		return false
	}
	packet := nativeMemberProofWithDeclarations(parent, enclosing, c.Work, bridges, lexical, resolve, reader.buildInvocationMetadata())
	if packet == nil || packet.static || packet.owner != declaring || len(packet.accessBridges) != 0 || len(p.pendingAnonymousSuperDependencies) >= nativeMemberDependencyNodeLimit || c.Work != nil && c.Work.CheckAlloc(int64(len(p.pendingAnonymousSuperDependencies)+1)*512) != nil {
		return false
	}
	if p.pendingAnonymousSuperDependencies == nil {
		p.pendingAnonymousSuperDependencies = map[string]*nativeMemberClass{}
	}
	if previous := p.pendingAnonymousSuperDependencies[binary]; previous != nil {
		if !nativeAnonymousForeignSuperSameOriginal(previous, packet, c.Work) {
			return false
		}
	} else {
		p.pendingAnonymousSuperDependencies[binary] = packet
	}
	p.anonymousSuperResolver = resolve
	return true
}

func (p *nativeMemberFamily) anonymousSuperClass(binary string) *nativeMemberClass {
	if p == nil || p.failed {
		return nil
	}
	if child := p.children[binary]; child != nil {
		return child
	}
	if packet := p.pendingAnonymousSuperDependencies[binary]; packet != nil {
		return packet
	}
	return p.allocationDependencies[binary]
}

// A protected member name may occur inside an owned anonymous subclass even
// though the source root is unrelated. Only this exact planned SUPER use and
// original enclosing-class ancestry license the separate declaration view.
func nativeAnonymousForeignSuperDeclarationUse(p *nativeMemberFamily, object *ClassObject, work *workbudget.Budget) bool {
	if p == nil || p.failed || object == nil || p.anonymousSuperResolver == nil || !nativeProofWork(work, 1) {
		return false
	}
	parent := p.pendingAnonymousSuperDependencies[object.GetClassName()]
	if parent == nil || parent.object == nil || !nativeAnonymousForeignSuperOriginalObjectsEqual(object, parent.object, work) {
		return false
	}
	for binary, group := range p.anonymousUnits {
		if group == nil || group.children[binary] == nil || !nativeProofWork(work, 1) {
			return false
		}
		unit := group.children[binary]
		if unit.memberSuper != parent || unit.object == nil || !nativeMemberJointAnonymousAccess(p, binary, work) {
			continue
		}
		owner, _, known := originalAnonymousOwner(unit.object)
		context, found := p.anonymousSuperResolver(owner)
		if known && found && context != nil && context.GetClassName() == owner && nativeMemberAncestorDeclarationDependency(context, object, p.anonymousSuperResolver, work) {
			return true
		}
	}
	return false
}

func nativeAnonymousForeignSuperConstructorAccessible(parent *nativeMemberClass, caller, descriptor string, work *workbudget.Budget) bool {
	if parent == nil || parent.object == nil || parent.static || len(parent.accessBridges) != 0 || parent.constructors[descriptor] == nil || !nativeProofWork(work, 1) {
		return false
	}
	packageName := func(name string) string {
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			return name[:i]
		}
		return ""
	}
	matches := 0
	for _, method := range parent.object.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, nk := sourceBridgeUTF8(parent.object, method.NameIndex)
		desc, dk := sourceBridgeUTF8(parent.object, method.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if name == "<init>" && desc == descriptor {
			matches++
			if method.AccessFlags&2 != 0 || method.AccessFlags&(1|4) == 0 && packageName(caller) != packageName(parent.owner) {
				return false
			}
		}
	}
	return matches == 1
}

// Cached names are insufficient: planning and committed source must refer to
// the same immutable original bytes and exact physical/source ctor packets.
func nativeAnonymousForeignSuperSameOriginal(expected, actual *nativeMemberClass, work *workbudget.Budget) bool {
	if expected == nil || actual == nil || expected.object == nil || actual.object == nil || expected.static || actual.static || expected.owner != actual.owner || expected.name != actual.name || expected.field != actual.field || expected.flags != actual.flags || expected.formalCount != actual.formalCount || expected.outerFormalCount != actual.outerFormalCount || len(expected.accessBridges) != 0 || len(actual.accessBridges) != 0 || len(expected.constructors) != len(actual.constructors) || !nativeProofWork(work, int64(len(expected.constructors))+1) {
		return false
	}
	before, after := expected.object.Bytes(), actual.object.Bytes()
	if !nativeProofWork(work, int64(len(before)+len(after))) || work != nil && work.CheckAlloc(int64(len(before)+len(after))) != nil || !bytes.Equal(before, after) {
		return false
	}
	for key, original := range expected.constructors {
		completed := actual.constructors[key]
		if original == nil || completed == nil || original.descriptor != completed.descriptor || original.sourceDescriptor != completed.sourceDescriptor || original.capturePC != completed.capturePC || original.delegatePC != completed.delegatePC || original.delegateOwner != completed.delegateOwner || original.delegateDescriptor != completed.delegateDescriptor {
			return false
		}
	}
	return true
}

// Only completed, independently owned source can replace planning packets.
// No foreign named class is ever added to the caller's lexical ownership.
func nativeAnonymousForeignSupersCommitted(p *nativeMemberFamily, work *workbudget.Budget) bool {
	if p == nil || p.failed || !nativeProofWork(work, int64(len(p.anonymousUnits))+1) || work != nil && work.CheckAlloc(int64(len(p.anonymousUnits))*16) != nil {
		return false
	}
	type replacement struct {
		unit   *nativeAnonymousClass
		parent *nativeMemberClass
	}
	replacements := []replacement{}
	for binary, group := range p.anonymousUnits {
		if group == nil || group.children[binary] == nil || !nativeProofWork(work, 1) {
			return false
		}
		unit := group.children[binary]
		if unit.object == nil || unit.object.GetClassName() != binary {
			return false
		}
		parent := unit.memberSuper
		if parent == nil {
			continue
		}
		if parent.object == nil {
			return false
		}
		if p.children[parent.object.GetClassName()] == parent {
			continue
		}
		name := parent.object.GetClassName()
		completed := p.allocationDependencies[name]
		if p.pendingAnonymousSuperDependencies[name] != parent || completed == nil || completed.sourceName == "" || p.sourceDependencies[name] != completed.sourceName || !nativeAnonymousForeignSuperSameOriginal(parent, completed, work) {
			return false
		}
		replacements = append(replacements, replacement{unit, completed})
	}
	// A later refusal must not leave earlier units bound to a partial source
	// view. Publish only after every original packet is validated.
	for _, replacement := range replacements {
		replacement.unit.memberSuper = replacement.parent
	}
	p.pendingAnonymousSuperDependencies = nil
	return true
}

// A parent's original archive index includes foreign anonymous SUPER users.
// Their first uninitialized-THIS call is a physical delegation, not a NEW.
// Certifying that exact packet does not publish their source scope or grant
// either family private access. Source completion is still independent.
func (c *ClassObjectDumper) nativeAnonymousForeignOriginalSuper(parent *nativeMemberClass, method *MemberInfo, descriptor string, pc int) bool {
	if c == nil || c.obj == nil || parent == nil || parent.object == nil || method == nil || parent.static || len(parent.accessBridges) != 0 {
		return false
	}
	name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	owner, enclosingMethod, anonymous := originalAnonymousOwner(c.obj)
	if !nk || !dk || name != "<init>" || !anonymous || owner == parent.owner {
		return false
	}
	matches := 0
	for _, original := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if original == method {
			matches++
		}
	}
	if matches != 1 {
		return false
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	root, known := resolve(owner)
	if !known || root == nil || root.GetClassName() != owner {
		return false
	}
	// An anonymous source context can itself be enclosed by named/anonymous
	// scopes. Rebuild those original lexical edges, then prove the complete
	// source forest rather than making a same-spelled temporary owner.
	seen := map[string]bool{}
	for !nativeMemberTopLevelEvidence(root, c.Work) {
		if len(seen) >= 64 || seen[root.GetClassName()] || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(len(seen)+1)*128) != nil {
			return false
		}
		seen[root.GetClassName()] = true
		enclosing, _, _, member := originalMemberOwner(root)
		if !member {
			var anonymous bool
			enclosing, _, anonymous = originalAnonymousOwner(root)
			if !anonymous {
				return false
			}
		}
		root, known = resolve(enclosing)
		if !known || root == nil || root.GetClassName() != enclosing {
			return false
		}
	}
	reader := NewClassObjectDumper(root)
	reader.Work, reader.options = c.Work, c.options
	reader.foldSiblingResolver, reader.declarationResolver = c.foldSiblingResolver, c.declarationResolver
	var packet *nativeAnonymousClass
	if root.GetClassName() == owner {
		physical := &nativeMemberFamily{owner: owner, children: map[string]*nativeMemberClass{}, lexicalObjects: map[string]*ClassObject{owner: root}}
		packet = reader.nativeAnonymousConstructorForCompiler(c.obj, owner, enclosingMethod, owner, physical, nil, reader.buildInvocationMetadata(), nil)
	} else {
		physical := reader.planNativeMemberFamily()
		if physical == nil || !reader.planNativeMemberAnonymousScopes(physical) {
			return false
		}
		group := physical.anonymousUnits[c.obj.GetClassName()]
		if group == nil || group.owner != owner {
			return false
		}
		packet = group.children[c.obj.GetClassName()]
		if packet == nil || packet.object == nil || !nativeAnonymousForeignSuperOriginalObjectsEqual(packet.object, c.obj, c.Work) {
			return false
		}
	}
	return packet != nil && packet.memberSuper != nil && packet.descriptor == desc && packet.superDescriptor == descriptor && packet.superPC == pc && nativeAnonymousForeignSuperSameOriginal(parent, packet.memberSuper, c.Work) && nativeAnonymousForeignSuperConstructorAccessible(parent, owner, descriptor, c.Work)
}

func nativeAnonymousForeignSuperOriginalObjectsEqual(before, after *ClassObject, work *workbudget.Budget) bool {
	if before == nil || after == nil || before.GetClassName() != after.GetClassName() || !nativeProofWork(work, 1) {
		return false
	}
	a, b := before.Bytes(), after.Bytes()
	return nativeProofWork(work, int64(len(a)+len(b))) && (work == nil || work.CheckAlloc(int64(len(a)+len(b))) == nil) && bytes.Equal(a, b)
}
