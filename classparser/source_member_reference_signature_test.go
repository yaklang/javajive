package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialMemberReferenceSignatureCompletionHasNarrowAuthority(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	original, raw, artifact := constructorSuffixFamilyRoundTrip(t, memberReferenceSignatureFixture, "none", []string{"SuffixOwner", "SuffixOwner$Node"}, "18:owner:binding:identity:null\n")
	base, owner := artifact.source.jar.sourceConstructorLowering, "SuffixOwner"
	origin, known := base.referenceActive[owner]
	if !known {
		t.Fatal("missing returned source receipt")
	}
	for _, variant := range []string{"control", "idempotent", "opaque attribute", "wrong argument", "raw reference", "wrong class binder", "wrong flags", "missing method", "missing signature", "fresh type annotation", "original type annotation", "unreturned", "pending", "stale original", "budget", "output budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			a := &sourceConstructorLoweringAuthority{jar: base.jar, referenceActive: map[string]constructorNativeSourceOrigin{owner: origin}}
			obj, old := mustParseLowering(t, raw), bytes.Clone(original)
			var method *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "identity" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("missing fixture method")
			}
			var work *workbudget.Budget
			switch variant {
			case "opaque attribute", "fresh type annotation":
				name := "OpaqueReferenceData"
				data := []byte{2, 4, 6}
				if variant == "fresh type annotation" {
					name = "RuntimeVisibleTypeAnnotations"
					data = []byte{0, 0}
				}
				obj.ConstantPoolManager.AddUtf8Info(name)
				method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: name, Length: uint32(len(data)), Info: data})
			case "wrong argument", "raw reference":
				for _, attr := range method.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						s, _ := sourceBridgeUTF8(obj, sig.SignatureIndex)
						replacement := "<Ljava/lang/Number;>"
						if variant == "raw reference" {
							replacement = ""
						}
						s = strings.ReplaceAll(s, "<TT;>", replacement)
						sig.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(s))
					}
				}
			case "wrong class binder":
				for _, attr := range obj.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						sig.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("<T:Ljava/lang/Object;>Ljava/lang/Object;"))
					}
				}
			case "wrong flags":
				method.AccessFlags |= 8
			case "missing method":
				obj.Methods = obj.Methods[:1]
			case "missing signature":
				var attrs []AttributeInfo
				for _, attr := range method.Attributes {
					if _, ok := attr.(*SignatureAttribute); !ok {
						attrs = append(attrs, attr)
					}
				}
				method.Attributes = attrs
			case "original type annotation":
				o := mustParseLowering(t, old)
				o.ConstantPoolManager.AddUtf8Info("RuntimeInvisibleTypeAnnotations")
				for _, m := range o.Methods {
					n, _ := sourceBridgeUTF8(o, m.NameIndex)
					if n == "identity" {
						m.Attributes = append(m.Attributes, &UnparsedAttribute{Name: "RuntimeInvisibleTypeAnnotations", Length: 2, Info: []byte{0, 0}})
					}
				}
				old = o.Bytes()
				changed := origin
				changed.originalHash = sha256.Sum256(old)
				a.referenceActive[owner] = changed
			case "unreturned":
				a.referenceActive = nil
			case "pending":
				a.referencePending, a.referenceActive = a.referenceActive, nil
			case "stale original":
				o := mustParseLowering(t, old)
				o.ConstantPoolManager.AddUtf8Info("stale original")
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
				var err error
				input, err = a.restoreMemberReferenceSignatures(old, input, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			saved := bytes.Clone(input)
			out, err := a.restoreMemberReferenceSignatures(old, input, work)
			if !bytes.Equal(input, saved) {
				t.Fatal("input mutated")
			}
			if variant == "budget" || variant == "output budget" || variant == "canceled" {
				if err == nil || out != nil {
					t.Fatal("work boundary ignored")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if variant != "control" && variant != "idempotent" && variant != "opaque attribute" {
				if !bytes.Equal(out, input) {
					t.Fatal("unproved signature edit")
				}
				return
			}
			before, after := mustParseLowering(t, input), mustParseLowering(t, out)
			if len(after.ConstantPool) < len(before.ConstantPool) || !reflect.DeepEqual(before.ConstantPool, after.ConstantPool[:len(before.ConstantPool)]) || len(before.Methods) != len(after.Methods) {
				t.Fatal("existing constants or methods changed")
			}
			for i, m := range before.Methods {
				n := after.Methods[i]
				if m.NameIndex != n.NameIndex || m.DescriptorIndex != n.DescriptorIndex || m.AccessFlags != n.AccessFlags || len(m.Attributes) != len(n.Attributes) {
					t.Fatal("method declaration changed")
				}
				for j, attr := range m.Attributes {
					if _, signature := attr.(*SignatureAttribute); !signature && !reflect.DeepEqual(attr, n.Attributes[j]) {
						t.Fatal("fresh Code or opaque attribute changed")
					}
				}
			}
			if again, e := a.restoreMemberReferenceSignatures(old, out, nil); e != nil || !bytes.Equal(again, out) {
				t.Fatal("not idempotent", e)
			}
		})
	}
}
