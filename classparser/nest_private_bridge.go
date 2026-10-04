package javaclassparser

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

type privateNestTarget struct{ name, descriptor string }
type privateNestSite struct {
	caller, method, descriptor string
	pc                         int
	kind                       uint8
	target                     privateNestTarget
}
type privateNestPlan struct {
	owner   *ClassObject
	bridges map[privateNestTarget]string
	sites   map[privateNestSite]bool
}

// The owner token is a lossless byte encoding, not a hash or a source spelling.
// Two original private methods in different owners may legally have identical
// descriptors, even when one owner inherits the other. Their package-visible
// final helpers must never acquire an overriding relationship. '$' cannot occur
// inside a hex token, so appending '$' for original-member collision avoidance
// also cannot enter another owner's naming sequence.
func privateNestBridgeBaseName(owner string, index int) (string, bool) {
	if owner == "" || index < 0 {
		return "", false
	}
	suffix := fmt.Sprintf("$%d", index)
	const prefix = "jdec$private$"
	// Generated names are ASCII, hence their UTF-8 classfile length is exactly
	// their source length. Refuse an owner whose complete identity cannot fit;
	// shortening it would discard the uniqueness proof.
	if len(owner) > (65535-len(prefix)-len(suffix))/2 {
		return "", false
	}
	return prefix + hex.EncodeToString([]byte(owner)) + suffix, true
}

// Complete identity names amplify an otherwise small set of invoke targets.
// Bound retained proof names separately from the original bytecode scan.
func privateNestBridgeNameBudget(previous, next int) (int, bool) {
	const limit = 1 << 20
	if previous < 0 || next < 0 || previous > limit || next > limit-previous {
		return 0, false
	}
	return previous + next, true
}

// Nest attributes are the VM's access evidence. '$' spelling and InnerClasses
// are not sufficient: unrelated classes can have both without sharing a nest.
func originalNestAttribute(obj *ClassObject, name string) ([]string, bool) {
	var result []string
	found := false
	for _, attr := range obj.Attributes {
		raw, ok := attr.(*UnparsedAttribute)
		if !ok || raw == nil || raw.Name != name {
			continue
		}
		if found || obj.MajorVersion < 55 || len(raw.Info) < 2 {
			return nil, false
		}
		found = true
		count, start := 1, 0
		if name == "NestMembers" {
			count, start = int(binary.BigEndian.Uint16(raw.Info)), 2
		}
		if count > 128 || len(raw.Info) != start+count*2 {
			return nil, false
		}
		for i := 0; i < count; i++ {
			index := binary.BigEndian.Uint16(raw.Info[start+i*2:])
			constant, err := obj.getConstantInfo(index)
			class, ok := constant.(*ConstantClassInfo)
			if err != nil || !ok || class == nil {
				return nil, false
			}
			n, err := obj.getUtf8(class.NameIndex)
			if err != nil || n == "" || slices.Contains(result, n) {
				return nil, false
			}
			result = append(result, n)
		}
	}
	return result, found
}

func hasOriginalNestAttribute(obj *ClassObject, name string) bool {
	for _, attr := range obj.Attributes {
		if raw, ok := attr.(*UnparsedAttribute); ok && raw != nil && raw.Name == name {
			return true
		}
	}
	return false
}

func nestInterfaceMemberRef(object *ClassObject, index int) bool {
	constant, err := object.getConstantInfo(uint16(index))
	_, ok := constant.(*ConstantInterfaceMethodrefInfo)
	return err == nil && ok
}

func nestPackage(n string) string {
	if i := strings.LastIndexByte(n, '/'); i >= 0 {
		return n[:i]
	}
	return ""
}

