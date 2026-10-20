package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialSourceComponentInputsCountDistinctRetainedModels(t *testing.T) {
	files := nativeCompileClasses(t, "class ModelOwner{}class ModelPeer{}")
	raw := files["ModelOwner.class"]
	planFor := func(t *testing.T) *nativeMemberPrepared {
		t.Helper()
		object, err := Parse(bytes.Clone(raw))
		if err != nil {
			t.Fatal(err)
		}
		return &nativeMemberPrepared{root: object, objects: map[string]*ClassObject{"ModelOwner": object}, family: &nativeMemberFamily{lexicalObjects: map[string]*ClassObject{"ModelOwner": object}}}
	}
	t.Run("shared and distinct parses", func(t *testing.T) {
		z := nativeArchive(t, files)
		defer z.Close()
		a := nativeMemberTransactionInputs{}
		first, second := planFor(t), planFor(t)
		first.family.modernNestObjects = first.family.lexicalObjects
		if !a.add(z, first, nil) || !a.add(z, first, nil) || a.bytes != int64(len(raw)) || len(a.models) != 1 {
			t.Fatal("shared model counted repeatedly")
		}
		if !a.add(z, second, nil) || a.bytes != 2*int64(len(raw)) || len(a.models) != 2 {
			t.Fatal("distinct retained parses collapsed by binary name")
		}
	})
	for _, variant := range []string{"nil plan", "nil root", "missing owned root", "nil model", "foreign identity", "missing original", "oversize original", "cumulative bytes", "model count", "memory", "scan limit", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			input := map[string][]byte{}
			for n, b := range files {
				input[n] = bytes.Clone(b)
			}
			plan := planFor(t)
			a := nativeMemberTransactionInputs{}
			var work *workbudget.Budget
			switch variant {
			case "nil plan":
				plan = nil
			case "nil root":
				plan.root = nil
			case "missing owned root":
				plan.family.lexicalObjects = nil
			case "nil model":
				plan.objects["ModelOwner"] = nil
			case "foreign identity":
				plan.objects["ModelOwner"], _ = Parse(files["ModelPeer.class"])
			case "missing original":
				delete(input, "ModelOwner.class")
			case "oversize original":
				input["ModelOwner.class"] = make([]byte, (2<<20)+1)
			case "cumulative bytes":
				a.bytes = (128 << 20) - int64(len(raw)) + 1
			case "model count":
				a.models = map[*ClassObject]bool{}
				for i := 0; i < 4096; i++ {
					a.models[&ClassObject{}] = true
				}
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "scan limit":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			z := nativeArchive(t, input)
			defer z.Close()
			if a.add(z, plan, work) {
				t.Fatal("unbounded or unbound retained model admitted")
			}
			if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
				t.Fatal("input accounting reserved source publication")
			}
		})
	}
}
