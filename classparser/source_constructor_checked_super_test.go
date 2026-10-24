package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialSourceConstructorCheckedSuperRequiresCompleteOriginalContract(t *testing.T) {
	original, _, artifact := constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, constructorCheckedSuperFixture, "none", []string{"SuffixOwner$1"}, "24:checked:super:identity:early:capture:metadata\n", 4, 8, ModernJavac)
	reader := artifact.source.jar.nativeMemberReader(mustParseLowering(t, bytes.Clone(original)))
	metadata := reader.buildInvocationMetadata()
	for _, variant := range []string{"control", "foreign member", "missing member", "declared Exceptions", "Signature", "changed capture", "suffix", "missing parent", "wrong parent", "incomplete parent", "generic parent", "duplicate target", "generic target", "unknown exceptions", "duplicate exception", "unknown exception", "unchecked only", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(original))
			var method *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					method = m
				}
			}
			packet := constructorLoweringTypedPacketFor(obj, method, true, true, nil, metadata)
			if packet == nil {
				t.Fatal("independent original packet")
			}
			provider := callbinding.Provider(func(name string) (callbinding.Class, bool) {
				c, known := metadata(name)
				if name != "SuffixBase" {
					return c, known
				}
				c.Methods = append([]callbinding.Method(nil), c.Methods...)
				switch variant {
				case "missing parent":
					return callbinding.Class{}, false
				case "wrong parent":
					c.Name = "AnotherParent"
				case "incomplete parent":
					c.MembersComplete = false
				case "generic parent":
					c.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
				}
				for i := range c.Methods {
					m := &c.Methods[i]
					if m.Name != "<init>" {
						continue
					}
					switch variant {
					case "duplicate target":
						c.Methods = append(c.Methods, *m)
					case "generic target":
						m.Generic = true
					case "unknown exceptions":
						m.ExceptionsKnown = false
					case "duplicate exception":
						m.Exceptions = []string{"java/io/IOException", "java/io/IOException"}
					case "unknown exception":
						m.Exceptions = []string{"UnresolvedFailure"}
					case "unchecked only":
						m.Exceptions = []string{"java/lang/IllegalArgumentException"}
					}
				}
				return c, known
			})
			var work *workbudget.Budget
			switch variant {
			case "foreign member":
				copy := *method
				method = &copy
			case "missing member":
				method = nil
			case "declared Exceptions":
				obj.ConstantPoolManager.AddUtf8Info("Exceptions")
				method.Attributes = append(method.Attributes, &ExceptionsAttribute{AttrLen: 2})
			case "Signature":
				obj.ConstantPoolManager.AddUtf8Info("Signature")
				method.Attributes = append(method.Attributes, &SignatureAttribute{AttrLen: 2, SignatureIndex: uint16(obj.ConstantPoolManager.AddUtf8Info(packet.plan.descriptor))})
			case "changed capture":
				constructorLoweringCode(method).Code[1] = 0x2c
			case "suffix":
				packet.plan.body = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got, valid := constructorLoweringCheckedSuperExceptions(obj, method, packet.plan, provider, work)
			if valid != (variant == "control") || valid && got != "java/io/IOException" {
				t.Fatalf("projection %q %v", got, valid)
			}
			if work != nil && work.Check() == nil {
				t.Fatal("resource refusal lost")
			}
		})
	}
}