func (c *ClassObjectDumper) buildPrivateNestPlan(owner *ClassObject, load func(string) (*ClassObject, bool)) (*privateNestPlan, error) {
	if owner == nil || owner.MajorVersion < 55 || owner.AccessFlags&0x200 != 0 {
		return nil, nil // Java 8 synthetic accessors already retain the binding.
	}
	eligible := false
	for _, method := range owner.Methods {
		name, err := owner.getUtf8(method.NameIndex)
		if err != nil {
			return nil, nil
		}
		if method.AccessFlags&2 != 0 && method.AccessFlags&(8|0x1000) == 0 && name != "<init>" {
			eligible = true
			break
		}
	}
	if !eligible {
		return nil, nil
	}
	host := owner
	if names, ok := originalNestAttribute(owner, "NestHost"); ok {
		var available bool
		host, available = load(names[0])
		if !available || host == nil || host.GetClassName() == owner.GetClassName() {
			return nil, nil
		}
	} else if hasOriginalNestAttribute(owner, "NestHost") {
		return nil, nil
	}
	if hasOriginalNestAttribute(host, "NestHost") {
		return nil, nil
	}
	members, ok := originalNestAttribute(host, "NestMembers")
	if !ok || len(members) == 0 || slices.Contains(members, host.GetClassName()) {
		return nil, nil
	}
	if owner != host && !slices.Contains(members, owner.GetClassName()) {
		return nil, nil
	}
	classes := []*ClassObject{host}
	for _, name := range members {
		if err := c.checkWork(); err != nil {
			return nil, err
		}
		member, available := load(name)
		if !available || member == nil || member.GetClassName() != name || nestPackage(name) != nestPackage(host.GetClassName()) {
			return nil, nil
		}
		h, reciprocal := originalNestAttribute(member, "NestHost")
		if !reciprocal || h[0] != host.GetClassName() {
			return nil, nil
		}
		if hasOriginalNestAttribute(member, "NestMembers") {
			return nil, nil
		}
		classes = append(classes, member)
	}
	private := map[privateNestTarget]*MemberInfo{}
	reserved := map[string]bool{}
	provider := c.buildInvocationMetadata()
	parents := append([]string{owner.GetSupperClassName()}, owner.GetInterfacesName()...)
	seenParents := map[string]bool{}
	for len(parents) > 0 {
		name := parents[0]
		parents = parents[1:]
		if name == "" || seenParents[name] {
			continue
		}
		if len(seenParents) >= 128 {
			return nil, nil
		}
		seenParents[name] = true
		metadata, known := provider(name)
		if !known || !metadata.MembersComplete || !metadata.ParentsComplete {
			return nil, nil
		}
		for _, method := range metadata.Methods {
			reserved[class_context.SafeIdentifier(method.Name)] = true
		}
		parents = append(parents, metadata.Parents...)
	}
	// A final instance bridge cannot be shadowed by any reconstructed nest
	// subclass. Reserve the entire closed nest's original member names.
	for _, member := range classes {
		for _, method := range member.Methods {
			name, err := member.getUtf8(method.NameIndex)
			if err != nil {
				return nil, nil
			}
			reserved[class_context.SafeIdentifier(name)] = true
		}
	}
	for _, method := range owner.Methods {
		name, e1 := owner.getUtf8(method.NameIndex)
		desc, e2 := owner.getUtf8(method.DescriptorIndex)
		if e1 != nil || e2 != nil {
			return nil, nil
		}
		reserved[class_context.SafeIdentifier(name)] = true
		// A synthetic lambda implementation is emitted inline, not as a member.
		if method.AccessFlags&2 != 0 && method.AccessFlags&(8|0x1000) == 0 && name != "<init>" {
			// Raw owner erasure covers class formals. Method formals require an
			// independent invocation instantiation proof; an intersection bound
			// cannot be synthesized by inserting extra runtime CHECKCASTs here.
			methodFormals := false
			for _, attr := range method.Attributes {
				if signature, ok := attr.(*SignatureAttribute); ok {
					sig, err := owner.getUtf8(signature.SignatureIndex)
					if err != nil || strings.HasPrefix(sig, "<") {
						methodFormals = true
					}
				}
			}
			if methodFormals {
				continue
			}
			key := privateNestTarget{name, desc}
			if private[key] != nil {
				return nil, nil
			}
			private[key] = method
		}
	}
	for _, field := range owner.Fields {
		name, err := owner.getUtf8(field.NameIndex)
		if err != nil {
			return nil, nil
		}
		reserved[class_context.SafeIdentifier(name)] = true
	}
	plan := &privateNestPlan{owner: owner, bridges: map[privateNestTarget]string{}, sites: map[privateNestSite]bool{}}
	used := map[privateNestTarget]bool{}
	work := 0
	for _, member := range classes {
		if member.GetClassName() == owner.GetClassName() {
			continue
		}
		for _, method := range member.Methods {
			if err := c.checkWork(); err != nil {
				return nil, err
			}
			methodName, e1 := member.getUtf8(method.NameIndex)
			methodDesc, e2 := member.getUtf8(method.DescriptorIndex)
			if e1 != nil || e2 != nil {
				return nil, nil
			}
			for _, attr := range method.Attributes {
				code, ok := attr.(*CodeAttribute)
				if !ok {
					continue
				}
				work += len(code.Code)
				if work > 1<<20 {
					return nil, nil
				}
				decoder := core.NewDecompiler(code.Code, nil)
				if err := decoder.ParseOpcode(); err != nil {
					return nil, nil
				}
				for _, op := range decoder.Opcodes() {
					if op == nil || op.Instr == nil {
						continue
					}
					var kind values.InvokeKind
					switch op.Instr.OpCode {
					case core.OP_INVOKEVIRTUAL:
						kind = values.InvokeVirtual
					case core.OP_INVOKESPECIAL:
						kind = values.InvokeSpecial
					case core.OP_INVOKEINTERFACE:
						kind = values.InvokeInterface
					default:
						continue
					}
					if len(op.Data) < 2 {
						return nil, nil
					}
					callee, ok := GetValueFromCP(member.ConstantPool, int(core.Convert2bytesToInt(op.Data[:2]))).(*values.JavaClassMember)
					if !ok || callee == nil || strings.ReplaceAll(callee.Name, ".", "/") != owner.GetClassName() {
						continue
					}
					target := privateNestTarget{callee.Member, callee.Description}
					if private[target] == nil {
						continue
					}
					if (owner.AccessFlags&0x200 != 0) != (kind == values.InvokeInterface || kind == values.InvokeSpecial && nestInterfaceMemberRef(member, int(core.Convert2bytesToInt(op.Data[:2])))) {
						// A virtual class invocation cannot witness an interface member.
						continue
					}
					used[target] = true
					plan.sites[privateNestSite{member.GetClassName(), methodName, methodDesc, int(op.CurrentOffset), uint8(kind), target}] = true
				}
			}
		}
	}
	keys := make([]privateNestTarget, 0, len(used))
	for key := range used {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b privateNestTarget) int {
		return strings.Compare(a.name+"\x00"+a.descriptor, b.name+"\x00"+b.descriptor)
	})
	nameBytes := 0
	for i, key := range keys {
		name, known := privateNestBridgeBaseName(owner.GetClassName(), i)
		if !known {
			return nil, nil
		}
		for reserved[name] {
			if _, fits := privateNestBridgeNameBudget(nameBytes, len(name)+1); len(name) >= 65535 || !fits {
				return nil, nil
			}
			if err := c.checkWork(); err != nil {
				return nil, err
			}
			name += "$"
		}
		var fits bool
		nameBytes, fits = privateNestBridgeNameBudget(nameBytes, len(name))
		if !fits {
			return nil, nil
		}
		reserved[name] = true
		plan.bridges[key] = name
	}
	return plan, nil
}

