package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialLambdaLocalNamespaceFollowsOriginalBodyIdentity(t *testing.T) {
	for _, variant := range []string{"original", "detached declaration", "different dumper", "duplicate declaration", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			first, second := &MemberInfo{}, &MemberInfo{}
			d := &ClassObjectDumper{obj: &ClassObject{Methods: []*MemberInfo{first, second}}}
			one, known := d.lambdaLocalNamespaceID(first)
			two, another := d.lambdaLocalNamespaceID(second)
			if !known || !another || one == 0 || two == 0 || one == two {
				t.Fatal("distinct original implementations need disjoint alpha namespaces")
			}
			switch variant {
			case "detached declaration":
				first = &MemberInfo{}
			case "different dumper":
				d.obj = &ClassObject{Methods: []*MemberInfo{{}, {}}}
			case "duplicate declaration":
				d.obj.Methods = append(d.obj.Methods, first)
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				// A new original still needs bounded allocation. An existing
				// namespace lookup allocates no replacement identity.
				first = &MemberInfo{}
				d.obj.Methods = append(d.obj.Methods, first)
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			again, known := d.lambdaLocalNamespaceID(first)
			if known != (variant == "original") || known && again != one || len(d.lambdaLocalNamespaces) != 2 {
				t.Fatalf("shared original namespace=%d/%v, initial=%d for %s", again, known, one, variant)
			}
		})
	}
}
