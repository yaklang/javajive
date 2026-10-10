package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

func nativeMemberInheritedFieldSignature(owner *ClassObject, field *MemberInfo, descriptor string, work *workbudget.Budget) (bool, bool) {
	generic := false
	for _, a := range field.Attributes {
		if sourceProofNil(a) || !nativeProofWork(work, 1) {
			return false, false
		}
		switch a := a.(type) {
		case *ConstantValueAttribute:
			return false, false
		case *SignatureAttribute:
			if generic {
				return false, false
			}
			signature, known := sourceBridgeUTF8(owner, a.SignatureIndex)
			if !known || len(signature) > 4096 || !strings.HasPrefix(signature, "L") && !strings.HasPrefix(signature, "[") || !nativeProofWork(work, int64(len(signature))*130+1) || work != nil && work.CheckAlloc(int64(len(signature))*256) != nil {
				return false, false
			}
			// A field occupies one reference parameter grammar position. This
			// reuses the closed signature erasure proof without inventing formals.
			erased, throws, known := types.EraseConcreteMethodSignatureWithThrows("(" + signature + ")V")
			if !known || len(throws) != 0 || erased != "("+descriptor+")V" {
				return false, false
			}
			generic = true
		}
	}
	return generic, true
}

func nativeMemberProtectedStaticFieldProof(obj *ClassObject, m *MemberInfo, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) *nativeMemberPrivateGetter {
	g := nativeMemberProtectedFieldProof(obj, m, resolve, work)
	if g == nil || !g.staticField {
		return nil
	}
	return g
}

// The same complete declaration graph proves inherited instance and static
// reads. GETFIELD retains the original subclass receiver: qualifying through
// the declaring parent would lose Java's protected receiver restriction.
func nativeMemberProtectedFieldProof(obj *ClassObject, m *MemberInfo, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) *nativeMemberPrivateGetter {
	if resolve == nil {
		return nil
	}
	g := nativeMemberGetterPacketProof(obj, m, resolve, work)
	if g == nil || !g.inheritedField {
		return nil
	}
	return g
}

// JVM fields resolve by name and descriptor, while Java source hiding uses
// the name. Both must select the same original declaration. Interfaces are
// searched before the superclass by the JVM; without complete interface field
// metadata that path is deliberately unproved. No resolver or flags are guessed.
func nativeMemberInheritedFieldTarget(obj *ClassObject, name, desc string, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) (*ClassObject, *MemberInfo) {
	if obj == nil || resolve == nil || work != nil && work.CheckAlloc(64*256) != nil {
		return nil, nil
	}
	seen := map[string]bool{}
	interfaceState := map[string]uint8{}
	interfaceEdges := 0
	var absent func(string, int) bool
	absent = func(owner string, depth int) bool {
		if depth > 64 || !nativeProofWork(work, 1) {
			return false
		}
		if interfaceState[owner] != 0 {
			return interfaceState[owner] == 2
		}
		if len(interfaceState) >= 256 || work != nil && work.CheckAlloc(int64(len(interfaceState)+1)*256+64*256) != nil {
			return false
		}
		interfaceState[owner] = 1
		iface, known := resolve(owner)
		if !known || iface == nil || iface.GetClassName() != owner || iface.AccessFlags&0x0200 == 0 {
			return false
		}
		for _, f := range iface.Fields {
			if f == nil || !nativeProofWork(work, 1) {
				return false
			}
			n, nk := sourceBridgeUTF8(iface, f.NameIndex)
			_, dk := sourceBridgeUTF8(iface, f.DescriptorIndex)
			if !nk || !dk || n == name {
				return false
			}
		}
		for _, index := range iface.Interfaces {
			interfaceEdges++
			if interfaceEdges > 4096 {
				return false
			}
			n, known := sourceBridgeClassName(iface, index)
			if !known || !absent(n, depth+1) {
				return false
			}
		}
		interfaceState[owner] = 2
		return true
	}
	for depth := 0; obj != nil && depth < 64; depth++ {
		owner := obj.GetClassName()
		if seen[owner] || obj.AccessFlags&0x0200 != 0 || !nativeProofWork(work, 1) {
			return nil, nil
		}
		seen[owner] = true
		var target *MemberInfo
		for _, f := range obj.Fields {
			if f == nil || !nativeProofWork(work, 1) {
				return nil, nil
			}
			n, nk := sourceBridgeUTF8(obj, f.NameIndex)
			d, dk := sourceBridgeUTF8(obj, f.DescriptorIndex)
			if !nk || !dk {
				return nil, nil
			}
			if n == name {
				if d != desc || target != nil {
					return nil, nil
				}
				target = f
			}
		}
		if target != nil {
			return obj, target
		}
		for _, index := range obj.Interfaces {
			interfaceEdges++
			if interfaceEdges > 4096 {
				return nil, nil
			}
			iface, known := sourceBridgeClassName(obj, index)
			if !known || !absent(iface, 0) {
				return nil, nil
			}
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
