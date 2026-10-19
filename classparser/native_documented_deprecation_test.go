package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeDocumentationDeprecationRequiresExactOriginalEncoding(t *testing.T) {
	files := nativeCompileClasses(t, `/** @deprecated marker without runtime annotation */ class DocumentedSubject{} @Deprecated class AnnotatedSubject{}`)
	for _, scenario := range []string{"original", "missing marker", "duplicate marker", "nonempty marker", "nil marker", "runtime annotation", "duplicate annotation", "invisible annotation", "unpaired annotation", "nil annotation table", "nil annotation", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(files["DocumentedSubject.class"])
			if err != nil {
				t.Fatal(err)
			}
			annotated, err := Parse(files["AnnotatedSubject.class"])
			if err != nil {
				t.Fatal(err)
			}
			var marker *DeprecatedAttribute
			var annotation *RuntimeVisibleAnnotationsAttribute
			for _, attr := range obj.Attributes {
				if a, ok := attr.(*DeprecatedAttribute); ok {
					marker = a
				}
			}
			for _, attr := range annotated.Attributes {
				if a, ok := attr.(*RuntimeVisibleAnnotationsAttribute); ok {
					annotation = a
				}
			}
			if marker == nil || annotation == nil || len(annotation.Annotations) != 1 || !nativeMemberDocumentationDeprecated(obj, nil) || nativeMemberDeprecatedMarkerRepresentable(obj, nil) {
				t.Fatal("independent compiler did not emit distinct documentation and annotation encodings")
			}
			removeMarker := func() {
				var attrs []AttributeInfo
				for _, attr := range obj.Attributes {
					if _, ok := attr.(*DeprecatedAttribute); !ok {
						attrs = append(attrs, attr)
					}
				}
				obj.Attributes = attrs
			}
			var work *workbudget.Budget
			switch scenario {
			case "missing marker":
				removeMarker()
			case "duplicate marker":
				obj.Attributes = append(obj.Attributes, marker)
			case "nonempty marker":
				marker.AttrLen = 1
			case "nil marker":
				obj.Attributes = append(obj.Attributes, (*DeprecatedAttribute)(nil))
			case "runtime annotation", "duplicate annotation", "invisible annotation", "unpaired annotation":
				obj.Attributes = append(obj.Attributes, annotation)
				if scenario == "duplicate annotation" {
					annotation.Annotations = append(annotation.Annotations, annotation.Annotations[0])
				}
				if scenario == "invisible annotation" {
					annotation.IsInvisible = true
				}
				if scenario == "unpaired annotation" {
					removeMarker()
				}
			case "nil annotation table":
				obj.Attributes = append(obj.Attributes, (*RuntimeVisibleAnnotationsAttribute)(nil))
			case "nil annotation":
				annotation.Annotations = append(annotation.Annotations, nil)
				obj.Attributes = append(obj.Attributes, annotation)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				// This class can have just the one marker attribute. Spend the
				// single permitted scan first so this checks actual exhaustion,
				// rather than assuming an original debug attribute also exists.
				if !nativeProofWork(work, 1) {
					t.Fatal("fresh one-scan budget already exhausted")
				}
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberDocumentationDeprecated(obj, work); got != (scenario == "original") {
				t.Fatalf("documentation projection=%v", got)
			}
			// Ordinary nondeprecated declarations and the exact annotation pair
			// remain representable, but neither may acquire a documentation tag.
			want := scenario == "original" || scenario == "missing marker" || scenario == "runtime annotation"
			if got := nativeMemberDeprecatedDeclarationRepresentable(obj, work); got != want {
				t.Fatalf("source declaration=%v want=%v", got, want)
			}
		})
	}
	if nativeMemberDocumentationDeprecated(nil, nil) || nativeMemberDeprecatedDeclarationRepresentable(nil, nil) {
		t.Fatal("absent original declaration admitted")
	}
}
