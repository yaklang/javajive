package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialSyntheticMethodFlagsKeepCompleteDeclarationsAndSpecialProtocols(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, syntheticMethodMetadataFixture, "MethodFlagOwner", "none", false)
	fresh := files["MethodFlagOwner.class"]
	want := mustParseLowering(t, bytes.Clone(fresh))
	for _, method := range want.Methods {
		name, _ := sourceBridgeUTF8(want, method.NameIndex)
		if name == "mix" || name == "apply" {
			method.AccessFlags |= syntheticMethodFlag
		}
	}
	original := want.Bytes()
	for _, variant := range []string{"control", "last visibility mismatch", "descriptor", "missing method", "extra method", "unexpected marker", "constructor protocol", "bridge protocol", "class identity", "work", "output", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			o := mustParseLowering(t, bytes.Clone(original))
			c := mustParseLowering(t, bytes.Clone(fresh))
			var work *workbudget.Budget
			var apply, mix, ctor *MemberInfo
			for _, method := range c.Methods {
				n, _ := sourceBridgeUTF8(c, method.NameIndex)
				switch n {
				case "apply":
					apply = method
				case "mix":
					mix = method
				case "<init>":
					ctor = method
				}
			}
			switch variant {
			case "last visibility mismatch":
				apply.AccessFlags = 2
			case "descriptor":
				apply.DescriptorIndex = uint16(c.ConstantPoolManager.AddUtf8Info("(I)J"))
			case "missing method":
				var keep []*MemberInfo
				for _, method := range c.Methods {
					if method != apply {
						keep = append(keep, method)
					}
				}
				c.Methods = keep
			case "extra method":
				copy := *apply
				copy.NameIndex = uint16(c.ConstantPoolManager.AddUtf8Info("newlyAdded"))
				c.Methods = append(c.Methods, &copy)
			case "unexpected marker":
				ctor.AccessFlags |= syntheticMethodFlag
			case "constructor protocol", "bridge protocol":
				for _, method := range o.Methods {
					n, _ := sourceBridgeUTF8(o, method.NameIndex)
					if variant == "constructor protocol" && n == "<init>" {
						method.AccessFlags |= syntheticMethodFlag
					}
					if variant == "bridge protocol" && n == "apply" {
						method.AccessFlags |= 0x0040
					}
				}
			case "class identity":
				c.AccessFlags |= 0x0010
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "output":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if mix == nil || apply == nil || ctor == nil {
				t.Fatal("authored method universe")
			}
			input, authority := c.Bytes(), o.Bytes()
			before, authorityBefore := bytes.Clone(input), bytes.Clone(authority)
			got, err := restoreDeclaredSyntheticMethodFlags(authority, input, work)
			if variant == "control" {
				if err != nil || bytes.Equal(got, input) || sourceMethodMetadataClosed(o, got, nil) != nil {
					t.Fatal("positive flag/bijection control", err)
				}
			} else if variant == "work" || variant == "output" || variant == "canceled" {
				if err == nil || got != nil {
					t.Fatal("resource failure returned a partial class", err)
				}
			} else {
				if err != nil || !bytes.Equal(got, input) {
					t.Fatal("unproved declaration was partially repaired", err)
				}
				if sourceMethodMetadataClosed(o, got, nil) == nil && variant != "class identity" {
					t.Fatal("existing declaration/flag refusal was weakened")
				}
			}
			if !bytes.Equal(input, before) || !bytes.Equal(authority, authorityBefore) {
				t.Fatal("flag restoration mutated inputs")
			}
		})
	}
}
