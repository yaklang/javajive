package javaclassparser

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialHiddenMemberSignatureRequiresExactFreshProjection(t *testing.T) {
	original, raw, artifact := constructorSuffixFamilyRoundTrip(t, ownAndOuterBindingFixture(), "none", []string{"SuffixOwner$Member"}, "12:own:outer:shadow:fields:identity\n")
	base, owner := artifact.source.jar.sourceConstructorLowering, "SuffixOwner$Member"
	fresh, err := restoreMethodParameterMetadata(original, raw, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	origin, known := base.genericActive[owner]
	if !known {
		t.Fatal("missing source receipt")
	}
	for _, variant := range []string{"control", "idempotent", "opaque attribute", "changed shadow binder", "changed outer projection", "changed own field", "added field signature", "changed own bound", "missing method", "duplicate method", "changed field name", "type annotations", "unreturned", "pending", "stale original", "budget", "canceled", "output budget"} {
		t.Run(variant, func(t *testing.T) {
			a := &sourceConstructorLoweringAuthority{jar: base.jar, genericActive: map[string]constructorNativeSourceOrigin{owner: origin}}
			obj := mustParseLowering(t, bytes.Clone(fresh))
			old := bytes.Clone(original)
			member := func(name string, fields bool) *MemberInfo {
				list := obj.Methods
				if fields {
					list = obj.Fields
				}
				for _, m := range list {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if n == name {
						return m
					}
				}
				t.Fatal("missing fixture member", name)
				return nil
			}
			setSignature := func(m *MemberInfo, s string) {
				index := uint16(obj.ConstantPoolManager.AddUtf8Info(s))
				for _, attr := range m.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						sig.SignatureIndex = index
						return
					}
				}
				obj.ConstantPoolManager.AddUtf8Info("Signature")
				m.Attributes = append(m.Attributes, &SignatureAttribute{SignatureIndex: index})
			}
			var work *workbudget.Budget
			switch variant {
			case "opaque attribute":
				obj.ConstantPoolManager.AddUtf8Info("OpaqueProjectionData")
				m := member("outer", false)
				m.Attributes = append(m.Attributes, &UnparsedAttribute{Name: "OpaqueProjectionData", Length: 3, Info: []byte{1, 3, 7}})
			case "changed shadow binder":
				setSignature(member("shadow", false), "<T:Ljava/lang/CharSequence;>(TU;)TU;")
			case "changed outer projection":
				setSignature(member("outer", false), "(Ljava/lang/Integer;)Ljava/lang/Integer;")
			case "changed own field":
				setSignature(member("selected", true), "Ljava/lang/CharSequence;")
			case "added field signature":
				setSignature(member("this$0", true), "Ljava/lang/Object;")
			case "changed own bound":
				for _, attr := range obj.Attributes {
					if s, ok := attr.(*SignatureAttribute); ok {
						s.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("<U:Ljava/lang/Object;>Ljava/lang/Object;"))
					}
				}
			case "missing method":
				obj.Methods = obj.Methods[1:]
			case "duplicate method":
				obj.Methods = append(obj.Methods, member("own", false))
			case "changed field name":
				member("saved", true).NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("unrelated"))
			case "type annotations":
				obj.ConstantPoolManager.AddUtf8Info("RuntimeVisibleTypeAnnotations")
				m := member("outer", false)
				m.Attributes = append(m.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleTypeAnnotations", Length: 2, Info: []byte{0, 0}})
			case "unreturned":
				a.genericActive = nil
			case "pending":
				a.genericPending, a.genericActive = a.genericActive, nil
			case "stale original":
				o := mustParseLowering(t, old)
				o.ConstantPoolManager.AddUtf8Info("changed original")
				old = o.Bytes()
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "output budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := obj.Bytes()
			if variant == "idempotent" {
				input, err = a.restoreMemberGenericScope(old, input, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			saved := bytes.Clone(input)
			out, err := a.restoreMemberGenericScope(old, input, work)
			if !bytes.Equal(saved, input) {
				t.Fatal("caller bytes changed")
			}
			if variant == "unreturned" || variant == "pending" || variant == "stale original" {
				if err != nil || !bytes.Equal(out, input) {
					t.Fatal("unpublished edit", err)
				}
				return
			}
			if variant != "control" && variant != "idempotent" && variant != "opaque attribute" {
				if err == nil || out != nil {
					t.Fatal("unproved projection", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			before, after := mustParseLowering(t, input), mustParseLowering(t, out)
			for i, m := range before.Methods {
				if !reflect.DeepEqual(constructorLoweringCode(m), constructorLoweringCode(after.Methods[i])) {
					t.Fatal("fresh executable changed")
				}
			}
			if !reflect.DeepEqual(before.ConstantPool, after.ConstantPool[:len(before.ConstantPool)]) {
				t.Fatal("existing constant changed")
			}
			if again, e := a.restoreMemberGenericScope(old, out, nil); e != nil || !bytes.Equal(again, out) {
				t.Fatal("non-idempotent projection", e)
			}
		})
	}
}
