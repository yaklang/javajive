package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorVarargsRestorationNeedsPublishedExactPacket(t *testing.T) {
	original, fresh, a := constructorSuffixRoundTrip(t, constructorVarargsCaptureFixture(), "none", "SuffixOwner$1", "72:constructor:closed:suffix:oracle\n")
	for _, kind := range []string{"control", "already marked", "visibility", "synthetic", "synchronized", "capture field", "wrong descriptor", "missing publication", "stale original", "budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			obj := mustParseLowering(t, fresh)
			var ctor *MemberInfo
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "<init>" {
					if ctor != nil {
						t.Fatal("fixture has multiple constructors")
					}
					ctor = m
				}
			}
			if ctor == nil || ctor.AccessFlags != 0 {
				t.Fatal("fixed-arity physical source constructor")
			}
			authority := a.source.jar.sourceConstructorLowering
			want := bytes.Clone(original)
			var work *workbudget.Budget
			switch kind {
			case "already marked":
				ctor.AccessFlags |= 0x80
			case "visibility":
				ctor.AccessFlags |= 1
			case "synthetic":
				ctor.AccessFlags |= 0x1000
			case "synchronized":
				ctor.AccessFlags |= 0x20
			case "capture field":
				found := false
				for _, f := range obj.Fields {
					if f.AccessFlags&0x10 != 0 {
						f.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("unrelatedCapture"))
						found = true
						break
					}
				}
				if !found {
					t.Fatal("fixture lacks capture field")
				}
			case "wrong descriptor":
				ctor.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("()V"))
			case "missing publication":
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar}
			case "stale original":
				old := mustParseLowering(t, want)
				old.ConstantPoolManager.AddUtf8Info("different original namespace")
				want = old.Bytes()
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := obj.Bytes()
			saved := sha256.Sum256(input)
			originalHash := sha256.Sum256(want)
			got, err := authority.lower(want, input, work)
			if kind == "control" || kind == "already marked" {
				if err != nil || bytes.Equal(got, input) {
					t.Fatal("full packet must lower", err)
				}
				restored := mustParseLowering(t, got)
				found := false
				for _, m := range restored.Methods {
					name, _ := sourceBridgeUTF8(restored, m.NameIndex)
					if name == "<init>" {
						found = true
						if m.AccessFlags != 0x80 {
							t.Fatal("original varargs marker lost")
						}
					}
				}
				if !found {
					t.Fatal("constructor missing")
				}
				twice, err := authority.lower(want, got, nil)
				if err != nil || !bytes.Equal(twice, got) {
					t.Fatal("non-idempotent complete packet", err)
				}
			} else if err == nil || got != nil {
				t.Fatal("unproved original metadata restoration", kind, err)
			}
			if sha256.Sum256(input) != saved || sha256.Sum256(want) != originalHash {
				t.Fatal("input mutated")
			}
		})
	}
}
