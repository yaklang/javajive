package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// An actual declaration certificate wins over a flat sibling. Without that
// certificate '$' remains part of the binary identifier, not evidence of
// nesting. Both ordinary annotation values and defaults use this binding.
func TestNativeEnumAnnotationValueUsesOriginalDeclarationNamespace(t *testing.T) {
	for _, row := range []struct {
		name, descriptor, source, want string
		known                          bool
	}{
		{"owned", "Lsample/Mark$Choice;", "sample.Mark.Choice", "sample.Mark.Choice.SECOND", true},
		{"dollar top level", "Lsample/Mark$Choice;", "", "Mark$Choice.SECOND", false},
		{"same simple name foreign", "Lother/Mark$Choice;", "other.Mark.Choice", "other.Mark.Choice.SECOND", true},
		{"multi-level", "Lsample/Outer$Inner$Choice;", "sample.Outer.Inner.Choice", "sample.Outer.Inner.Choice.SECOND", true},
	} {
		t.Run(row.name, func(t *testing.T) {
			binary := strings.ReplaceAll(row.descriptor[1:len(row.descriptor)-1], "/", ".")
			ctx := &class_context.ClassContext{PackageName: "sample", DeclarationSourceName: func(name string) (string, bool) {
				if name == binary {
					return row.source, row.known
				}
				return "", false
			}}
			d := &ClassObjectDumper{FuncCtx: ctx}
			e := &ElementValuePairAttribute{Name: "value", Tag: 'e', Value: &EnumConstValue{TypeName: row.descriptor, ConstName: "SECOND"}}
			got, err := d.formatAnnotationElementValue(e)
			if err != nil || got != row.want {
				t.Fatalf("default %q %v want %q", got, err, row.want)
			}
			got, err = d.DumpAnnotation(&AnnotationAttribute{TypeName: "Lsample/Mark;", ElementValuePairs: []*ElementValuePairAttribute{e}})
			if err != nil || got != "@Mark(value="+row.want+")" {
				t.Fatalf("actual %q %v", got, err)
			}
		})
	}
}
func TestNativeEnumAnnotationMalformedValueFailsClosed(t *testing.T) {
	d := &ClassObjectDumper{FuncCtx: &class_context.ClassContext{}}
	for _, v := range []any{nil, (*EnumConstValue)(nil), "value", &EnumConstValue{TypeName: "I"}, &EnumConstValue{TypeName: "[Lsample/Choice;"}, &EnumConstValue{TypeName: "L;"}, &EnumConstValue{TypeName: "Lsample/Choice"}} {
		if _, err := d.formatAnnotationEnumConstant(v); err == nil {
			t.Fatalf("accepted invalid original enum descriptor/value %#v", v)
		}
	}
}
