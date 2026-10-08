package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A completed foreign member declaration supplies a SUPER binding view, never
// private or lexical ownership. Its enclosing operand may be omitted only if
// the original child passes its unchanged enclosing parameter and that same
// object widens to the parent's declaring class. No capture store commutes.
func nativeMemberExternalSupersClosed(p *nativeMemberFamily, metadata callbinding.Provider, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if p == nil || resolve == nil {
		return false
	}
	for _, child := range p.children {
		if child == nil || child.object == nil {
			return false
		}
		parent := p.allocationDependencies[child.object.GetSupperClassName()]
		if parent == nil || parent.static {
			continue
		}

		// Enclosing identity is independent of generic arity. Original outer
		// and member Signatures remain in the normal source binding view; that
		// separate binder still selects the exact original constructor and
		// preserves every explicit operand. This proof omits only slot 1.
		if child.static || parent.object == nil || parent.owner == child.owner || len(parent.accessBridges) != 0 || !nativeMemberOriginalClassWidening(child.owner, parent.owner, resolve, work) {
			return false
		}
		// An owned access bridge delegates to THIS, not the foreign parent.
		// Reconstruct its original unused-marker packet before excluding it
		// from the SUPER inventory. This supplies no foreign private access.
		var originalBridges map[string]*nativeConstructorAccessBridge
		if len(child.accessBridges) != 0 {
			reader := NewClassObjectDumper(child.object)
			reader.Work = work
			originalBridges = reader.originalNativeConstructorAccessBridges()
		}
		for _, method := range child.object.Methods {
			if method == nil || !nativeProofWork(work, 1) {
				return false
			}
			name, nk := sourceBridgeUTF8(child.object, method.NameIndex)
			desc, dk := sourceBridgeUTF8(child.object, method.DescriptorIndex)
			if !nk || !dk {
				return false
			}
			if name != "<init>" {
				continue
			}
			if bridge := child.accessBridges[desc]; bridge != nil {
				original := originalBridges[desc]
				if original == nil || *original != *bridge || original.method != method {
					return false
				}
				continue
			}
			ctor := child.constructors[desc]
			if ctor == nil {
				return false
			}
			if ctor.delegateOwner == child.object.GetClassName() {
				continue
			}
			target := nativeMemberConstructorForAllocation(parent, ctor.delegateDescriptor)
			if ctor.capturePC < 0 || ctor.delegateOwner != parent.object.GetClassName() || target == nil || target.sourceDescriptor == "" {
				return false
			}
			// Foreign private constructor protocols require their own joint access
			// certificate; a binding-only dependency does not grant that capability.
			declarations := 0
			for _, m := range parent.object.Methods {
				if m == nil || !nativeProofWork(work, 1) {
					return false
				}
				n, nk := sourceBridgeUTF8(parent.object, m.NameIndex)
				d, dk := sourceBridgeUTF8(parent.object, m.DescriptorIndex)
				if !nk || !dk {
					return false
				}
				if n == "<init>" && d == ctor.delegateDescriptor {
					declarations++
					if m.AccessFlags&2 != 0 {
						return false
					}
				}
			}
			if declarations != 1 {
				return false
			}
			proved := false
			for _, a := range method.Attributes {
				code, ok := a.(*CodeAttribute)
				if !ok {
					continue
				}
				if code == nil || !nativeProofWork(work, int64(len(code.Code))) {
					return false
				}
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(child.object.ConstantPool, i) })
				decoder.Work = work
				if decoder.ParseOpcode() != nil {
					return false
				}
				ops := constructorMotionOps(decoder)
				start := -1
				for i, op := range ops {
					if int(op.CurrentOffset) == ctor.capturePC {
						start = i + 1
						break
					}
				}
				params, _, err := callbinding.Descriptor(desc)
				if err != nil || len(params) == 0 || params[0] != "L"+child.owner+";" || start < 0 {
					return false
				}
				next, call := constructorMotionDelegationEnclosing(child.object, ops, start, params, constructorParameterSlots(params), metadata, nil, 1)
				if next == 0 || call == nil {
					next, call = nativeMemberFrameDelegationWithMetadata(child.object, method, code, ops, start, work, metadata, nil)
				}
				if next > 0 && call != nil && call.Name == ctor.delegateOwner && call.Description == ctor.delegateDescriptor && int(ops[next-1].CurrentOffset) == ctor.delegatePC {
					proved = true
				}
			}
			if !proved {
				return false
			}
			ctor.projectedSuper = true
		}
	}
	return true
}

// Indexed foreign SUPER calls are not NEW allocations. Certify their original
// uninitialized-THIS delegation without publishing or borrowing foreign scope.
// The foreign family must separately prove its complete source before any of
// its source spellings/constructor projections can be emitted.
func (c *ClassObjectDumper) nativeMemberForeignOriginalSuper(p *nativeMemberFamily, method *MemberInfo, owner, descriptor string, pc int) bool {
	if c == nil || c.obj == nil || p == nil || method == nil || p.children[c.obj.GetClassName()] != nil || owner != c.obj.GetSupperClassName() {
		return false
	}
	name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !nk || !dk || name != "<init>" {
		return false
	}
	parent := p.allocationClass(owner)
	if parent == nil || parent.static || len(parent.accessBridges) != 0 || nativeMemberConstructorForAllocation(parent, descriptor) == nil {
		return false
	}
	enclosing, _, _, member := originalMemberOwner(c.obj)
	if !member || enclosing == p.owner {
		return false
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	root, known := resolve(enclosing)
	if !known || root == nil || !nativeMemberTopLevelEvidence(root, c.Work) || !nativeMemberOriginalClassWidening(enclosing, parent.owner, resolve, c.Work) {
		return false
	}
	if !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(512) != nil {
		return false
	}
	// A foreign indexed caller may itself have a private constructor access
	// bridge. Its own original packet belongs only to this temporary physical
	// class proof; never import it into the parent's lexical/bridge ownership.
	bridges := c.originalNativeConstructorAccessBridges()
	child := nativeMemberProofWithDeclarations(c.obj, root, c.Work, bridges, map[string]*ClassObject{enclosing: root}, resolve, c.buildInvocationMetadata())
	if child == nil || child.static {
		return false
	}
	ctor := child.constructors[desc]
	if ctor == nil || ctor.capturePC < 0 || ctor.delegatePC != pc || ctor.delegateOwner != owner || ctor.delegateDescriptor != descriptor {
		return false
	}
	view := &nativeMemberFamily{children: map[string]*nativeMemberClass{c.obj.GetClassName(): child}, allocationDependencies: map[string]*nativeMemberClass{owner: parent}}
	return nativeMemberExternalSupersClosed(view, c.buildInvocationMetadata(), resolve, c.Work)
}
