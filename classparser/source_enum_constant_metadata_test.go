package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnumConstantModifierCompletionNeedsPublishedForwarding(t *testing.T) {
	originals, fresh, artifact := enumPortTestInputs(t, "source,lines,vars")
	const name = "PortOwner$Mode$1.class"
	authority := artifact.source.jar.sourceConstructorLowering
	input, err := authority.lowerEnumConstructorPort(originals[name], fresh[name], nil)
	if err != nil {
		t.Fatal(err)
	}
	input, err = authority.restoreEnumLexicalOwner(originals[name], input, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"control", "idempotent", "same metadata modern target", "receiver", "invoke", "handler", "extra instruction", "class identity", "private self", "missing self", "duplicate self", "named self", "unreturned source", "work", "output", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(input))
			var ctor *MemberInfo
			var table *InnerClassesAttribute
			var self *InnerClassInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					ctor = m
				}
			}
			for _, attr := range obj.Attributes {
				if rows, ok := attr.(*InnerClassesAttribute); ok {
					table = rows
					for _, row := range rows.Classes {
						n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
						if n == obj.GetClassName() {
							self = row
						}
					}
				}
			}
			if ctor == nil || table == nil || self == nil {
				t.Fatal("incomplete original control")
			}
			code := constructorLoweringCode(ctor)
			owner := authority
			var budget *workbudget.Budget
			switch variant {
			case "receiver":
				code.Code[0] = core.OP_ALOAD_1
			case "invoke":
				code.Code[len(code.Code)-4] = core.OP_INVOKEVIRTUAL
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: uint16(len(code.Code) - 1)}}
			case "extra instruction":
				code.Code = append([]byte{core.OP_NOP}, code.Code...)
			case "class identity":
				obj.ThisClass = uint16(obj.ConstantPoolManager.AddNewClassInfo("OtherModifierOwner"))
			case "private self":
				self.InnerClassAccessFlags |= 2
			case "missing self":
				self.InnerClassInfoIndex = obj.SuperClass
			case "duplicate self":
				table.Classes = append(table.Classes, self)
			case "named self":
				self.InnerNameIndex = sourceBridgePoolString(t, obj, "Named")
			case "unreturned source":
				owner = &sourceConstructorLoweringAuthority{jar: authority.jar}
			case "work":
				budget = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "output":
				budget = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				budget = workbudget.New(ctx, workbudget.Limits{})
			}
			enumPortFixCodeLength(t, obj, code)
			raw := obj.Bytes()
			if variant == "idempotent" || variant == "same metadata modern target" {
				raw, err = owner.restoreEnumConstantModifiers(originals[name], raw, nil)
				if err != nil {
					t.Fatal(err)
				}
				if variant == "same metadata modern target" {
					obj = mustParseLowering(t, raw)
					obj.MajorVersion = 61
					raw = obj.Bytes()
				}
			}
			before := bytes.Clone(raw)
			out, err := owner.restoreEnumConstantModifiers(originals[name], raw, budget)
			if !bytes.Equal(before, raw) {
				t.Fatal("input was mutated")
			}
			switch variant {
			case "control":
				if err != nil || len(out) != len(raw) || bytes.Equal(out, raw) {
					t.Fatal("missing original modifiers", err)
				}
				different := 0
				for i := range raw {
					if raw[i] != out[i] {
						different++
					}
				}
				if different > 2 {
					t.Fatal("non-flag bytes changed")
				}
			case "idempotent", "same metadata modern target", "unreturned source":
				if err != nil || !bytes.Equal(out, raw) {
					t.Fatal("unexpected edit", err)
				}
			default:
				if err == nil || out != nil {
					t.Fatal("invalid forwarding packet acquired modifier rewrite")
				}
			}
		})
	}
}
