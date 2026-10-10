package javaclassparser

import (
	"context"
	"testing"

	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeCaptureNamesRequireSymmetricDisjointContours(t *testing.T) {
	for _, change := range []string{"disjoint", "reverse order", "same identity", "different source name", "old encloses new", "new encloses old", "earlier history encloses new", "retained snapshot", "later contour overlaps", "nil map", "empty name", "nil identity", "not visible", "nil visible identity", "too many visible", "too many names", "work", "memory", "canceled"} {
		t.Run(change, func(t *testing.T) {
			a, b, c := coreutils.NewRootVariableId(), coreutils.NewRootVariableId(), coreutils.NewRootVariableId()
			names := &nativeCaptureNameBindings{}
			first := map[*coreutils.VariableId]bool{a: true}
			second := map[*coreutils.VariableId]bool{b: true}
			name := "slice"
			if change == "new encloses old" || change == "retained snapshot" {
				first[b] = true
			}
			if change == "earlier history encloses new" {
				first[c] = true
			}
			if change == "reverse order" {
				a, b = b, a
				first = map[*coreutils.VariableId]bool{a: true}
				second = map[*coreutils.VariableId]bool{b: true}
			}
			if !names.bind(name, a, first, nil) {
				t.Fatal("initial contour")
			}
			var work *workbudget.Budget
			switch change {
			case "same identity":
				b = a
				second = first
			case "different source name":
				name = "other"
			case "old encloses new":
				second[a] = true
			case "earlier history encloses new":
				if !names.bind(name, b, second, nil) {
					t.Fatal("sibling")
				}
				b = c
				second = map[*coreutils.VariableId]bool{c: true}
			case "retained snapshot":
				delete(first, b)
			case "later contour overlaps":
				if !names.bind(name, b, second, nil) {
					t.Fatal("sibling")
				}
				second[a] = true
			case "nil map":
				names = nil
			case "empty name":
				name = ""
			case "nil identity":
				b = nil
			case "not visible":
				delete(second, b)
			case "nil visible identity":
				second[nil] = true
			case "too many visible":
				for i := 0; i < 4097; i++ {
					second[coreutils.NewRootVariableId()] = true
				}
			case "too many names":
				for i := 0; i < 4097; i++ {
					names.bindings[string(rune(i+256))] = nil
				}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := change == "disjoint" || change == "reverse order" || change == "same identity" || change == "different source name"
			if got := names.bind(name, b, second, work); got != want {
				t.Fatalf("source contour admitted=%v", got)
			}
		})
	}
}

func TestNativeCaptureNameSnapshotsBoundTotalRetainedEvidence(t *testing.T) {
	names := &nativeCaptureNameBindings{}
	id := coreutils.NewRootVariableId()
	visible := map[*coreutils.VariableId]bool{id: true}
	for i := 1; i < 4096; i++ {
		visible[coreutils.NewRootVariableId()] = true
	}
	for i := 0; i < 16; i++ {
		if !names.bind(string(rune('A'+i)), id, visible, nil) {
			t.Fatal("bounded contour", i)
		}
	}
	if names.entries != 65536 || !names.bind("A", id, visible, nil) || names.entries != 65536 {
		t.Fatal("stable snapshot was copied or exact bound rejected")
	}
	next := coreutils.NewRootVariableId()
	if names.bind("extra", next, map[*coreutils.VariableId]bool{next: true}, nil) || len(names.bindings) != 16 || names.entries != 65536 {
		t.Fatal("failed admission retained unbounded evidence")
	}
}
