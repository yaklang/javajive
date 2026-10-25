package javaclassparser

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestAdversarialConstructorMemberReferenceCompletionRequiresBothProofs(t *testing.T) {
	old, raw, artifact := constructorSuffixFamilyRoundTrip(t, constructorMemberReferenceFixture, "source,lines,vars", []string{"SuffixOwner$Holder", "SuffixOwner$Item"}, "10:constructor:owner:binding:identity\n")
	base, owner := artifact.source.jar.sourceConstructorLowering, "SuffixOwner$Holder"
	for _, variant := range []string{"control", "unreturned reference", "pending reference", "unreturned scope", "wrong argument", "wrong hidden port", "fresh type annotation", "field only drift", "field static drift", "field extra synthetic", "field type annotation", "field missing signature", "field raw reference", "field unknown binder"} {
		t.Run(variant, func(t *testing.T) {
			a := &sourceConstructorLoweringAuthority{jar: base.jar,
				bindingActive:   map[string]constructorNativeSourceOrigin{owner: base.bindingActive[owner]},
				referenceActive: map[string]constructorNativeSourceOrigin{owner: base.referenceActive[owner]},
				genericActive:   map[string]constructorNativeSourceOrigin{owner: base.genericActive[owner]}}
			obj := mustParseLowering(t, raw)
			var constructor, field *MemberInfo
			for _, m := range obj.Methods {
				if name, _ := sourceBridgeUTF8(obj, m.NameIndex); name == "<init>" {
					constructor = m
				}
			}
			for _, f := range obj.Fields {
				if name, _ := sourceBridgeUTF8(obj, f.NameIndex); name == "item" {
					field = f
				}
			}
			if constructor == nil || field == nil {
				t.Fatal("fixture declarations")
			}
			switch variant {
			case "unreturned reference":
				a.referenceActive = nil
			case "pending reference":
				a.referencePending, a.referenceActive = a.referenceActive, nil
			case "unreturned scope":
				a.genericActive = nil
			case "field static drift":
				field.AccessFlags |= StaticFlag
			case "field extra synthetic":
				field.AccessFlags |= syntheticFieldFlag
			case "field type annotation":
				obj.ConstantPoolManager.AddUtf8Info("RuntimeVisibleTypeAnnotations")
				field.Attributes = append(field.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleTypeAnnotations", Length: 2, Info: []byte{0, 0}})
			case "fresh type annotation":
				obj.ConstantPoolManager.AddUtf8Info("RuntimeVisibleTypeAnnotations")
				constructor.Attributes = append(constructor.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleTypeAnnotations", Length: 2, Info: []byte{0, 0}})
			case "field missing signature":
				var attrs []AttributeInfo
				for _, attr := range field.Attributes {
					if _, signature := attr.(*SignatureAttribute); !signature {
						attrs = append(attrs, attr)
					}
				}
				field.Attributes = attrs
			case "wrong argument", "wrong hidden port", "field only drift", "field raw reference", "field unknown binder":
				member := constructor
				if strings.HasPrefix(variant, "field ") {
					member = field
				}
				for _, attr := range member.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						s, _ := sourceBridgeUTF8(obj, sig.SignatureIndex)
						changed := strings.ReplaceAll(s, "<TT;>", "<Ljava/lang/Number;>")
						if variant == "wrong hidden port" {
							changed = strings.Replace(s, "(LSuffixOwner;", "(LSuffixOwner<TT;>;", 1)
						}
						if variant == "field raw reference" {
							changed = strings.ReplaceAll(s, "<TT;>", "")
						}
						if variant == "field unknown binder" {
							changed = strings.ReplaceAll(s, "<TT;>", "<TMissing;>")
						}
						if changed == s {
							t.Fatal("mutation did not change signature")
						}
						sig.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(changed))
					}
				}
			}
			input := obj.Bytes()
			saved := bytes.Clone(input)
			out, err := a.restoreConstructorBindingSignatures(old, input, nil)
			if !bytes.Equal(saved, input) {
				t.Fatal("mutated input")
			}
			if variant != "control" && !strings.HasPrefix(variant, "field ") {
				if err == nil || out != nil {
					t.Fatal("unproved constructor projection accepted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			out, err = a.restoreMemberReferenceSignatures(old, out, nil)
			if strings.HasPrefix(variant, "field ") {
				if finished, finishErr := artifact.FinishClass(owner+".class", input); finishErr == nil || finished != nil {
					t.Fatal("unproved field projection escaped the complete artifact", variant, finishErr)
				}
				if err == nil || out != nil {
					t.Fatal("unproved field projection accepted", variant, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			before, after := mustParseLowering(t, input), mustParseLowering(t, out)
			if len(before.Methods) != len(after.Methods) || len(before.Fields) != len(after.Fields) || len(after.ConstantPool) < len(before.ConstantPool) || !reflect.DeepEqual(before.ConstantPool, after.ConstantPool[:len(before.ConstantPool)]) {
				t.Fatal("changed fresh inventory or constant prefix")
			}
			for group, members := range [][]*MemberInfo{before.Methods, before.Fields} {
				next := after.Methods
				if group == 1 {
					next = after.Fields
				}
				for i, m := range members {
					n := next[i]
					if m.NameIndex != n.NameIndex || m.DescriptorIndex != n.DescriptorIndex || m.AccessFlags != n.AccessFlags || len(m.Attributes) != len(n.Attributes) {
						t.Fatal("changed physical declaration")
					}
					for j, attr := range m.Attributes {
						if _, signature := attr.(*SignatureAttribute); !signature && !reflect.DeepEqual(attr, n.Attributes[j]) {
							t.Fatal("changed fresh Code or opaque metadata")
						}
					}
				}
			}
			want := mustParseLowering(t, old)
			for i, f := range after.Fields {
				name, _ := sourceBridgeUTF8(after, f.NameIndex)
				if name != "item" {
					continue
				}
				actual, _, valid := sourceMethodSignatureKey(after, f)
				expected, _, known := sourceMethodSignatureKey(want, want.Fields[i])
				if !valid || !known || (actual == expected) != (variant == "control") {
					t.Fatal("field projection authority", variant)
				}
			}
			if again, e := a.restoreConstructorBindingSignatures(old, out, nil); e != nil || !bytes.Equal(again, out) {
				t.Fatal("constructor completion not idempotent", e)
			}
			if again, e := a.restoreMemberReferenceSignatures(old, out, nil); e != nil || !bytes.Equal(again, out) {
				t.Fatal("reference completion not idempotent", e)
			}
		})
	}
}
