package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// EnclosingMethod is a symbolic declaration edge, not a constant-pool index.
// Keep exact MUTF-8 keys when moving that metadata into a fresh pool.
type sourceEnclosingMethodKey struct{ owner, name, descriptor string }

func sourceEnclosingMethodIdentity(obj *ClassObject, work *workbudget.Budget) (sourceEnclosingMethodKey, bool, bool) {
	var key sourceEnclosingMethodKey
	if obj == nil {
		return key, false, false
	}
	var found bool
	for _, attr := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return key, false, false
		}
		raw, ok := attr.(*UnparsedAttribute)
		if !ok {
			continue
		}
		if raw == nil {
			return key, false, false
		}
		if raw.Name != "EnclosingMethod" {
			continue
		}
		if found || raw.Length != 4 || len(raw.Info) != 4 {
			return key, false, false
		}
		found = true
		var valid bool
		key.owner, valid = sourceParameterClassKey(obj, binary.BigEndian.Uint16(raw.Info))
		i := int(binary.BigEndian.Uint16(raw.Info[2:]))
		if !valid || i < 1 || i > len(obj.ConstantPool) {
			return key, false, false
		}
		nt, ok := obj.ConstantPool[i-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			return key, false, false
		}
		key.name, ok = sourceParameterUTF8Key(obj, nt.NameIndex)
		key.descriptor, valid = sourceParameterUTF8Key(obj, nt.DescriptorIndex)
		if !ok || !valid {
			return key, false, false
		}
	}
	return key, found, true
}

// This domain has one explicit enclosing instance, no class-owned formals,
// and an exact nonstatic enclosing method. Local named classes, initializers,
// static contexts and ambiguous declarations retain their separate proofs.
// Constructor capture/forwarding equality is independently checked by the
// generic-scope transaction, so this edge grants no executable edit.
func (c *ClassObjectDumper) anonymousGenericScopeOwner() (string, bool) {
	obj := c.obj
	if obj == nil {
		return "", false
	}
	owner, _, anonymous := originalAnonymousOwner(obj)
	sig, valid := nativeMethodLocalOriginalSignature(obj, obj.Attributes, c.Work)
	if !anonymous || !valid || obj.MajorVersion < 49 || obj.MajorVersion > 52 || obj.MinorVersion != 0 || len(types.ClassFormalTypeParamNames(sig)) != 0 {
		return "", false
	}
	key, present, valid := sourceEnclosingMethodIdentity(obj, c.Work)
	if !present || !valid {
		return "", false
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	parent, known := resolve(owner)
	if !known || parent == nil || parent.GetClassName() != owner || parent.MajorVersion < 49 || parent.MajorVersion > 52 || parent.MinorVersion != 0 || len(parent.Methods) > 4096 {
		return "", false
	}
	ownerKey, known := sourceParameterClassKey(parent, parent.ThisClass)
	if !known || ownerKey != key.owner {
		return "", false
	}
	var declaration *MemberInfo
	for _, m := range parent.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return "", false
		}
		n, nk := sourceParameterUTF8Key(parent, m.NameIndex)
		d, dk := sourceParameterUTF8Key(parent, m.DescriptorIndex)
		if !nk || !dk {
			return "", false
		}
		if n == key.name && d == key.descriptor {
			if declaration != nil {
				return "", false
			}
			declaration = m
		}
	}
	if declaration == nil || declaration.AccessFlags&(StaticFlag|0x0500) != 0 {
		return "", false
	}
	scopes, local, valid := c.flattenedMethodLexicalScopesResolved(resolve)
	if !valid || !local || len(scopes) < 3 {
		return "", false
	}
	return owner, true
}

// An anonymous allocation has one constructor. Its enclosing instance and
// captured locals are independently typed parameter-to-field edges; the SUPER
// argument list is a separate ordered packet. Preserve all edges rather than
// guessing that the first/last parameters are the only hidden ones.
func anonymousScopeConstructorGraph(object *ClassObject, outer string, original bool, work *workbudget.Budget, provider callbinding.Provider, expected map[string]*memberScopeConstructorProof) map[string]*memberScopeConstructorProof {
	if object == nil || len(object.Methods) > 4096 {
		return nil
	}
	proofs := map[string]*memberScopeConstructorProof{}
	for _, method := range object.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return nil
		}
		name, named := sourceBridgeUTF8(object, method.NameIndex)
		desc, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !named || !typed {
			return nil
		}
		if name != "<init>" {
			continue
		}
		if len(proofs) != 0 {
			return nil
		}
		var packet *constructorLoweringPacket
		if original {
			packet = constructorLoweringTypedPacketFor(object, method, true, true, work, provider)
		} else {
			prior := expected[desc]
			if prior == nil || prior.capture == nil {
				return nil
			}
			count := len(prior.capture.captures)
			packet = constructorLoweringTypedPacketFor(object, method, true, false, work, provider, count)
			if packet == nil {
				packet = constructorLoweringTypedPacketFor(object, method, false, false, work, provider, count)
			}
		}
		if packet == nil || len(packet.plan.captures) == 0 || len(packet.plan.captures) > 128 {
			return nil
		}
		enclosing := 0
		for _, capture := range packet.plan.captures {
			if capture.slot == 1 && capture.field.descriptor == "L"+outer+";" {
				enclosing++
			}
		}
		if enclosing != 1 {
			return nil
		}
		proofs[desc] = &memberScopeConstructorProof{capture: packet.plan}
	}
	if len(proofs) != 1 {
		return nil
	}
	return proofs
}
