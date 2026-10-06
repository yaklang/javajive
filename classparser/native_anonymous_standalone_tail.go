package javaclassparser

import (
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A standalone tail is a separate binary/source declaration. It can compose
// with a native anonymous prefix only when its final header, static lexical
// ownership and original constructor packet are independently representable,
// and no symbolic dependency borrows a now-unnameable prefix type.
func (c *ClassObjectDumper) nativeAnonymousStandaloneTailClosed(p *nativeAnonymousFamily, members *nativeMemberFamily, forest *nativeAnonymousForest, metadata callbinding.Provider, access map[string]*nativeConstructorAccessBridge) bool {
	if p == nil || c.obj == nil || p.owner != c.obj.GetClassName() || c.options.TargetSourceVersion != 0 && c.options.TargetSourceVersion != 8 {
		return false
	}
	if len(p.standalone) != 0 && !nativeAnonymousForestVersion(c.obj, c.Work) {
		return false
	}
	for name, object := range p.standalone {
		if object == nil || object.GetClassName() != name || !nativeSourceBinaryName(object.GetSupperClassName()) || !nativeAnonymousForestVersion(object, c.Work) || !nativeProofWork(c.Work, 1) {
			return false
		}
		owner, method, known := originalAnonymousOwner(object)
		if !known || owner != p.owner || method == "" {
			return false
		}
		suffix, known := strings.CutPrefix(name, p.owner+"$")
		ordinal, e := strconv.Atoi(suffix)
		if !known || e != nil || strconv.Itoa(ordinal) != suffix || ordinal <= len(p.children) {
			return false
		}
		matches := 0
		for _, declaration := range c.obj.Methods {
			if declaration == nil {
				return false
			}
			n, nk := sourceBridgeUTF8(c.obj, declaration.NameIndex)
			d, dk := sourceBridgeUTF8(c.obj, declaration.DescriptorIndex)
			if !nk || !dk || !nativeProofWork(c.Work, 1) {
				return false
			}
			if n+d == method {
				if declaration.AccessFlags&8 == 0 {
					return false
				}
				matches++
			}
		}
		if matches != 1 {
			return false
		}
		packet := nativeAnonymousConstructorRepresentationProof(object, owner, method, "", c.Work, members, forest, metadata, true, access)
		if packet == nil || packet.enclosingField != "" || packet.superDescriptor != "()V" || packet.sourceSuperDescriptor != "()V" {
			return false
		}
		// The independent flat declaration initializes its captures after its super call.
		// Reuse the original constructor effect interpreter on every supported
		// runtime profile before composing that distinct source transaction.
		// Native anonymous ordinal regeneration uses the Java-8 source profile.
		reader := NewClassObjectDumper(object)
		reader.options = c.options
		reader.options.TargetSourceVersion = 8
		reader.Work = c.Work
		reader.foldSiblingResolver = c.foldSiblingResolver
		reader.declarationResolver = c.declarationResolver
		writes := map[string]bool{}
		for field := range packet.fields {
			writes[field] = true
		}
		if !reader.constructorCaptureChainDoesNotObserve(object.GetSupperClassName(), packet.superDescriptor, writes) {
			return false
		}
		// This initial domain has no inherited lexical type variables. Method-own
		// formals remain valid inside their own declarations; class/field signatures
		// must be closed without borrowing an enclosing class or method binder.
		for _, attribute := range object.Attributes {
			if sig, ok := attribute.(*SignatureAttribute); ok {
				if sig == nil {
					return false
				}
				s, known := sourceBridgeUTF8(object, sig.SignatureIndex)
				if !known || !nativeProofWork(c.Work, int64(len(s))) {
					return false
				}
				formals, refs, valid := types.SignatureTypeVariableReferences(s)
				if !valid || len(formals) != 0 || len(refs) != 0 {
					return false
				}
			}
		}
		if !nativeProofWork(c.Work, int64(len(object.Fields)+len(object.Methods))) || c.Work != nil && c.Work.CheckAlloc(int64(len(object.Fields)+len(object.Methods))*16) != nil {
			return false
		}
		for memberIndex, member := range append(append([]*MemberInfo{}, object.Fields...), object.Methods...) {
			if member == nil {
				return false
			}
			descriptor, known := sourceBridgeUTF8(object, member.DescriptorIndex)
			if !known {
				return false
			}
			for child := range p.children {
				if strings.Contains(descriptor, "L"+child+";") {
					return false
				}
			}
			for _, attribute := range member.Attributes {
				if sig, ok := attribute.(*SignatureAttribute); ok {
					if sig == nil {
						return false
					}
					s, known := sourceBridgeUTF8(object, sig.SignatureIndex)
					if !known || !nativeProofWork(c.Work, int64(len(s))) {
						return false
					}
					formals, refs, valid := types.SignatureTypeVariableReferences(s)
					if !valid || memberIndex < len(object.Fields) && len(formals) != 0 {
						return false
					}
					declared := map[string]bool{}
					for _, n := range formals {
						declared[n] = true
					}
					for _, n := range refs {
						if !declared[n] {
							return false
						}
					}
				}
			}
		}
		// Declaration/signature/annotation/class-constant references all share
		// the bounded original dependency scanner. A literal/cast/array type
		// cannot evade closure merely by lacking a CP member descriptor.
		dependencies, known := nativeMemberDependencyNames(object, c.Work)
		if !known {
			return false
		}
		for _, dependency := range dependencies {
			if p.children[dependency] != nil {
				return false
			}
		}
		for _, constant := range object.ConstantPool {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			if reference := nativeConstantMember(constant); reference != nil {
				target, known := sourceBridgeClassName(object, reference.ClassIndex)
				if !known || p.children[target] != nil {
					return false
				}
			}
			if nt, ok := constant.(*ConstantNameAndTypeInfo); ok {
				if nt == nil {
					return false
				}
				d, known := sourceBridgeUTF8(object, nt.DescriptorIndex)
				if !known {
					return false
				}
				for child := range p.children {
					if strings.Contains(d, "L"+child+";") {
						return false
					}
				}
			}
		}
	}
	return true
}

// The standalone certificate proves no dependence on the prefix. The archive
// index must separately exclude external type users and handles: a class not
// mentioned by the owner's InnerClasses table cannot borrow unnameable types.
func nativeAnonymousPrefixArchiveClosed(p *nativeAnonymousFamily, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if p == nil || index == nil || !index.valid {
		return false
	}
	for name := range p.children {
		if !nativeProofWork(work, 1) || index.handles[name] {
			return false
		}
		for user := range index.typeUsers[name] {
			if !nativeProofWork(work, 1) || user != p.owner && p.children[user] == nil {
				return false
			}
		}
	}
	return true
}

// Original method handles are a source-scope boundary, independently of method
// spelling or synthetic flags. Retain the executable flat representation until
// implementation-parameter origins can be transferred to the lifted source
// scope. An unrelated method/constructor handle does not cross this boundary.
func nativeAnonymousAllocationMethodsStayLexical(object *ClassObject, p *nativeAnonymousFamily, work *workbudget.Budget) bool {
	if object == nil || p == nil || p.owner != object.GetClassName() {
		return false
	}
	for _, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		handle, ok := constant.(*ConstantMethodHandleInfo)
		if !ok {
			continue
		}
		if handle == nil || handle.ReferenceKind < 1 || handle.ReferenceKind > 9 || handle.ReferenceIndex == 0 || int(handle.ReferenceIndex) > len(object.ConstantPool) {
			return false
		}
		if handle.ReferenceKind <= 4 || handle.ReferenceKind == 8 {
			continue
		}
		ref := nativeConstantMember(object.ConstantPool[handle.ReferenceIndex-1])
		if ref == nil || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(object.ConstantPool) {
			return false
		}
		owner, known := sourceBridgeClassName(object, ref.ClassIndex)
		nt, ok := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !known || !ok || nt == nil {
			return false
		}
		name, nk := sourceBridgeUTF8(object, nt.NameIndex)
		desc, dk := sourceBridgeUTF8(object, nt.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if owner != p.owner {
			continue
		}
		for _, unit := range p.children {
			if unit == nil || !nativeProofWork(work, 1) {
				return false
			}
			if unit.method == name+desc {
				return false
			}
		}
	}
	return true
}
