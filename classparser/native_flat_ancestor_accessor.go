package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core/callbinding"

// A terminally flat source retains original accessor packets as ordinary Java
// methods. javac's synthetic conflict check also searches superclasses, even
// for private or otherwise inaccessible methods. A projected descendant cannot
// regenerate a same-name/same-erased-type synthetic accessor while that ancestor
// is ordinary source. Original ACC_SYNTHETIC is not a receipt for the *source*
// flag, and a declaration-only dependency does not own the ancestor's lowering.
// Refuse this source composition; keep the original ordinary invocation instead
// of renaming a method or moving its declaring-class initialization boundary.
func nativeMemberFlatAncestorAccessorNamesClosed(prepared *nativeMemberPrepared, flat *ClassObject) bool {
	if prepared == nil || prepared.reader == nil || prepared.family == nil || prepared.family.failed || flat == nil {
		return false
	}
	d, family := prepared.reader, prepared.family
	flatName := flat.GetClassName()
	for _, getter := range family.getters {
		if getter == nil || !nativeProofWork(d.Work, 1) {
			return false
		}
		conflict := false
		for _, m := range flat.Methods {
			if m == nil || !nativeProofWork(d.Work, 1) {
				return false
			}
			n, named := sourceBridgeUTF8(flat, m.NameIndex)
			desc, typed := sourceBridgeUTF8(flat, m.DescriptorIndex)
			if !named || !typed || len(desc) == 0 || len(desc) > 65535 || !nativeProofWork(d.Work, int64(len(n)+len(desc))+1) || d.Work != nil && d.Work.CheckAlloc(int64(len(desc))*8+128) != nil {
				return false
			}
			if _, _, err := callbinding.Descriptor(desc); err != nil {
				return false
			}
			conflict = conflict || n == getter.name && desc == getter.descriptor
		}
		if !conflict {
			continue
		}
		current := family.lexicalObjects[getter.owner]
		if current == nil || current.GetClassName() != getter.owner || d.Work != nil && d.Work.CheckAlloc(64*128) != nil {
			return false
		}
		seen := map[string]bool{}
		resolve := d.nativeAnnotationDeclarationResolver()
		for {
			name := current.GetClassName()
			if name == flatName || seen[name] || len(seen) >= 64 || !nativeProofWork(d.Work, 1) {
				return false
			}
			seen[name] = true
			parent := current.GetSupperClassName()
			if parent == "java/lang/Object" {
				break
			}
			if parent == "" || parent == flatName {
				return false
			}
			current = family.lexicalObjects[parent]
			if current == nil {
				var known bool
				current, known = resolve(parent)
				if !known {
					return false
				}
			}
			if current == nil || current.GetClassName() != parent {
				return false
			}
		}
	}
	return true
}
