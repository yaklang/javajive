package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Flattening removes Java's lexical declarations but does not remove their
// binding identities. Resolve original ownership edges and the full enclosing
// method descriptor before reconstructing any captured formal. Dollar names
// and a method name alone cannot establish a lexical scope.
func (c *ClassObjectDumper) flattenedMethodLexicalScopes() ([]types.LexicalTypeScope, bool, bool) {
	if c.foldSiblingResolver == nil {
		return nil, false, true
	}
	seen := map[string]bool{}
	resolve := func(name string) (*ClassObject, bool) {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		b, known := c.foldSiblingResolver(name)
		if !known {
			return nil, false
		}
		obj, err := c.parseResolved(b)
		return obj, err == nil && obj != nil && obj.GetClassName() == name
	}
	var walk func(*ClassObject, int) ([]types.LexicalTypeScope, bool, bool)
	walk = func(obj *ClassObject, depth int) ([]types.LexicalTypeScope, bool, bool) {
		if obj == nil || depth >= 64 || seen[obj.GetClassName()] || !nativeProofWork(c.Work, 1) {
			return nil, true, false
		}
		seen[obj.GetClassName()] = true
		localRole := false
		var raw *UnparsedAttribute
		for _, a := range obj.Attributes {
			if !nativeProofWork(c.Work, 1) {
				return nil, true, false
			}
			if r, ok := a.(*UnparsedAttribute); ok && r != nil && r.Name == "EnclosingMethod" {
				localRole = true
				if raw != nil || r.Length != 4 || len(r.Info) != 4 {
					return nil, true, false
				}
				raw = r
			}
			if table, ok := a.(*InnerClassesAttribute); ok && table != nil {
				for _, row := range table.Classes {
					if !nativeProofWork(c.Work, 1) {
						return nil, true, false
					}
					if row != nil && row.InnerClassInfoIndex == obj.ThisClass && row.OuterClassInfoIndex == 0 {
						localRole = true
					}
				}
			}
		}
		sig, valid := nativeMethodLocalOriginalSignature(obj, obj.Attributes, c.Work)
		if !valid {
			return nil, localRole, false
		}
		var scopes []types.LexicalTypeScope
		hasMethod := raw != nil
		if raw != nil {
			if obj.MajorVersion < 49 {
				return nil, true, false
			}
			owner, known := sourceBridgeClassName(obj, binary.BigEndian.Uint16(raw.Info[:2]))
			if !known || owner == obj.GetClassName() {
				return nil, true, false
			}
			parent, known := resolve(owner)
			if !known || !c.flattenedLocalOwnerRows(obj, parent) {
				return nil, true, false
			}
			index := int(binary.BigEndian.Uint16(raw.Info[2:]))
			static := false
			methodSig, descriptor := "", ""
			if index == 0 {
				// An instance initializer is copied into every non-delegating
				// constructor. Type scope needs consistent original contexts,
				// not the single-placement certificate for a native source body.
				static, valid = originalInitializerTypeScopeStatic(parent, obj, c.Work)
				if !valid {
					return nil, true, false
				}
			} else {
				if index < 1 || index > len(obj.ConstantPool) {
					return nil, true, false
				}
				nt, ok := obj.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
				if !ok || nt == nil {
					return nil, true, false
				}
				name, nk := sourceBridgeUTF8(obj, nt.NameIndex)
				desc, dk := sourceBridgeUTF8(obj, nt.DescriptorIndex)
				_, ret, err := callbinding.Descriptor(desc)
				if !nk || !dk || name == "" || name == "<clinit>" || err != nil || name == "<init>" && ret != "V" {
					return nil, true, false
				}
				var method *MemberInfo
				for _, m := range parent.Methods {
					if m == nil || !nativeProofWork(c.Work, 1) {
						return nil, true, false
					}
					n, nk := sourceBridgeUTF8(parent, m.NameIndex)
					d, dk := sourceBridgeUTF8(parent, m.DescriptorIndex)
					if !nk || !dk {
						return nil, true, false
					}
					if n == name && d == desc {
						if method != nil {
							return nil, true, false
						}
						method = m
					}
				}
				if method == nil {
					return nil, true, false
				}
				static = method.AccessFlags&StaticFlag != 0
				methodSig, valid = nativeMethodLocalOriginalSignature(parent, method.Attributes, c.Work)
				if !valid {
					return nil, true, false
				}
				descriptor = desc
				if methodSig == "" {
					methodSig = desc
				}
			}
			if !static {
				scopes, _, valid = walk(parent, depth+1)
				if !valid {
					return nil, true, false
				}
			}
			if methodSig != "" {
				scopes = append(scopes, types.LexicalTypeScope{Signature: methodSig, Method: true, Descriptor: descriptor})
			}
		} else if owner, _, flags, member := originalMemberOwner(obj); member {
			if flags&StaticFlag == 0 {
				parent, known := resolve(owner)
				if !known {
					return nil, false, false
				}
				scopes, hasMethod, valid = walk(parent, depth+1)
				if !valid {
					return nil, hasMethod, false
				}
			}
		} else if !nativeMemberTopLevelEvidence(obj, c.Work) {
			return nil, localRole, false
		}
		scopes = append(scopes, types.LexicalTypeScope{Signature: sig})
		return scopes, hasMethod, true
	}
	return walk(c.obj, 0)
}

