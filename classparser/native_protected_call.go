package javaclassparser

import (
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

func nativeBinaryPackage(name string) string {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return ""
}

// Class-method resolution follows original superclass declarations, not a CP
// name or a guessed protected flag. Interfaces cannot declare protected methods.
func nativeMemberCallTarget(obj *ClassObject, name, desc string, inherited bool, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) (*ClassObject, *MemberInfo) {
	seen := map[string]bool{}
	for depth := 0; obj != nil && depth < 64; depth++ {
		owner := obj.GetClassName()
		if seen[owner] || !nativeProofWork(work, 1) || obj.AccessFlags&0x0200 != 0 {
			return nil, nil
		}
		seen[owner] = true
		var target *MemberInfo
		for _, m := range obj.Methods {
			if m == nil || !nativeProofWork(work, 1) {
				return nil, nil
			}
			n, nk := sourceBridgeUTF8(obj, m.NameIndex)
			d, dk := sourceBridgeUTF8(obj, m.DescriptorIndex)
			if !nk || !dk {
				return nil, nil
			}
			if n == name && d == desc {
				if target != nil {
					return nil, nil
				}
				target = m
			}
		}
		if target != nil {
			return obj, target
		}
		if !inherited || resolve == nil {
			return nil, nil
		}
		parent := obj.GetSupperClassName()
		if parent == "" {
			return nil, nil
		}
		next, known := resolve(parent)
		if !known || next == nil || next.GetClassName() != parent {
			return nil, nil
		}
		obj = next
	}
	return nil, nil
}

func nativeMemberAccessorSymbolKey(getter *nativeMemberPrivateGetter) string {
	if nativeMemberAccessorClonedSymbol(getter) {
		// javac clones the inherited protected symbol for each source access. The
		// original bridge identity witnesses that occurrence; equal target strings
		// must not merge two independently registered symbols.
		return getter.owner + "\x00" + getter.name + getter.descriptor
	}
	return getter.owner + "\x00" + getter.field + getter.fieldDescriptor
}

func nativeMemberAccessorClonedSymbol(getter *nativeMemberPrivateGetter) bool {
	return getter != nil && (getter.inheritedField || getter.call != nil && getter.call.inherited)
}
