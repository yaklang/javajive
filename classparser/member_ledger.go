package javaclassparser

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"slices"
	"strings"
)

// MemberRecord accounts for every original declaration. State records source
// reconstruction, not a claim of behavioral equivalence or bytecode identity.
type MemberRecord struct {
	Owner      string `json:"owner"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Descriptor string `json:"descriptor"`
	State      string `json:"state"`
	Evidence   string `json:"evidence,omitempty"`
}
type ValidationObservation struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

var ErrSyntaxUnavailable = errors.New("syntax validator unavailable")

func newMemberLedger(obj *ClassObject) []MemberRecord {
	out := make([]MemberRecord, 0, len(obj.Fields)+len(obj.Methods))
	for _, group := range []struct {
		kind    string
		members []*MemberInfo
	}{{"field", obj.Fields}, {"method", obj.Methods}} {
		for _, m := range group.members {
			n, _ := obj.getUtf8(m.NameIndex)
			d, _ := obj.getUtf8(m.DescriptorIndex)
			out = append(out, MemberRecord{Owner: obj.GetClassName(), Kind: group.kind, Name: n, Descriptor: d, State: "pending"})
		}
	}
	return out
}
func finalizeMemberStatus(r *DecompileResult) {
	if r.Status != "complete" && r.Status != "partial" && r.Status != "unsupported" {
		return
	}
	partial, unknown := false, false
	for _, m := range r.Members {
		switch m.State {
		case "stub", "dropped":
			partial = true
		case "preserved", "regenerated":
			if m.Evidence == "" {
				unknown = true
			}
		default:
			unknown = true
		}
	}
	if partial {
		r.Status = "partial"
	} else if unknown {
		r.Status = "unsupported"
	}
}
func observeSyntax(r *DecompileResult, o DecompileOptions) {
	r.Syntax = ValidationObservation{Status: "unavailable"}
	if o.ValidateSyntax == nil {
		r.Syntax.Error = ErrSyntaxUnavailable.Error()
		return
	}
	ctx := o.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		r.Syntax.Status = "budget_exceeded"
		r.Syntax.Error = ctx.Err().Error()
		return
	}
	err := o.ValidateSyntax(ctx, r.Source)
	switch {
	case err == nil:
		r.Syntax.Status = "valid"
	case errors.Is(err, ErrSyntaxUnavailable):
		r.Syntax.Status = "unavailable"
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		r.Syntax.Status = "budget_exceeded"
	default:
		r.Syntax.Status = "invalid"
		if r.Status == "complete" {
			r.Status = "unsupported"
		}
	}
	if err != nil {
		r.Syntax.Error = err.Error()
	}
}

func (c *ClassObjectDumper) recordMembers(methods []*dumpedMethods, fields []dumpedFields) {
	if c.report == nil {
		return
	}
	for i := range c.report.Members {
		row := &c.report.Members[i]
		if row.Owner != c.obj.GetClassName() {
			continue
		}
		row.State = "unsupported"
		row.Evidence = "declaration not emitted; no regeneration proof"
		if row.Kind == "field" {
			for _, f := range fields {
				if f.fieldName == class_context.SafeIdentifier(row.Name) {
					row.State = "preserved"
					row.Evidence = "field emitted from declaration metadata"
					break
				}
			}
			if c.recordKeyword != "" && c.recordSkipFields[row.Name] {
				row.State = "regenerated"
				row.Evidence = "validated Record attribute component emitted in record header"
			}
			continue
		}
		if slices.Contains(c.report.StubMethods, row.Name+row.Descriptor) {
			row.State = "stub"
			row.Evidence = "decompiler emitted a throwing or empty initializer fallback"
			continue
		}
		for _, m := range methods {
			if m != nil && m.methodName == row.Name && m.descriptor == row.Descriptor {
				row.State = "preserved"
				row.Evidence = "method emitted from original declaration"
				break
			}
		}
		if row.State == "preserved" {
			continue
		}
		var member *MemberInfo
		for _, m := range c.obj.Methods {
			n, _ := c.obj.getUtf8(m.NameIndex)
			d, _ := c.obj.getUtf8(m.DescriptorIndex)
			if n == row.Name && d == row.Descriptor {
				member = m
				break
			}
		}
		if c.isRegenerableInheritedBridge(member, row.Name, row.Descriptor) {
			row.State = "regenerated"
			row.Evidence = "javac regenerates inherited bridge for a public interface method"
			continue
		}
		if c.recordKeyword != "" && (c.recordSkipMethods[row.Name+row.Descriptor] || c.recordSkipMethods[row.Name]) {
			row.State = "regenerated"
			row.Evidence = "validated record canonical member reconstructed by record declaration"
			continue
		}
		if member == nil {
			continue
		}
		// A consumed lambda needs synthetic metadata and a successfully rendered
		// cached body; a lambda$ prefix alone is not proof of consumption.
		key := fmt.Sprintf("name:%s,desc:%s", row.Name, row.Descriptor)
		cached := c.dumpedMethodsSet[key]
		if isSyntheticMethod(member.AccessFlags) && slices.Contains(c.lambdaMethods[row.Name], row.Descriptor) && cached != nil && cached.bodyCode != "stub" {
			row.State = "regenerated"
			row.Evidence = "synthetic lambda body consumed at bootstrap call site"
			continue
		}
		access, _ := getMethodAccessFlagsVerbose(member.AccessFlags)
		if row.Name == "<init>" && cached != nil && strings.TrimSpace(cached.bodyCode) == "" && c.isOmittableDefaultCtor(row.Descriptor, access) && c.isImplicitSuperBody(member) {
			row.State = "regenerated"
			row.Evidence = "sole no-arg constructor with matching class visibility and implicit super body"
			continue
		}
		if row.Name == "<clinit>" && cached != nil && strings.TrimSpace(cached.bodyCode) == "" {
			row.State = "regenerated"
			row.Evidence = "initializer fully consumed into emitted field initializers or empty return"
			continue
		}
	}
}

// isRegenerableInheritedBridge proves the narrow javac bridge shape produced
// when a public class inherits a public interface implementation through a
// package-private superclass. The bridge is absent from source declarations,
// but javac recreates it when those source classes are compiled together.
// Similar-looking bridges with casts, argument adaptation, a different owner,
// or an unresolved interface stay unsupported.
func (c *ClassObjectDumper) isRegenerableInheritedBridge(member *MemberInfo, name, descriptor string) bool {
	if c == nil || c.obj == nil || c.obj.AccessFlags&0x0001 == 0 || member == nil || !isBridgeMethod(member.AccessFlags) || !isSyntheticMethod(member.AccessFlags) {
		return false
	}
	parentName := c.obj.GetSupperClassName()
	if parentName == "" {
		return false
	}
	forwardOwner, forwardName, forwardDescriptor, ok := c.bridgeForwardTarget(member)
	if !ok || forwardOwner != parentName || forwardName != name || forwardDescriptor != descriptor {
		return false
	}
	parent, ok := c.resolveBridgeClass(parentName)
	if !ok || parent.AccessFlags&0x0001 != 0 {
		return false
	}
	if !hasInheritedConcretePublicMethod(parent, name, descriptor) {
		return false
	}
	for _, interfaceName := range parent.GetInterfacesName() {
		iface, ok := c.resolveBridgeClass(interfaceName)
		if !ok || iface.AccessFlags&0x0200 == 0 || iface.AccessFlags&0x0001 == 0 {
			continue
		}
		for _, interfaceMethod := range iface.Methods {
			methodName, errName := iface.getUtf8(interfaceMethod.NameIndex)
			methodDescriptor, errDescriptor := iface.getUtf8(interfaceMethod.DescriptorIndex)
			if errName == nil && errDescriptor == nil && methodName == name && methodDescriptor == descriptor &&
				interfaceMethod.AccessFlags&0x0001 != 0 && interfaceMethod.AccessFlags&0x0008 == 0 {
				return true
			}
		}
	}
	return false
}

func (c *ClassObjectDumper) resolveBridgeClass(name string) (*ClassObject, bool) {
	if c == nil || c.foldSiblingResolver == nil {
		return nil, false
	}
	data, ok := c.foldSiblingResolver(name)
	if !ok {
		return nil, false
	}
	obj, err := c.parseResolved(data)
	if err != nil || obj == nil || obj.GetClassName() != name {
		return nil, false
	}
	return obj, true
}

func hasInheritedConcretePublicMethod(obj *ClassObject, name, descriptor string) bool {
	if obj == nil {
		return false
	}
	for _, method := range obj.Methods {
		methodName, errName := obj.getUtf8(method.NameIndex)
		methodDescriptor, errDescriptor := obj.getUtf8(method.DescriptorIndex)
		if errName == nil && errDescriptor == nil && methodName == name && methodDescriptor == descriptor &&
			method.AccessFlags&0x0001 != 0 && method.AccessFlags&0x0008 == 0 && method.AccessFlags&0x0400 == 0 {
			return true
		}
	}
	return false
}

func (c *ClassObjectDumper) bridgeForwardTarget(member *MemberInfo) (owner, name, descriptor string, ok bool) {
	for _, attribute := range member.Attributes {
		code, isCode := attribute.(*CodeAttribute)
		if !isCode {
			continue
		}
		bytes := code.Code
		if len(code.ExceptionTable) != 0 || len(bytes) != 5 || bytes[0] != 0x2a || bytes[1] != 0xb7 {
			return "", "", "", false
		}
		constant, err := c.obj.getConstantInfo(binary.BigEndian.Uint16(bytes[2:4]))
		if err != nil {
			return "", "", "", false
		}
		methodRef, isMethodRef := constant.(*ConstantMethodrefInfo)
		if !isMethodRef {
			return "", "", "", false
		}
		owner, err = c.obj.getUtf8(methodRef.ClassIndex)
		if err != nil {
			return "", "", "", false
		}
		nameAndType, err := c.obj.getConstantInfo(methodRef.NameAndTypeIndex)
		if err != nil {
			return "", "", "", false
		}
		pair, isNameAndType := nameAndType.(*ConstantNameAndTypeInfo)
		if !isNameAndType {
			return "", "", "", false
		}
		name, err = c.obj.getUtf8(pair.NameIndex)
		if err != nil {
			return "", "", "", false
		}
		descriptor, err = c.obj.getUtf8(pair.DescriptorIndex)
		if err != nil {
			return "", "", "", false
		}
		return owner, name, descriptor, bytes[4] == bridgeReturnOpcode(descriptor)
	}
	return "", "", "", false
}

func bridgeReturnOpcode(descriptor string) byte {
	close := strings.LastIndexByte(descriptor, ')')
	if close < 0 || close+1 >= len(descriptor) {
		return 0
	}
	switch descriptor[close+1] {
	case 'V':
		return 0xb1 // return
	case 'J':
		return 0xad // lreturn
	case 'F':
		return 0xae // freturn
	case 'D':
		return 0xaf // dreturn
	case 'L', '[':
		return 0xb0 // areturn
	case 'B', 'C', 'I', 'S', 'Z':
		return 0xac // ireturn
	default:
		return 0
	}
}

// A dropped default constructor is regenerated only for the exact implicit
// super() bytecode shape. An accidentally emptied body is not proof.
func (c *ClassObjectDumper) isImplicitSuperBody(m *MemberInfo) bool {
	for _, a := range m.Attributes {
		code, ok := a.(*CodeAttribute)
		if !ok {
			continue
		}
		b := code.Code
		if len(code.ExceptionTable) != 0 || len(b) != 5 || b[0] != 0x2a || b[1] != 0xb7 || b[4] != 0xb1 {
			return false
		}
		cp, e := c.obj.getConstantInfo(binary.BigEndian.Uint16(b[2:4]))
		if e != nil {
			return false
		}
		ref, ok := cp.(*ConstantMethodrefInfo)
		if !ok {
			return false
		}
		owner, e := c.obj.getUtf8(ref.ClassIndex)
		if e != nil || owner != c.obj.GetSupperClassName() {
			return false
		}
		nt, e := c.obj.getConstantInfo(ref.NameAndTypeIndex)
		if e != nil {
			return false
		}
		pair, ok := nt.(*ConstantNameAndTypeInfo)
		if !ok {
			return false
		}
		name, e1 := c.obj.getUtf8(pair.NameIndex)
		desc, e2 := c.obj.getUtf8(pair.DescriptorIndex)
		return e1 == nil && e2 == nil && name == "<init>" && desc == "()V"
	}
	return false
}
