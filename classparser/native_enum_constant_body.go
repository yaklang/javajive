package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
	"slices"
	"strconv"
	"strings"
)

// A constant-specific class is owned by an original constant allocation, not
// by an unnamed row alone. The source compiler erases its constructor; prove
// that constructor forwards every original operand exactly once, without an
// initializer effect, through the original private enum access bridge.
type nativeEnumConstantBody struct {
	object                    *ClassObject
	owner, descriptor         string
	plan                      nativeEnumConstantAllocation
	superDescriptor           string
	superPC                   int
	rendered                  bool
	legacyConstructorMetadata bool
	assertions                *nativeMemberAssertion
}

func nativeEnumConstantBodyProof(parent *ClassObject, plan nativeEnumConstantAllocation, bodyOrdinal int, bridges map[string]*nativeConstructorAccessBridge, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) *nativeEnumConstantBody {
	if parent == nil || resolve == nil || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(512) != nil {
		return nil
	}
	obj, known := resolve(plan.allocatedClass)
	if !known || obj == nil || obj.GetClassName() != plan.allocatedClass || obj.AccessFlags != 0x4030 || obj.GetSupperClassName() != parent.GetClassName() || len(obj.Fields) > 1 || len(obj.Interfaces) != 0 || !nativeAccessorVersion(obj, work) {
		return nil
	}
	// MethodParameters is optional, including on Java 8. The supported
	// pre-nestmate enum profile also permits STATIC/FINAL self-row metadata:
	// classfile ACC_FINAL and the absence of captures prove runtime finality
	// and ownership independently. Report regeneration metadata differences;
	// preserve every original executable forwarding operand and exception.
	legacyMetadata := obj.MajorVersion >= 49 && obj.MajorVersion <= 52 && obj.MinorVersion == 0
	selfMetadataDifferent := false
	owner, method, known := originalAnonymousOwner(obj)
	if !known || owner != parent.GetClassName() || method != "" || obj.GetClassName() != owner+"$"+strconv.Itoa(bodyOrdinal) {
		return nil
	}
	var assertions *nativeMemberAssertion
	if len(obj.Fields) != 0 {
		statusOwner, known := nativeEnumAssertionStatusOwner(parent, resolve, work)
		if !known {
			return nil
		}
		var valid bool
		assertions, valid = nativeMemberAssertionProof(obj, statusOwner, work)
		if !valid || assertions == nil || !assertions.pureInitializer {
			return nil
		}
	}
	selfRows, innerTables, enclosing, source := 0, 0, 0, 0
	for _, a := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return nil
		}
		switch a := a.(type) {
		case *InnerClassesAttribute:
			if a == nil || innerTables != 0 {
				return nil
			}
			innerTables++
			for _, row := range a.Classes {
				if row == nil || !nativeProofWork(work, 1) {
					return nil
				}
				name, ok := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
				if !ok {
					return nil
				}
				if row.OuterClassInfoIndex != 0 {
					declarationOwner, valid := sourceBridgeClassName(obj, row.OuterClassInfoIndex)
					if !valid || declarationOwner == obj.GetClassName() {
						return nil
					}
				}
				if row.InnerNameIndex == 0 && name != obj.GetClassName() && row.InnerClassAccessFlags&0x1000 == 0 {
					referenced, found := resolve(name)
					if !found || referenced == nil {
						return nil
					}
					nestedOwner, _, anonymous := originalAnonymousOwner(referenced)
					if anonymous && nestedOwner == obj.GetClassName() {
						return nil
					}
				}
				if name == obj.GetClassName() {
					selfRows++
					flags := row.InnerClassAccessFlags
					validFlags := flags == 0x4010 || legacyMetadata && flags&0x4000 != 0 && flags & ^uint16(0x4018) == 0
					selfMetadataDifferent = flags != 0x4010
					if row.OuterClassInfoIndex != 0 || row.InnerNameIndex != 0 || !validFlags {
						return nil
					}
				}
			}
		case *UnparsedAttribute:
			if a == nil || a.Name != "EnclosingMethod" || enclosing != 0 {
				return nil
			}
			enclosing++
		case *SourceFileAttribute:
			if a == nil || a.AttrLen != 2 || source != 0 {
				return nil
			}
			source++
			if _, ok := sourceBridgeUTF8(obj, a.SourceFileIndex); !ok {
				return nil
			}
		default:
			return nil
		}
	}
	if selfRows != 1 || innerTables != 1 || enclosing != 1 || !nativeMemberTypeScope(obj, nil, work) {
		return nil
	}
	var ctor *MemberInfo
	for _, m := range obj.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return nil
		}
		name, nk := sourceBridgeUTF8(obj, m.NameIndex)
		descriptor, dk := sourceBridgeUTF8(obj, m.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if name == "<clinit>" {
			if assertions != nil && assertions.initializer == m {
				continue
			}
			return nil
		}
		if name == "<init>" {
			if ctor != nil || descriptor != plan.descriptor || m.AccessFlags != 0 {
				return nil
			}
			ctor = m
		}
	}
	if ctor == nil {
		return nil
	}
	params, result, err := callbinding.Descriptor(plan.descriptor)
	if err != nil || result != "V" || len(params) < 2 || params[0] != "Ljava/lang/String;" || params[1] != "I" {
		return nil
	}
	var code *CodeAttribute
	parametersSeen := false
	for _, a := range ctor.Attributes {
		switch a := a.(type) {
		case *CodeAttribute:
			if a == nil || code != nil {
				return nil
			}
			code = a
		case *ExceptionsAttribute:
		case *UnparsedAttribute:
			if a == nil || a.Name != "MethodParameters" || parametersSeen || a.Length != uint32(1+4*len(params)) || len(a.Info) != 1+4*len(params) || int(a.Info[0]) != len(params) || !nativeProofWork(work, int64(len(a.Info))) {
				return nil
			}
			parametersSeen = true
			for i := range params {
				packet := a.Info[1+4*i : 5+4*i]
				expected := byte(0)
				if i < 2 {
					expected = 0x10
				}
				if packet[0] != 0 || packet[1] != 0 || packet[2] != expected || packet[3] != 0 {
					return nil
				}
			}
		case *SignatureAttribute:
			// This compiler-created subclass constructor has physical parameters,
			// without the enum declaration's hidden-parameter Signature projection.
			// A generic constructor needs a separately proved regenerated Signature.
			return nil
		default:
			return nil
		}
	}
	if (!parametersSeen && !legacyMetadata) || (parametersSeen && obj.MajorVersion < 52) || code == nil || len(code.ExceptionTable) != 0 || len(code.Code) > 512 || !nativeProofWork(work, int64(len(code.Code))) || code.MaxLocals != uint16(nativeMemberParameterWidth(params)+1) || code.MaxStack != uint16(nativeMemberParameterWidth(params)+2) {
		return nil
	}
	for _, a := range code.Attributes {
		if !nativeProofWork(work, 1) {
			return nil
		}
		switch a := a.(type) {
		case *LineNumberTableAttribute:
			if a == nil {
				return nil
			}
		case *UnparsedAttribute:
			if a == nil || a.Name != "LocalVariableTable" || a.Length != uint32(len(a.Info)) || len(a.Info) < 2 || len(a.Info) != 2+10*int(uint16(a.Info[0])<<8|uint16(a.Info[1])) || !nativeProofWork(work, int64(len(a.Info))) {
				return nil
			}
		default:
			return nil
		}
	}
	dec := core.NewDecompiler(code.Code, nil)
	dec.Work = work
	if dec.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(dec)
	if len(ops) != len(params)+4 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[0]) != 0 {
		return nil
	}
	slot := 1
	for i, p := range params {
		if !constructorMotionLoad(ops[i+1], p) || core.GetRetrieveIdx(ops[i+1]) != slot {
			return nil
		}
		slot += constructorEffectType(p).width()
	}
	null, call, ret := ops[len(ops)-3], ops[len(ops)-2], ops[len(ops)-1]
	if !nativeEnumOpcode(null, core.OP_ACONST_NULL) || !nativeEnumOpcode(ret, core.OP_RETURN) {
		return nil
	}
	target := constructorMotionMember(obj, call, core.OP_INVOKESPECIAL)
	if target == nil || target.Name != owner || target.Member != "<init>" || !nativeEnumMemberOperand(obj, call, core.OP_INVOKESPECIAL, owner, "<init>", target.Description) {
		return nil
	}
	bridge := bridges[target.Description]
	if bridge == nil || bridge.target != plan.descriptor {
		return nil
	}
	bridgeParams, bridgeResult, err := callbinding.Descriptor(bridge.descriptor)
	if err != nil || bridgeResult != "V" || len(bridgeParams) != len(params)+1 || !slices.Equal(bridgeParams[:len(params)], params) {
		return nil
	}
	// Exceptions are part of the generated constructor ABI, even when its body
	// is a pure forwarding packet. Source regeneration inherits the enum target.
	exceptions := func(object *ClassObject, m *MemberInfo) ([]string, bool) {
		var names []string
		count := 0
		for _, a := range m.Attributes {
			if !nativeProofWork(work, 1) {
				return nil, false
			}
			if a, ok := a.(*ExceptionsAttribute); ok {
				if a == nil {
					return nil, false
				}
				count++
				for _, i := range a.ExceptionIndexTable {
					if !nativeProofWork(work, 1) {
						return nil, false
					}
					n, ok := sourceBridgeClassName(object, i)
					if !ok {
						return nil, false
					}
					names = append(names, n)
				}
			}
		}
		return names, count <= 1
	}
	a, ak := exceptions(obj, ctor)
	b, bk := exceptions(parent, bridge.method)
	if !ak || !bk || !slices.Equal(a, b) {
		return nil
	}
	return &nativeEnumConstantBody{object: obj, owner: owner, descriptor: plan.descriptor, plan: plan, superDescriptor: bridge.descriptor, superPC: int(call.CurrentOffset), legacyConstructorMetadata: legacyMetadata && (!parametersSeen || selfMetadataDifferent), assertions: assertions}
}