func (c *ClassObjectDumper) wirePrivateNestBridges() {
	if c.obj.MajorVersion < 55 || (!hasOriginalNestAttribute(c.obj, "NestHost") && !hasOriginalNestAttribute(c.obj, "NestMembers")) {
		return
	}
	cache := map[string]*ClassObject{c.obj.GetClassName(): c.obj}
	load := func(name string) (*ClassObject, bool) {
		if obj, seen := cache[name]; seen {
			return obj, obj != nil
		}
		cache[name] = nil
		if c.foldSiblingResolver == nil {
			return nil, false
		}
		data, ok := c.foldSiblingResolver(name)
		if !ok {
			return nil, false
		}
		obj, err := c.parseResolved(data)
		if err != nil || obj.GetClassName() != name {
			return nil, false
		}
		cache[name] = obj
		return obj, true
	}
	plans := map[string]*privateNestPlan{}
	getPlan := func(owner string) *privateNestPlan {
		if plan, seen := plans[owner]; seen {
			return plan
		}
		plans[owner] = nil
		obj, ok := load(owner)
		if !ok {
			return nil
		}
		plan, err := c.buildPrivateNestPlan(obj, load)
		if err != nil {
			return nil
		}
		plans[owner] = plan
		return plan
	}
	c.privateNestOwnPlan = getPlan(c.obj.GetClassName())
	c.FuncCtx.PrivateNestBridge = func(owner, name, descriptor string, kind uint8, pc int) (string, bool) {
		owner = strings.ReplaceAll(owner, ".", "/")
		if owner == c.obj.GetClassName() {
			return "", false
		}
		plan := getPlan(owner)
		if plan == nil {
			return "", false
		}
		target := privateNestTarget{name, descriptor}
		site := privateNestSite{c.obj.GetClassName(), c.FuncCtx.FunctionName, c.FuncCtx.CurrentMethodDesc, pc, kind, target}
		return plan.bridges[target], plan.sites[site] && plan.bridges[target] != ""
	}
}