func TestAdversarialSourceConstructorCheckedSuperFinishRejectsUnownedMetadata(t *testing.T) {
	original, compiled, artifact := constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, constructorCheckedSuperFixture, "none", []string{"SuffixOwner$1"}, "24:checked:super:identity:early:capture:metadata\n", 4, 8, ModernJavac)
	base := artifact.source.jar.sourceConstructorLowering
	publication := base.active["SuffixOwner$1"]
	if publication == nil || len(publication.plans) != 1 {
		t.Fatal("returned source publication")
	}
	var descriptor string
	for desc := range publication.plans {
		descriptor = desc
	}
	for _, variant := range []string{"control", "opaque attribute", "already finished", "unreturned source", "pending only", "stale original", "changed plan", "missing plan", "wrong throws", "missing throws", "empty throws", "duplicate throws", "Signature", "changed code", "changed flags", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			p := *publication
			p.plans = map[string]*sourceConstructorLoweringPlan{}
			for desc, plan := range publication.plans {
				copy := *plan
				p.plans[desc] = &copy
			}
			a := &sourceConstructorLoweringAuthority{jar: base.jar, active: map[string]*sourceConstructorLoweringPublication{"SuffixOwner$1": &p}}
			obj := mustParseLowering(t, bytes.Clone(compiled))
			method, known := sourceAnonymousConstructorMethod(obj, descriptor, nil)
			if !known {
				t.Fatal("fresh constructor")
			}
			var table *ExceptionsAttribute
			for _, attr := range method.Attributes {
				if e, ok := attr.(*ExceptionsAttribute); ok {
					table = e
				}
			}
			if table == nil {
				t.Fatal("fresh compiler must produce the temporary source declaration")
			}
			old := bytes.Clone(original)
			var work *workbudget.Budget
			switch variant {
			case "opaque attribute":
				obj.ConstantPoolManager.AddUtf8Info("IndependentWitness")
				method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: "IndependentWitness", Length: 3, Info: []byte{9, 2, 7}})
			case "unreturned source":
				a.active = nil
			case "pending only":
				a.pending, a.active = a.active, nil
			case "stale original":
				changed := mustParseLowering(t, old)
				changed.ConstantPoolManager.AddUtf8Info("changed identity")
				old = changed.Bytes()
			case "changed plan":
				for _, plan := range p.plans {
					plan.checkedSuperExceptions = "java/sql/SQLException"
				}
			case "missing plan":
				for _, plan := range p.plans {
					plan.checkedSuperExceptions = ""
				}
			case "wrong throws":
				table.ExceptionIndexTable[0] = uint16(obj.ConstantPoolManager.AddNewClassInfo("java/sql/SQLException"))
			case "missing throws":
				var kept []AttributeInfo
				for _, attr := range method.Attributes {
					if attr != table {
						kept = append(kept, attr)
					}
				}
				method.Attributes = kept
			case "empty throws":
				table.ExceptionIndexTable, table.AttrLen = nil, 2
			case "duplicate throws":
				method.Attributes = append(method.Attributes, table)
			case "Signature":
				obj.ConstantPoolManager.AddUtf8Info("Signature")
				method.Attributes = append(method.Attributes, &SignatureAttribute{AttrLen: 2, SignatureIndex: uint16(obj.ConstantPoolManager.AddUtf8Info(descriptor))})
			case "changed code":
				constructorLoweringCode(method).Code[1] = 0x2b
			case "changed flags":
				method.AccessFlags |= 1
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := obj.Bytes()
			if _, err := Parse(input); variant == "duplicate throws" {
				// Duplicate method attributes are malformed and belong to the
				// parser refusal, before the semantic projection boundary.
				if err == nil {
					t.Fatal("duplicate Exceptions table parsed")
				}
			} else if err != nil {
				t.Fatal("negative input must reach semantic checks", err)
			}
			if variant == "already finished" {
				var err error
				input, err = a.lower(old, input, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			oldHash, freshHash := sha256.Sum256(old), sha256.Sum256(input)
			result, err := a.lower(old, input, work)
			positive := variant == "control" || variant == "opaque attribute" || variant == "already finished"
			if !positive {
				if err == nil || result != nil {
					t.Fatal("unproved metadata edit accepted", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				finished := mustParseLowering(t, result)
				desc, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
				ctor, _ := sourceAnonymousConstructorMethod(finished, desc, nil)
				if _, present, valid := sourceAnonymousExceptionNames(finished, ctor, nil); !valid || present {
					t.Fatal("original absence not restored")
				}
				if variant == "already finished" && !bytes.Equal(input, result) {
					t.Fatal("non-idempotent projection")
				}
				if variant == "opaque attribute" {
					found := false
					for _, attr := range ctor.Attributes {
						if opaque, ok := attr.(*UnparsedAttribute); ok && opaque.Name == "IndependentWitness" {
							found = bytes.Equal(opaque.Info, []byte{9, 2, 7})
						}
					}
					if !found {
						t.Fatal("unrelated fresh metadata lost")
					}
				}
			}
			if sha256.Sum256(old) != oldHash || sha256.Sum256(input) != freshHash {
				t.Fatal("input mutation")
			}
		})
	}
}
