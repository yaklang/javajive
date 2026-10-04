package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeMemberParameterAnnotationOriginalArityProof(t *testing.T) {
	a := &AnnotationAttribute{TypeName: "Lp/Mark;"}
	for _, row := range []struct {
		name, method, descriptor string
		enclosing                bool
		table                    [][]*AnnotationAttribute
		accept                   bool
	}{
		{"method", "run", "(Ljava/lang/Object;J)V", true, [][]*AnnotationAttribute{{a}, {a}}, true},
		{"source constructor", "<init>", "(Lp/Owner;Ljava/lang/Object;J)V", true, [][]*AnnotationAttribute{{a}, {a}}, true},
		{"descriptor constructor", "<init>", "(Lp/Owner;Ljava/lang/Object;J)V", true, [][]*AnnotationAttribute{nil, {a}, {a}}, true},
		{"annotated omitted operand", "<init>", "(Lp/Owner;Ljava/lang/Object;J)V", true, [][]*AnnotationAttribute{{a}, {a}, {a}}, false},
		{"static constructor", "<init>", "(Ljava/lang/Object;J)V", false, [][]*AnnotationAttribute{{a}, {a}}, true},
		{"short static table", "<init>", "(Ljava/lang/Object;J)V", false, [][]*AnnotationAttribute{{a}}, false},
		{"short ordinary table", "run", "(Ljava/lang/Object;J)V", true, [][]*AnnotationAttribute{{a}}, false},
		{"oversized table", "run", "(Ljava/lang/Object;)V", true, [][]*AnnotationAttribute{{a}, {a}}, false},
		{"bad descriptor", "run", "not a descriptor", false, nil, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			table := &RuntimeVisibleParameterAnnotationsAttribute{NumParameters: uint8(len(row.table)), ParameterAnnotations: row.table}
			if got := nativeMemberParameterAnnotationsClosed(table, row.method, row.descriptor, row.enclosing, nil); got != row.accept {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
	for _, scenario := range []string{"nil table", "inconsistent count", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			table := &RuntimeVisibleParameterAnnotationsAttribute{NumParameters: 1, ParameterAnnotations: [][]*AnnotationAttribute{{a}}}
			var work *workbudget.Budget
			switch scenario {
			case "nil table":
				table = nil
			case "inconsistent count":
				table.NumParameters = 2
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if nativeMemberParameterAnnotationsClosed(table, "run", "(Ljava/lang/Object;)V", true, work) {
				t.Fatal("unproved parameter table accepted")
			}
		})
	}
}

func TestNativeMemberAnnotationProjectionKeepsOriginalTableIndices(t *testing.T) {
	files := nativeCompileClasses(t, strings.Replace(nativeMemberParameterAnnotationFixture, "public class AnnotatedDriver", "class AnnotatedDriver", 1))
	obj, err := Parse(files["AnnotatedMember$Child.class"])
	if err != nil {
		t.Fatal(err)
	}
	enclosing, err := Parse(files["AnnotatedMember.class"])
	if err != nil {
		t.Fatal(err)
	}
	child := nativeMemberProofWithOwner(obj, enclosing, nil)
	if child == nil {
		t.Fatal("original member proof refused")
	}
	const descriptor = "(LAnnotatedMember;Ljava/lang/Object;J)V"
	for _, row := range []struct {
		name          string
		native        bool
		table, source int
		want          []int
	}{
		{"native source table", true, 2, 2, []int{0, 1}},
		{"native descriptor table", true, 3, 2, []int{1, 2}},
		{"flat source table", false, 2, 3, []int{-1, 0, 1}},
		{"flat descriptor table", false, 3, 3, []int{0, 1, 2}},
	} {
		t.Run(row.name, func(t *testing.T) {
			d := NewClassObjectDumper(obj)
			if row.native {
				d.nativeMemberCurrent = child
			}
			for i, want := range row.want {
				if got := dumpedParamAnnoIndex(d, "<init>", descriptor, row.source, row.table, i); got != want {
					t.Fatalf("source index%d: original table index%d != %d", i, got, want)
				}
			}
		})
	}
}
