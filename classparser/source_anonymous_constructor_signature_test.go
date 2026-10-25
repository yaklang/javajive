package javaclassparser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialAnonymousConstructorSignatureRequiresExactCaptureProof(t *testing.T) {
	source := strings.ReplaceAll(strings.ReplaceAll(anonymousCapturedConstructorFixture, "PHYSICAL", "2"), "GENERIC", "0")
	t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
	old, fresh, artifact := constructorSuffixFamilyWithOriginalCompilerRoundTrip(t, source, "none", []string{"SuffixOwner$1"}, "15:anonymous:constructor:physical:declared\n", 8, 8, ModernJavac, ModernJavac)
	authority := artifact.source.jar.sourceConstructorLowering
	origin, exists := authority.bindingActive["SuffixOwner$1"]
	if !exists {
		t.Fatal("real returned source receipt missing")
	}
	fresh, err := restoreReconstructionSyntheticFlags(old, fresh, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"control", "idempotent", "opaque attribute", "unreturned", "pending only", "stale original", "wrong owner", "wrong superclass", "changed version", "missing field", "field descriptor", "field flags", "wrong slot", "duplicate constructor", "missing constructor", "constructor flags", "nonempty signature", "duplicate signature", "added method", "budget", "canceled", "output budget"} {
		t.Run(variant, func(t *testing.T) {
			a := &sourceConstructorLoweringAuthority{jar: authority.jar, bindingActive: map[string]constructorNativeSourceOrigin{"SuffixOwner$1": origin}}
			original := bytes.Clone(old)
			input := bytes.Clone(fresh)
			obj := mustParseLowering(t, input)
			var ctor *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					ctor = m
				}
			}
			if ctor == nil {
				t.Fatal("fixture constructor")
			}
			pool := NewConstantPoolWithConstant(&obj.ConstantPool)
			var work *workbudget.Budget
			switch variant {
			case "idempotent":
				input, err = a.restoreAnonymousCapturedConstructorSignature(original, input, nil)
				if err != nil {
					t.Fatal(err)
				}
			case "opaque attribute":
				pool.AddUtf8Info("OpaqueTestEvidence")
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "OpaqueTestEvidence", Length: 3, Info: []byte{9, 7, 1}})
			case "unreturned":
				a.bindingActive = nil
			case "pending only":
				a.bindingPending = a.bindingActive
				a.bindingActive = nil
			case "stale original":
				original = append(original, 0)
			case "wrong owner":
				obj.ThisClass = obj.SuperClass
			case "wrong superclass":
				obj.SuperClass = obj.ThisClass
			case "changed version":
				obj.MajorVersion = 53
			case "missing field":
				obj.Fields = obj.Fields[1:]
			case "field descriptor":
				obj.Fields[0].DescriptorIndex = uint16(pool.AddUtf8Info("D"))
			case "field flags":
				obj.Fields[0].AccessFlags ^= 0x10
			case "wrong slot":
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
					t.Fatal("typed reference capture absent")
				}
			case "duplicate constructor":
				obj.Methods = append(obj.Methods, ctor)
			case "missing constructor":
				kept := []*MemberInfo{}
				for _, m := range obj.Methods {
					if m != ctor {
						kept = append(kept, m)
					}
				}
				obj.Methods = kept
			case "constructor flags":
				ctor.AccessFlags ^= 0x01
			case "nonempty signature", "duplicate signature":
				text := "(Ljava/lang/Object;J)V"
				if variant == "duplicate signature" {
					text = "()V"
				}
				pool.AddUtf8Info("Signature")
				s := &SignatureAttribute{SignatureIndex: uint16(pool.AddUtf8Info(text))}
				ctor.Attributes = append(ctor.Attributes, s)
				if variant == "duplicate signature" {
					ctor.Attributes = append(ctor.Attributes, s)
				}
			case "added method":
				obj.Methods = append(obj.Methods, obj.Methods[len(obj.Methods)-1])
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "output budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			if variant != "idempotent" {
				input = obj.Bytes()
			}
			saved := bytes.Clone(input)
			savedOriginal := bytes.Clone(original)
			out, e := a.restoreAnonymousCapturedConstructorSignature(original, input, work)
			if !bytes.Equal(saved, input) || !bytes.Equal(savedOriginal, original) {
				t.Fatal("input mutation")
			}
			good := variant == "control" || variant == "idempotent" || variant == "opaque attribute"
			decline := variant == "unreturned" || variant == "pending only" || variant == "stale original"
			if decline {
				if e == nil && !bytes.Equal(out, input) {
					t.Fatal("unpublished signature edit")
				}
				return
			}
			if !good {
				if e == nil || out != nil {
					t.Fatal("unproved projection accepted", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			before, after := mustParseLowering(t, input), mustParseLowering(t, out)
			if len(before.Methods) != len(after.Methods) {
				t.Fatal("method universe changed")
			}
			for i, m := range before.Methods {
				b, n := constructorLoweringCode(m), constructorLoweringCode(after.Methods[i])
				if b == nil || n == nil || !bytes.Equal(b.Code, n.Code) {
					t.Fatal("fresh executable code changed")
				}
			}
			if again, e := a.restoreAnonymousCapturedConstructorSignature(original, out, nil); e != nil || !bytes.Equal(again, out) {
				t.Fatal("non-idempotent completion", e)
			}
			if e := sourceMethodMetadataClosed(mustParseLowering(t, old), out, nil); e != nil {
				t.Fatal("declared constructor contract", e)
			}
		})
	}
}
