package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A source field name must denote the original symbolic field through the
// anonymous receiver's real ancestry. Unknown, cyclic or ambiguous inherited
// declarations cannot authorize removing that otherwise unnameable receiver.
// Generic field signatures require a separate instantiated source-type proof.
func nativeAnonymousLexicalFieldDeclaration(root *ClassObject, name, descriptor string, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) (*ClassObject, *MemberInfo, bool) {
	if root == nil || resolve == nil || work != nil && work.CheckAlloc(512) != nil {
		return nil, nil, false
	}
	if _, result, err := callbinding.Descriptor("()" + descriptor); err != nil || result == "V" || name == "" {
		return nil, nil, false
	}
	type declaration struct {
		owner *ClassObject
		field *MemberInfo
	}
	seen, active := map[string]declaration{}, map[string]bool{}
	var visit func(*ClassObject, int) (declaration, bool)
	visit = func(obj *ClassObject, depth int) (declaration, bool) {
		if obj == nil || depth >= 64 || !nativeProofWork(work, 1) {
			return declaration{}, false
		}
		key := obj.GetClassName()
		if key == "" || active[key] {
			return declaration{}, false
		}
		if found, known := seen[key]; known {
			return found, true
		}
		if len(seen)+len(active) >= 128 || work != nil && work.CheckAlloc(int64(len(seen)+len(active)+1)*128) != nil {
			return declaration{}, false
		}
		active[key] = true
		defer delete(active, key)
		var found declaration
		for _, field := range obj.Fields {
			if field == nil || !nativeProofWork(work, 1) {
				return declaration{}, false
			}
			n, known := sourceBridgeUTF8(obj, field.NameIndex)
			if !known {
				return declaration{}, false
			}
			if n != name {
				continue
			}
			d, known := sourceBridgeUTF8(obj, field.DescriptorIndex)
			if !known || d != descriptor || found.field != nil || field.AccessFlags&(0x1000|0x4000) != 0 {
				return declaration{}, false
			}
			for _, attr := range field.Attributes {
				if _, generic := attr.(*SignatureAttribute); generic {
					return declaration{}, false
				}
			}
			found = declaration{obj, field}
		}
		if found.field != nil {
			seen[key] = found
			return found, true
		}
		if len(obj.Interfaces) > 128 || !nativeProofWork(work, int64(len(obj.Interfaces))+1) || work != nil && work.CheckAlloc(int64(len(obj.Interfaces)+1)*32) != nil {
			return declaration{}, false
		}
		parents := append([]string{}, obj.GetInterfacesName()...)
		if parent := obj.GetSupperClassName(); parent != "" {
			parents = append(parents, parent)
		}
		for _, parent := range parents {
			p, known := resolve(parent)
			if !known || p == nil || p.GetClassName() != parent {
				return declaration{}, false
			}
			next, known := visit(p, depth+1)
			if !known || next.field != nil && found.field != nil && (next.owner.GetClassName() != found.owner.GetClassName() || next.field != found.field) {
				return declaration{}, false
			}
			if next.field != nil {
				found = next
			}
		}
		seen[key] = found
		return found, true
	}
	found, known := visit(root, 0)
	return found.owner, found.field, known && found.owner != nil && found.field != nil
}
