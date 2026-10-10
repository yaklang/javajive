package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// InnerClasses describes declarations; it grants no JVM private permission.
// Before joining source scopes, resolve the original used symbols that could
// select an emitted private declaration. Legacy accessor calls remain legal:
// their package method calls and the private operations inside the declaring
// class are checked separately. Unused constant-pool entries are not accesses.
func nativeMemberPrivatePermissionClosed(p *nativeMemberFamily, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if p == nil || p.failed || resolve == nil || len(p.lexicalObjects) == 0 || len(p.lexicalObjects) > 4096 || !nativeProofWork(work, 1) {
		return false
	}
	private := map[string]bool{}
	memberCount, privateBytes := int64(0), int64(0)
	key := func(field bool, name, desc string) string {
		kind := "method\x00"
		if field {
			kind = "field\x00"
		}
		return kind + name + "\x00" + desc
	}
	for name, object := range p.lexicalObjects {
		if object == nil || object.GetClassName() != name || !nativeProofWork(work, 1) {
			return false
		}
		for _, field := range []bool{true, false} {
			members := object.Methods
			if field {
				members = object.Fields
			}
			for _, member := range members {
				memberCount++
				if member == nil || memberCount > 65536 || !nativeProofWork(work, 1) {
					return false
				}
				if member.AccessFlags&2 == 0 {
					continue
				}
				n, nk := sourceBridgeUTF8(object, member.NameIndex)
				d, dk := sourceBridgeUTF8(object, member.DescriptorIndex)
				if !nk || !dk {
					return false
				}
				identity := key(field, n, d)
				if !private[identity] {
					privateBytes += int64(len(identity)) + 128
					if privateBytes > 16<<20 || work != nil && work.CheckAlloc(privateBytes) != nil {
						return false
					}
					private[identity] = true
				}
			}
		}
	}
	if len(private) == 0 {
		return true
	}
	if work != nil && work.CheckAlloc(int64(len(private)+len(p.lexicalObjects))*128) != nil {
		return false
	}
	original := func(name string) (*ClassObject, bool) {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if obj := p.lexicalObjects[name]; obj != nil {
			return obj, obj.GetClassName() == name
		}
		obj, known := resolve(name)
		return obj, known && obj != nil && obj.GetClassName() == name
	}
	codeBytes := int64(0)
	for _, caller := range p.lexicalObjects {
		check := func(op *core.OpCode) bool {
			if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
				return false
			}
			kind := op.Instr.OpCode
			field := kind == core.OP_GETFIELD || kind == core.OP_PUTFIELD || kind == core.OP_GETSTATIC || kind == core.OP_PUTSTATIC
			ref := constructorMotionMember(caller, op, kind)
			if ref == nil {
				return false
			}
			if !private[key(field, ref.Member, ref.Description)] {
				return true
			}
			owner, member, known := nativePrivatePermissionTarget(ref.Name, ref.Member, ref.Description, field, original, work)
			if !known {
				return false
			}
			if member == nil || member.AccessFlags&2 == 0 || p.lexicalObjects[owner.GetClassName()] != owner {
				return true
			}
			static := kind == core.OP_GETSTATIC || kind == core.OP_PUTSTATIC || kind == core.OP_INVOKESTATIC
			if (member.AccessFlags&8 != 0) != static {
				return false
			}
			return caller == owner || nativeModernNestPrivateConstructorAccess(p, caller, owner, work)
		}
		constant := func(index uint16) ConstantInfo {
			if index == 0 || int(index) > len(caller.ConstantPool) {
				return nil
			}
			return caller.ConstantPool[index-1]
		}
		handle := func(index uint16) bool {
			h, ok := constant(index).(*ConstantMethodHandleInfo)
			if !ok || h == nil || h.ReferenceKind < 1 || h.ReferenceKind > 9 {
				return false
			}
			kind := []int{0, core.OP_GETFIELD, core.OP_GETSTATIC, core.OP_PUTFIELD, core.OP_PUTSTATIC, core.OP_INVOKEVIRTUAL, core.OP_INVOKESTATIC, core.OP_INVOKESPECIAL, core.OP_INVOKESPECIAL, core.OP_INVOKEINTERFACE}[h.ReferenceKind]
			d := []byte{byte(h.ReferenceIndex >> 8), byte(h.ReferenceIndex)}
			if kind == core.OP_INVOKEINTERFACE {
				d = append(d, 1, 0)
			}
			return check(&core.OpCode{Instr: &core.Instruction{OpCode: kind}, Data: d})
		}
		var boot *BootstrapMethodsAttribute
		for _, attr := range caller.Attributes {
			if !nativeProofWork(work, 1) {
				return false
			}
			if b, ok := attr.(*BootstrapMethodsAttribute); ok {
				if b == nil || boot != nil || int(b.NumBootstrapMethods) != len(b.BootstrapMethods) {
					return false
				}
				boot = b
			}
		}
		for _, method := range caller.Methods {
			if method == nil || !nativeProofWork(work, 1) {
				return false
			}
			seen := false
			for _, attr := range method.Attributes {
				code, ok := attr.(*CodeAttribute)
				if !ok {
					continue
				}
				if code == nil || seen {
					return false
				}
				codeBytes += int64(len(code.Code))
				if len(code.Code) > 1<<20 || codeBytes > 32<<20 || !nativeProofWork(work, int64(len(code.Code))) || work != nil && work.CheckAlloc(int64(len(code.Code))*32+privateBytes) != nil {
					return false
				}
				seen = true
				decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, index) })
				decoder.Work = work
				if decoder.ParseOpcode() != nil {
					return false
				}
				for _, op := range decoder.Opcodes() {
					if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
						return false
					}
					switch op.Instr.OpCode {
					case core.OP_GETFIELD, core.OP_PUTFIELD, core.OP_GETSTATIC, core.OP_PUTSTATIC, core.OP_INVOKEVIRTUAL, core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE:
						if !check(op) {
							return false
						}
					case core.OP_LDC, core.OP_LDC_W, core.OP_LDC2_W:
						var index uint16
						if len(op.Data) == 1 {
							index = uint16(op.Data[0])
						} else if len(op.Data) == 2 {
							index = binary.BigEndian.Uint16(op.Data)
						} else {
							return false
						}
						if _, ok := constant(index).(*ConstantMethodHandleInfo); ok && !handle(index) {
							return false
						}
					case core.OP_INVOKEDYNAMIC:
						if len(op.Data) != 4 || op.Data[2] != 0 || op.Data[3] != 0 {
							return false
						}
						site, ok := constant(binary.BigEndian.Uint16(op.Data)).(*ConstantInvokeDynamicInfo)
						if !ok || site == nil || boot == nil || int(site.BootstrapMethodAttrIndex) >= len(boot.BootstrapMethods) {
							return false
						}
						b := boot.BootstrapMethods[site.BootstrapMethodAttrIndex]
						if b == nil || int(b.NumBootstrapArguments) != len(b.BootstrapArguments) || !handle(b.BootstrapMethodRef) {
							return false
						}
						for _, index := range b.BootstrapArguments {
							if !nativeProofWork(work, 1) {
								return false
							}
							if _, ok := constant(index).(*ConstantMethodHandleInfo); ok && !handle(index) {
								return false
							}
						}
					}
				}
			}
		}
	}
	return true
}

