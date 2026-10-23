package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorBindingSignatureCompletionPreservesFreshCodeAndRejectsDrift(t *testing.T) {
	original, fresh, artifact := constructorSuffixFamilyRoundTrip(t, constructorEnclosingGenericFixture("Runnable"), "source,lines,vars", []string{"SuffixOwner$Base", "SuffixOwner$Member"}, "6:generic:enclosing:capture:identity:failure\n")
	authority := artifact.source.jar.sourceConstructorLowering
	owner := "SuffixOwner$Base"
	origin, known := authority.bindingActive[owner]
	if !known {
		t.Fatal("missing real returned source")
	}
	for _, variant := range []string{"control", "already finished", "opaque attribute", "unreturned source", "pending only", "stale original", "wrong class formal", "wrong signature", "missing signature", "duplicate signature", "wrong descriptor", "changed flags", "duplicate constructor", "wrong capture", "budget", "canceled", "output budget"} {
		t.Run(variant, func(t *testing.T) {
			a := &sourceConstructorLoweringAuthority{jar: authority.jar, bindingActive: map[string]constructorNativeSourceOrigin{owner: origin}}
			input := bytes.Clone(fresh)
			old := bytes.Clone(original)
			obj := mustParseLowering(t, input)
			pool := NewConstantPoolWithConstant(&obj.ConstantPool)
			var ctor *MemberInfo
			var sig *SignatureAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					ctor = m
					for _, attr := range m.Attributes {
						if s, ok := attr.(*SignatureAttribute); ok {
							sig = s
						}
					}
				}
			}
			if ctor == nil || sig == nil {
				t.Fatal("fresh constructor signature")
			}
			var work *workbudget.Budget
			switch variant {
			case "already finished":
				var err error
				input, err = a.restoreConstructorBindingSignatures(old, input, nil)
				if err != nil {
					t.Fatal(err)
				}
			case "opaque attribute":
				pool.AddUtf8Info("UnrelatedProofData")
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "UnrelatedProofData", Length: 3, Info: []byte{9, 2, 7}})
			case "unreturned source":
				a.bindingActive = nil
			case "pending only":
				a.bindingPending = a.bindingActive
				a.bindingActive = nil
			case "stale original":
				old = append(old, 0)
			case "wrong class formal":
				for _, attr := range obj.Attributes {
					if s, ok := attr.(*SignatureAttribute); ok {
						s.SignatureIndex = uint16(pool.AddUtf8Info("<T:Ljava/lang/Object;>Ljava/lang/Object;"))
					}
				}
			case "wrong signature":
				sig.SignatureIndex = uint16(pool.AddUtf8Info("(LSuffixOwner;Ljava/lang/Runnable;)V"))
			case "missing signature":
				attrs := ctor.Attributes[:0]
				for _, attr := range ctor.Attributes {
					if _, ok := attr.(*SignatureAttribute); !ok {
						attrs = append(attrs, attr)
					}
				}
				ctor.Attributes = attrs
			case "duplicate signature":
				ctor.Attributes = append(ctor.Attributes, sig)
			case "wrong descriptor":
				ctor.DescriptorIndex = uint16(pool.AddUtf8Info("(LSuffixOwner;Ljava/lang/Object;)V"))
			case "changed flags":
				ctor.AccessFlags |= 1
			case "duplicate constructor":
				obj.Methods = append(obj.Methods, ctor)
			case "wrong capture":
				code := constructorLoweringCode(ctor)
				found := false
				for i := 0; i+2 < len(code.Code); i++ {
					if code.Code[i] == 0x2a && code.Code[i+1] == 0x2b && code.Code[i+2] == 0xb5 {
						code.Code[i+1] = 0x2c
						found = true
						break
					}
				}
				if !found {
					t.Fatal("fresh capture fixture")
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "output budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			if variant != "already finished" {
				input = obj.Bytes()
			}
			saved := bytes.Clone(input)
			out, err := a.restoreConstructorBindingSignatures(old, input, work)
			if !bytes.Equal(input, saved) {
				t.Fatal("mutated caller bytes")
			}
			good := variant == "control" || variant == "already finished" || variant == "opaque attribute"
			decline := variant == "unreturned source" || variant == "pending only" || variant == "stale original"
			if decline {
				if err == nil && !bytes.Equal(out, input) {
					t.Fatal("unpublished input gained edit authority")
				}
				if err == nil && sourceMethodMetadataClosed(mustParseLowering(t, original), out, nil) == nil {
					t.Fatal("unproved signature accepted")
				}
				return
			}
			if !good {
				if err == nil || out != nil {
					t.Fatal("unproved signature edit accepted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			parseLayout := func(data []byte) (*ClassObject, *compiledMetadataLayout) {
				t.Helper()
				reader, e := NewClassReaderWithBudget(data, nil)
				if e != nil {
					t.Fatal(e)
				}
				layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
				obj, e := parseWithMetadataLayout(reader, nil, layout)
				if e != nil {
					t.Fatal(e)
				}
				return obj, layout
			}
			before, bl := parseLayout(input)
			after, al := parseLayout(out)
			if err := sourceMethodMetadataClosed(mustParseLowering(t, original), out, nil); err != nil {
				t.Fatal(err)
			}
			if len(before.ConstantPool) > len(after.ConstantPool) || len(before.Methods) != len(after.Methods) {
				t.Fatal("changed fresh universe")
			}
			for i, span := range bl.constants {
				other := al.constants[i]
				if !bytes.Equal(input[span.start:span.end], out[other.start:other.end]) {
					t.Fatal("shared constant mutated", i)
				}
			}
			for i, m := range before.Methods {
				if constructorLoweringCode(m) == nil {
					continue
				}
				bc, e := enumArrayCodePayload(input, before, bl.methods[m], nil)
				if e != nil {
					t.Fatal(e)
				}
				ac, e := enumArrayCodePayload(out, after, al.methods[after.Methods[i]], nil)
				if e != nil || !bytes.Equal(bc.data, ac.data) {
					t.Fatal("fresh Code changed", i, e)
				}
			}
			if variant == "opaque attribute" {
				found := false
				for _, a := range after.Attributes {
					if u, ok := a.(*UnparsedAttribute); ok && u.Name == "UnrelatedProofData" {
						found = bytes.Equal(u.Info, []byte{9, 2, 7})
					}
				}
				if !found {
					t.Fatal("opaque metadata lost")
				}
			}
			again, err := a.restoreConstructorBindingSignatures(old, out, nil)
			if err != nil || !bytes.Equal(again, out) {
				t.Fatal("not idempotent", err)
			}
		})
	}
}
