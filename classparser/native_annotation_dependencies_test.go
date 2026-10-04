package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
	"sort"
	"testing"
)

func TestNativeAnnotationDependenciesTrackAllOriginalValueEdges(t *testing.T) {
	leaf := &AnnotationAttribute{TypeName: "Lpkg/Nested;", ElementValuePairs: []*ElementValuePairAttribute{{Name: "type", Tag: 'c', Value: "[[Lpkg/ArrayElement;"}}}
	annotation := &AnnotationAttribute{TypeName: "Lpkg/Mark;", ElementValuePairs: []*ElementValuePairAttribute{
		{Name: "class", Tag: 'c', Value: "Lpkg/Owner$Child;"},
		{Name: "enum", Tag: 'e', Value: &EnumConstValue{TypeName: "Lpkg/Choice;", ConstName: "YES"}},
		{Name: "nested", Tag: '@', Value: leaf},
		{Name: "array", Tag: '[', Value: []*ElementValuePairAttribute{{Tag: '@', Value: leaf}, {Tag: 'c', Value: "I"}, {Tag: 'c', Value: "V"}}},
	}}
	for _, where := range []string{"class or field", "parameter", "default", "type-use", "code type-use"} {
		t.Run(where, func(t *testing.T) {
			attrs := []AttributeInfo{&RuntimeVisibleAnnotationsAttribute{Annotations: []*AnnotationAttribute{annotation}}}
			switch where {
			case "parameter":
				attrs = []AttributeInfo{&RuntimeVisibleParameterAnnotationsAttribute{ParameterAnnotations: [][]*AnnotationAttribute{{annotation}}}}
			case "default":
				attrs = []AttributeInfo{&AnnotationDefaultAttribute{DefaultValue: &ElementValuePairAttribute{Tag: '@', Value: annotation}}}
			case "type-use", "code type-use":
				attrs = []AttributeInfo{&TypeAnnotationsAttribute{Annotations: []*TypeAnnotation{{Annotation: annotation}}}}
				if where == "code type-use" {
					attrs = []AttributeInfo{&CodeAttribute{Attributes: attrs}}
				}
			}
			found := map[string]bool{}
			if !nativeAnnotationDependencies(attrs, nil, func(n string) { found[n] = true }) {
				t.Fatal("original value graph refused")
			}
			got := []string{}
			for n := range found {
				got = append(got, n)
			}
			sort.Strings(got)
			want := []string{"pkg/ArrayElement", "pkg/Choice", "pkg/Mark", "pkg/Nested", "pkg/Owner$Child"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("edges %v != %v", got, want)
			}
		})
	}
}

func TestNativeAnnotationDependenciesRejectMalformedAndUnboundedGraphs(t *testing.T) {
	for _, scenario := range []string{"annotation cycle", "array cycle", "unknown tag", "nil nested", "nil element", "wrong enum", "method descriptor", "generic class literal", "void array", "wrong annotation descriptor", "duplicate element", "unknown raw attribute", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			a := &AnnotationAttribute{TypeName: "Lp/Mark;", ElementValuePairs: []*ElementValuePairAttribute{{Name: "value", Tag: 'c', Value: "Lp/Child;"}}}
			attrs := []AttributeInfo{&RuntimeVisibleAnnotationsAttribute{Annotations: []*AnnotationAttribute{a}}}
			var work *workbudget.Budget
			e := a.ElementValuePairs[0]
			switch scenario {
			case "annotation cycle":
				e.Tag = '@'
				e.Value = a
			case "array cycle":
				e.Tag = '['
				e.Value = []*ElementValuePairAttribute{e}
			case "unknown tag":
				e.Tag = '?'
			case "nil nested":
				e.Tag = '@'
				e.Value = (*AnnotationAttribute)(nil)
			case "nil element":
				a.ElementValuePairs = []*ElementValuePairAttribute{nil}
			case "wrong enum":
				e.Tag = 'e'
				e.Value = "Lp/Choice;"
			case "method descriptor":
				e.Value = "()Lp/Child;"
			case "generic class literal":
				e.Value = "TT;"
			case "void array":
				e.Value = "[V"
			case "wrong annotation descriptor":
				a.TypeName = "[Lp/Mark;"
			case "duplicate element":
				a.ElementValuePairs = append(a.ElementValuePairs, e)
			case "unknown raw attribute":
				attrs = []AttributeInfo{&UnparsedAttribute{Name: "UnknownAnnotation"}}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if nativeAnnotationDependencies(attrs, work, func(string) {}) {
				t.Fatal("unclosed metadata accepted")
			}
		})
	}
}

func TestNativeFieldAnnotationTypePrefixProof(t *testing.T) {
	files := nativeCompileClasses(t, `import java.lang.annotation.*;@Retention(RetentionPolicy.RUNTIME) @Target({ElementType.FIELD,ElementType.TYPE_USE}) @interface BothOrigin{String value();}class FieldOrigin{@BothOrigin("same") Object value;}`)
	for _, scenario := range []string{"original", "different value", "different visibility", "array dimension", "type argument", "method return", "missing type table", "nil field", "nil type", "nil annotation"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(files["FieldOrigin.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(obj)
			if _, err := d.DumpClass(); err != nil {
				t.Fatal(err)
			}
			field := obj.Fields[0]
			var declaration *AnnotationAttribute
			var table *TypeAnnotationsAttribute
			for _, attr := range field.Attributes {
				switch a := attr.(type) {
				case *RuntimeVisibleAnnotationsAttribute:
					declaration = a.Annotations[0]
				case *TypeAnnotationsAttribute:
					table = a
				}
			}
			if declaration == nil || table == nil || len(table.Annotations) != 1 {
				t.Fatal("missing original dual witness")
			}
			typ, err := types.ParseDescriptor("Ljava/lang/Object;")
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "different value":
				table.Annotations[0].Annotation.ElementValuePairs[0].Value = "other"
			case "different visibility":
				table.IsInvisible = true
			case "array dimension":
				table.Annotations[0].TypePath = []TypePathEntry{{Kind: typePathKindArray}}
			case "type argument":
				table.Annotations[0].TypePath = []TypePathEntry{{Kind: typePathKindTypeArg}}
			case "method return":
				table.Annotations[0].TargetType = 0x14
			case "missing type table":
				field.Attributes = nil
			case "nil field":
				field = nil
			case "nil type":
				typ = nil
			case "nil annotation":
				declaration = nil
			}
			if got := d.fieldAnnotationEmittedByType(field, typ, declaration, false); got != (scenario == "original") {
				t.Fatalf("coalesced=%v", got)
			}
		})
	}
}
