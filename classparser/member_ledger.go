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
		if c.recordKeyword != "" && (c.recordSkipMethods[row.Name+row.Descriptor] || c.recordSkipMethods[row.Name]) {
			row.State = "regenerated"
			row.Evidence = "validated record canonical member reconstructed by record declaration"
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