func (c *ClassObjectDumper) privateNestHelpers() ([]*dumpedMethods, error) {
	plan := c.privateNestOwnPlan
	if plan == nil {
		return nil, nil
	}
	keys := make([]privateNestTarget, 0, len(plan.bridges))
	for key := range plan.bridges {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b privateNestTarget) int {
		return strings.Compare(a.name+"\x00"+a.descriptor, b.name+"\x00"+b.descriptor)
	})
	var helpers []*dumpedMethods
	owner := types.NewJavaClass(strings.ReplaceAll(c.obj.GetClassName(), "/", ".")).String(c.FuncCtx)
	for _, key := range keys {
		parsed, err := types.ParseMethodDescriptor(key.descriptor)
		if err != nil || parsed == nil || parsed.FunctionType() == nil {
			return nil, fmt.Errorf("invalid private nest bridge descriptor")
		}
		ft := parsed.FunctionType()
		params, args := []string{}, []string{}
		for i, typ := range ft.ParamTypes {
			param := fmt.Sprintf("arg%d", i)
			params = append(params, typ.String(c.FuncCtx)+" "+param)
			args = append(args, param)
		}
		var member *MemberInfo
		for _, m := range c.obj.Methods {
			n, _ := c.obj.getUtf8(m.NameIndex)
			d, _ := c.obj.getUtf8(m.DescriptorIndex)
			if n == key.name && d == key.descriptor {
				member = m
				break
			}
		}
		exceptions, known := originalMethodExceptions(c.obj, member)
		if !known {
			return nil, fmt.Errorf("unknown private bridge Exceptions")
		}
		var throws []string
		for _, e := range exceptions {
			throws = append(throws, types.NewJavaClass(strings.ReplaceAll(e, "/", ".")).String(c.FuncCtx))
		}
		clause := ""
		if len(throws) > 0 {
			clause = " throws " + strings.Join(throws, ", ")
		}
		// Raw owner view erases class-scope formals to precisely the original
		// descriptor without adding a narrower argument check. The target stays
		// lexically private, so this invocation cannot dispatch to a subclass.
		call := "((" + owner + ")this)." + class_context.SafeIdentifier(key.name) + "(" + strings.Join(args, ", ") + ");"
		if ft.ReturnType.String(c.FuncCtx) != "void" {
			call = "return " + call
		}
		code := "final " + ft.ReturnType.String(c.FuncCtx) + " " + plan.bridges[key] + "(" + strings.Join(params, ", ") + ")" + clause + " { " + call + " }\n"
		if err := c.holdOutput(int64(len(code))); err != nil {
			return nil, err
		}
		helpers = append(helpers, &dumpedMethods{methodName: plan.bridges[key], code: code})
	}
	return helpers, nil
}
