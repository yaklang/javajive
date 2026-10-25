package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialSyntheticFlagsRequirePublishedLambdaBijection(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	original, fresh, artifact := constructorSuffixFamilyRoundTrip(t, lambdaSyntheticCompositionFixture, "source,lines,vars", []string{"SyntheticLambdaOwner", "SyntheticLambdaOwner$Reader"}, "24:lambda:ordinary-synthetic:binding:overflow\n")
	active := artifact.source.jar.sourceLambdaMetadata.plan("SyntheticLambdaOwner")
	if active == nil || len(active.methods) < 2 {
		t.Fatal("fixture has no published nested-lambda projection")
	}
	for _, variant := range []string{"control", "idempotent", "no plan", "stale original", "wrong owner", "unreturned source", "duplicate generated", "duplicate original", "descriptor", "ordinary method", "missing method", "visibility", "unexpected synthetic", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			plan := *active
			plan.methods = append([]sourceLambdaMetadataMethod(nil), active.methods...)
			published := &plan
			obj := mustParseLowering(t, bytes.Clone(fresh))
			want := mustParseLowering(t, bytes.Clone(original))
			var work *workbudget.Budget
			switch variant {
			case "no plan":
				published = nil
			case "stale original":
				plan.originalHash[0] ^= 1
			case "wrong owner":
				plan.owner = "ForeignOwner"
			case "unreturned source":
				plan.sourceHash = [32]byte{}
			case "duplicate generated":
				plan.methods[1].generated = plan.methods[0].generated
			case "duplicate original":
				plan.methods[1].original = plan.methods[0].original
			case "descriptor":
				plan.methods[0].generated.descriptor = "()V"
			case "ordinary method":
				for _, m := range want.Methods {
					name, _ := sourceBridgeUTF8(want, m.NameIndex)
					desc, _ := sourceBridgeUTF8(want, m.DescriptorIndex)
					if name == "choose" {
						plan.methods[0].original = sourceLambdaMethodIdentity{name, desc}
						plan.methods[0].generated.descriptor = desc
					}
				}
			case "missing method":
				obj.Methods = obj.Methods[:len(obj.Methods)-1]
			case "visibility":
				obj.Methods[0].AccessFlags ^= 1
			case "unexpected synthetic":
				obj.Methods[0].AccessFlags |= syntheticMethodFlag
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := obj.Bytes()
			if variant == "idempotent" {
				var err error
				input, err = restoreReconstructionSyntheticFlagsWithLambda(original, input, published, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			saved := bytes.Clone(input)
			result, err := restoreReconstructionSyntheticFlagsWithLambda(original, input, published, work)
			if !bytes.Equal(input, saved) {
				t.Fatal("input mutated")
			}
			switch variant {
			case "control":
				if err != nil || len(result) != len(input) {
					t.Fatal("flag completion", err)
				}
				edits := 0
				for i, b := range input {
					if result[i] != b {
						if result[i] != b|0x10 {
							t.Fatal("changed non-synthetic bit")
						}
						edits++
					}
				}
				if edits != 1 {
					t.Fatalf("expected exactly one synthetic bit, got %d changed bytes", edits)
				}
			case "idempotent", "no plan", "missing method", "visibility", "unexpected synthetic":
				if err != nil || !bytes.Equal(input, result) {
					t.Fatal("unproved universe partially modified", err)
				}
			default:
				if err == nil || result != nil {
					t.Fatal("unpublished or ambiguous mapping accepted", err)
				}
			}
		})
	}
}