func nativeEnumConstantConstructorOwned(p *nativeMemberFamily, obj *ClassObject, descriptor string) bool {
	if p == nil || obj == nil {
		return false
	}
	body := p.enumConstants[obj.GetClassName()]
	return body != nil && body.object == obj && body.descriptor == descriptor && p.children[body.owner] != nil && p.children[body.owner].enumSynthesis != nil && p.children[body.owner].enumSynthesis.bodies[obj.GetClassName()] == body
}

// A discovery read is separately parsed. Recheck it against the enum's sealed
// allocation/constructor certificate before excluding it from expression
// discovery. An unnamed row, enum flag or matching binary name alone cannot
// supply this role, and an unknown enum body remains a discovery obligation.
func (c *ClassObjectDumper) nativeMemberEnumConstantAnonymousRole(p *nativeMemberFamily, object *ClassObject) bool {
	if c == nil || p == nil || object == nil || !nativeProofWork(c.Work, 1) {
		return false
	}
	name := object.GetClassName()
	body := p.enumConstants[name]
	if body == nil || body.object == nil || object.MajorVersion != body.object.MajorVersion || object.MinorVersion != body.object.MinorVersion || p.lexicalObjects[name] != body.object || !nativeEnumConstantConstructorOwned(p, body.object, body.descriptor) {
		return false
	}
	parent := p.children[body.owner]
	if parent.object == nil || !isGenuineEnum(parent.object) || body.plan.allocatedClass != name || body.plan.descriptor != body.descriptor {
		return false
	}
	if original, found := parent.enumSynthesis.constants[body.plan.constant]; !found || original != body.plan {
		return false
	}
	ordinal, err := strconv.Atoi(strings.TrimPrefix(name, body.owner+"$"))
	if err != nil || ordinal <= 0 {
		return false
	}
	resolve := func(binary string) (*ClassObject, bool) {
		if binary == name {
			return object, true
		}
		if original := p.lexicalObjects[binary]; original != nil {
			return original, true
		}
		if c.foldSiblingResolver == nil {
			return nil, false
		}
		raw, known := c.foldSiblingResolver(binary)
		if !known {
			return nil, false
		}
		parsed, err := c.parseResolved(raw)
		return parsed, err == nil && parsed != nil && parsed.GetClassName() == binary
	}
	proof := nativeEnumConstantBodyProof(parent.object, body.plan, ordinal, parent.accessBridges, resolve, c.Work)
	return proof != nil && proof.owner == body.owner && proof.descriptor == body.descriptor && proof.superDescriptor == body.superDescriptor && proof.superPC == body.superPC
}
func nativeEnumConstantSuperOwned(p *nativeMemberFamily, obj *ClassObject, name, descriptor, target, physical string, pc int) bool {
	if name != "<init>" || !nativeEnumConstantConstructorOwned(p, obj, descriptor) {
		return false
	}
	body := p.enumConstants[obj.GetClassName()]
	return body.owner == target && body.superDescriptor == physical && body.superPC == pc
}
func nativeEnumConstantAllocationOwned(p *nativeMemberFamily, obj *ClassObject, m *MemberInfo, pc int, target string) bool {
	if p == nil || obj == nil || m == nil {
		return false
	}
	body := p.enumConstants[target]
	if body == nil || body.owner != obj.GetClassName() {
		return false
	}
	name, nk := sourceBridgeUTF8(obj, m.NameIndex)
	desc, dk := sourceBridgeUTF8(obj, m.DescriptorIndex)
	return nk && dk && name == "<clinit>" && desc == "()V" && body.plan.newPC == pc
}
func nativeEnumConstantsSourceClosed(p *nativeMemberFamily) bool {
	if p == nil {
		return false
	}
	for _, body := range p.enumConstants {
		if body == nil || !body.rendered {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) foldNativeEnumConstantBodies() (map[string]string, bool) {
	current, p := c.nativeMemberCurrent, c.nativeMemberRoot
	if current == nil || current.enumSynthesis == nil || len(current.enumSynthesis.bodies) == 0 {
		return nil, false
	}
	out := map[string]string{}
	for name, plan := range current.enumSynthesis.constants {
		body := current.enumSynthesis.bodies[plan.allocatedClass]
		if body == nil {
			continue
		}
		if p == nil || p.enumConstants[plan.allocatedClass] != body || body.object.GetClassName() != plan.allocatedClass {
			if p != nil {
				p.failed = true
			}
			return nil, true
		}
		rendered := c.renderFoldedConstantObject(body.object, c.GetConstructorMethodName())
		if rendered == "" {
			p.failed = true
			return nil, true
		}
		out[name] = rendered
		body.rendered = true
		if body.legacyConstructorMetadata {
			c.appendDiagnostic(DecompileDiagnostic{Code: "enum_constant_legacy_metadata", Method: body.object.GetClassName() + ".<init>" + body.descriptor, Message: "The original pre-nestmate enum constant constructor packet is proved. Recompilation may add unnamed generated parameter metadata and different STATIC/FINAL bits to its anonymous InnerClasses self row; executable descriptor, final class, forwarding operands and exceptions are preserved."})
		}
	}
	if len(out) != len(current.enumSynthesis.bodies) {
		p.failed = true
		return nil, true
	}
	return out, true
}

// Constant classes have no Java source name. Their original references must
// therefore be either metadata or the single proved enum initialization;
// an unrelated allocation, typed declaration or class literal cannot acquire
// that ownership merely because the class carries ACC_ENUM.
func (z *JarFS) nativeEnumConstantsArchiveClosed(p *nativeMemberFamily, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if p == nil || index == nil || !index.valid || len(p.enumConstants) > 64 {
		return false
	}
	hidden := map[string]bool{}
	users := map[string]bool{}
	addUser := func(user string) bool {
		if !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(users)+1)*96) != nil {
			return false
		}
		users[user] = true
		return true
	}
	for name, body := range p.enumConstants {
		if body == nil || body.object == nil || !nativeProofWork(work, 1) {
			return false
		}
		hidden[name] = true
		if !addUser(name) || !addUser(body.owner) {
			return false
		}
		if index.handles[name] {
			return false
		}
		for user := range index.typeUsers[name] {
			if !addUser(user) {
				return false
			}
		}
	}
	for user := range users {
		if !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(users))*96) != nil {
			return false
		}
		raw, found := z.enumSiblingResolver()(user)
		if !found {
			return false
		}
		obj, err := z.nativeMemberReader(nil).parseResolved(raw)
		if err != nil || obj.GetClassName() != user || hidden[obj.GetSupperClassName()] || !nativeMemberBridgeMarkerAttributesClosed(obj, hidden, work) {
			return false
		}
		descriptorClosed := func(desc string) bool {
			if !nativeProofWork(work, int64(len(desc))*int64(len(hidden))) {
				return false
			}
			for name := range hidden {
				if strings.Contains(desc, "L"+name+";") {
					return false
				}
			}
			return true
		}
		for _, m := range append(append([]*MemberInfo{}, obj.Fields...), obj.Methods...) {
			if m == nil || !nativeProofWork(work, 1) {
				return false
			}
			desc, ok := sourceBridgeUTF8(obj, m.DescriptorIndex)
			if !ok || !descriptorClosed(desc) {
				return false
			}
		}
		for _, constant := range obj.ConstantPool {
			if !nativeProofWork(work, 1) {
				return false
			}
			if nt, ok := constant.(*ConstantNameAndTypeInfo); ok {
				if nt == nil {
					return false
				}
				desc, known := sourceBridgeUTF8(obj, nt.DescriptorIndex)
				if !known || !descriptorClosed(desc) {
					return false
				}
			}
			if mt, ok := constant.(*ConstantMethodTypeInfo); ok {
				if mt == nil {
					return false
				}
				desc, known := sourceBridgeUTF8(obj, mt.DescriptorIndex)
				if !known || !descriptorClosed(desc) {
					return false
				}
			}
			member := nativeConstantMember(constant)
			if member == nil {
				continue
			}
			target, ok := sourceBridgeClassName(obj, member.ClassIndex)
			if !ok {
				return false
			}
			if !hidden[target] {
				continue
			}
			if member.NameAndTypeIndex == 0 || int(member.NameAndTypeIndex) > len(obj.ConstantPool) {
				return false
			}
			nt, ok := obj.ConstantPool[member.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			if !ok || nt == nil {
				return false
			}
			name, nk := sourceBridgeUTF8(obj, nt.NameIndex)
			desc, dk := sourceBridgeUTF8(obj, nt.DescriptorIndex)
			_, methodRef := constant.(*ConstantMethodrefInfo)
			_, fieldRef := constant.(*ConstantFieldrefInfo)
			if !nk || !dk {
				return false
			}
			if fieldRef {
				// javac may use the constant subclass as the symbolic owner of a
				// nonprivate inherited enum field. It is still this exact receiver and
				// the original enum declaration, never a foreign anonymous source type.
				if user != target || !(nativeEnumConstantInheritedField(p, target, name, desc, work) || nativeEnumConstantAssertionFieldOwned(p, target, name, desc, work)) {
					return false
				}
				continue
			}
			if !methodRef {
				return false
			}
			if name == "<init>" {
				body := p.enumConstants[target]
				if body.owner != user || body.descriptor != desc {
					return false
				}
			} else if user != target {
				return false
			}
		}
		for _, m := range obj.Methods {
			for _, a := range m.Attributes {
				code, ok := a.(*CodeAttribute)
				if !ok {
					continue
				}
				if code == nil || !nativeProofWork(work, int64(len(code.Code))) {
					return false
				}
				d := core.NewDecompiler(code.Code, nil)
				d.Work = work
				if d.ParseOpcode() != nil {
					return false
				}
				for _, op := range d.Opcodes() {
					if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
						return false
					}
					var target string
					var known bool
					switch op.Instr.OpCode {
					case core.OP_NEW, core.OP_CHECKCAST, core.OP_INSTANCEOF, core.OP_ANEWARRAY, core.OP_MULTIANEWARRAY:
						if len(op.Data) < 2 {
							return false
						}
						target, known = sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data[:2]))
					case core.OP_LDC:
						if len(op.Data) != 1 {
							return false
						}
						target, known = sourceBridgeClassName(obj, uint16(op.Data[0]))
					case core.OP_LDC_W:
						if len(op.Data) != 2 {
							return false
						}
						target, known = sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data))
					}
					if known && (!descriptorClosed(target) || hidden[target] && (op.Instr.OpCode != core.OP_NEW || !nativeEnumConstantAllocationOwned(p, obj, m, int(op.CurrentOffset), target))) {
						return false
					}
					call := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL)
					if call != nil && hidden[call.Name] && call.Member == "<init>" {
						body := p.enumConstants[call.Name]
						name, nk := sourceBridgeUTF8(obj, m.NameIndex)
						desc, dk := sourceBridgeUTF8(obj, m.DescriptorIndex)
						if !nk || !dk || user != body.owner || name != "<clinit>" || desc != "()V" || int(op.CurrentOffset) != body.plan.invokePC || call.Description != body.descriptor {
							return false
						}
					}
				}
			}
		}
	}
	return true
}