// Resolve exact JVM name+descriptor identities. Fields search interfaces before
// the superclass. Methods search the class chain; inherited interface methods
// cannot supply private declarations. Constructors never inherit. Missing or
// cyclic metadata is unproved, rather than permission to use a lexical name.
func nativePrivatePermissionTarget(owner, name, desc string, field bool, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) (*ClassObject, *MemberInfo, bool) {
	state := map[string]uint8{}
	nodes := 0
	var visit func(string, int) (*ClassObject, *MemberInfo, bool)
	visit = func(binary string, depth int) (*ClassObject, *MemberInfo, bool) {
		if depth >= 64 || state[binary] == 1 || !nativeProofWork(work, 1) {
			return nil, nil, false
		}
		if state[binary] == 2 {
			return nil, nil, true
		}
		nodes++
		if nodes > 256 || work != nil && work.CheckAlloc(int64(nodes)*128) != nil {
			return nil, nil, false
		}
		object, known := resolve(binary)
		if !known || object == nil || object.GetClassName() != binary {
			return nil, nil, false
		}
		state[binary] = 1
		defer func() { state[binary] = 2 }()
		members := object.Methods
		if field {
			members = object.Fields
		}
		var target *MemberInfo
		for _, member := range members {
			if member == nil || !nativeProofWork(work, 1) {
				return nil, nil, false
			}
			n, nk := sourceBridgeUTF8(object, member.NameIndex)
			d, dk := sourceBridgeUTF8(object, member.DescriptorIndex)
			if !nk || !dk {
				return nil, nil, false
			}
			if n == name && d == desc {
				if target != nil {
					return nil, nil, false
				}
				target = member
			}
		}
		if target != nil {
			return object, target, true
		}
		if !field && (name == "<init>" || object.AccessFlags&0x0200 != 0) {
			return nil, nil, true
		}
		if field {
			for _, index := range object.Interfaces {
				n, known := sourceBridgeClassName(object, index)
				if !known {
					return nil, nil, false
				}
				o, m, valid := visit(n, depth+1)
				if !valid || m != nil {
					return o, m, valid
				}
			}
		}
		if object.SuperClass == 0 {
			return nil, nil, true
		}
		parent, known := sourceBridgeClassName(object, object.SuperClass)
		if !known {
			return nil, nil, false
		}
		return visit(parent, depth+1)
	}
	return visit(owner, 0)
}