// Both original attribute tables must agree on the local/anonymous declaration.
func (c *ClassObjectDumper) flattenedLocalOwnerRows(child, parent *ClassObject) bool {
	var flags uint16
	var simple string
	rows := func(obj *ClassObject, corroborate bool) bool {
		count := 0
		for _, a := range obj.Attributes {
			if table, ok := a.(*InnerClassesAttribute); ok {
				if table == nil {
					return false
				}
				for _, row := range table.Classes {
					if row == nil || !nativeProofWork(c.Work, 1) {
						return false
					}
					n, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
					if !known {
						return false
					}
					if n != child.GetClassName() {
						continue
					}
					count++
					name := ""
					if row.InnerNameIndex != 0 {
						name, known = sourceBridgeUTF8(obj, row.InnerNameIndex)
						if !known || name == "" {
							return false
						}
					}
					// Older javac emits ACC_STATIC here for anonymous classes
					// in static contexts. This binary-row flag is corroborated
					// across both tables, but it does not establish a lexical
					// scope cut. The exact original enclosing method above does.
					if count != 1 || row.OuterClassInfoIndex != 0 {
						return false
					}
					if corroborate {
						if simple != name || flags != row.InnerClassAccessFlags {
							return false
						}
					} else {
						simple, flags = name, row.InnerClassAccessFlags
					}
				}
			}
		}
		return count == 1
	}
	return rows(child, false) && rows(parent, true)
}

func (c *ClassObjectDumper) originalFlattenedMethodScopes() ([]types.LexicalTypeScope, bool, error) {
	scopes, method, valid := c.flattenedMethodLexicalScopes()
	if c.Work != nil && c.Work.Err() != nil {
		return nil, true, c.Work.Err()
	}
	if !method {
		return nil, false, nil
	}
	if !valid {
		return nil, true, fmt.Errorf("unproven original enclosing method type scope for %s", c.obj.GetClassName())
	}
	var signatures []string
	for _, s := range scopes {
		signatures = append(signatures, s.Signature)
	}
	if !nativeMethodLocalBindingBudget(signatures, "()V", c.Work) {
		if c.Work != nil && c.Work.Err() != nil {
			return nil, true, c.Work.Err()
		}
		return nil, true, fmt.Errorf("enclosing method type scope budget exhausted")
	}
	return scopes, true, nil
}

func (c *ClassObjectDumper) projectFlattenedMethodFormals(free []string) ([]string, map[string]string, map[string]string, bool, error) {
	// With no captured formal there is no generic declaration to project.
	// Source/constructor eligibility remains the responsibility of its proof.
	if len(free) == 0 {
		return nil, nil, nil, false, nil
	}
	scopes, method, err := c.originalFlattenedMethodScopes()
	if err != nil || !method {
		return nil, nil, nil, method, err
	}
	names, clauses, erased, valid := types.ProjectLexicalTypeParameters(scopes, free, c.FuncCtx)
	if !valid {
		return nil, nil, nil, true, fmt.Errorf("unrepresentable original enclosing method type binding for %s", c.obj.GetClassName())
	}
	for n, d := range erased {
		erased[n] = strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(d, "L"), ";"), "/", ".")
	}
	return names, clauses, erased, true, nil
}

func (c *ClassObjectDumper) erasedFlattenedMethodFormals(names []string) (map[string]string, bool, error) {
	scopes, method, err := c.originalFlattenedMethodScopes()
	if err != nil || !method {
		return nil, method, err
	}
	erased, valid := types.LexicalTypeParameterErasures(scopes, names)
	if !valid {
		return nil, true, fmt.Errorf("unproven original enclosing method erasure for %s", c.obj.GetClassName())
	}
	for n, d := range erased {
		erased[n] = strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(d, "L"), ";"), "/", ".")
	}
	return erased, true, nil
}