func nativeEnumConstantInheritedField(p *nativeMemberFamily, bodyName, name, descriptor string, work *workbudget.Budget) bool {
	body := p.enumConstants[bodyName]
	if body == nil || p.children[body.owner] == nil || len(body.object.Fields) != 0 {
		return false
	}
	count := 0
	for _, field := range p.children[body.owner].object.Fields {
		if field == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(p.children[body.owner].object, field.NameIndex)
		d, dk := sourceBridgeUTF8(p.children[body.owner].object, field.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if n == name && d == descriptor {
			if field.AccessFlags&(0x1000|2) != 0 {
				return false
			}
			count++
		}
	}
	return count == 1
}

// The only newly source-owned field is the independently certified assertion
// flag of this exact constant body. Its full read/write and handle closure has
// already been checked on original code; foreign field users remain refused.
func nativeEnumConstantAssertionFieldOwned(p *nativeMemberFamily, owner, name, descriptor string, work *workbudget.Budget) bool {
	if p == nil || !nativeProofWork(work, 1) {
		return false
	}
	body := p.enumConstants[owner]
	return body != nil && body.object != nil && body.object.GetClassName() == owner && body.assertions != nil && body.assertions.pureInitializer && body.assertions.initializer != nil && name == nativeAssertionField && descriptor == "Z"
}
