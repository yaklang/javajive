package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestInvocationMetadataReferenceHierarchyRequiresExactOriginalDependencyDeclaration(t *testing.T) {
	files := nativeCompileDebugClasses(t, `interface DeclaredView{}class DeclaredValue implements DeclaredView{}class OtherDeclaration{}class DeclarationOwner{DeclaredView value;}`, "none")
	obj, e := Parse(files["DeclarationOwner.class"])
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []string{"external", "archive declarations", "wrong identity", "truncated", "absent"} {
		t.Run(v, func(t *testing.T) {
			d := NewClassObjectDumper(obj)
			resolver := func(n string) ([]byte, bool) {
				b, ok := files[n+".class"]
				if n == "DeclaredValue" {
					if v == "wrong identity" {
						b = files["OtherDeclaration.class"]
					}
					if v == "truncated" {
						b = b[:len(b)/2]
					}
					if v == "absent" {
						return nil, false
					}
				}
				return b, ok
			}
			if v == "archive declarations" {
				d.archiveDeclarationResolver = resolver
			} else {
				d.declarationResolver = resolver
			}
			p := d.buildSiblingSuperTypes()
			got := types.IsReferenceSubtypeBridged("DeclaredValue", "DeclaredView", p)
			want := v == "external" || v == "archive declarations"
			if got != want {
				t.Fatalf("dependency evidence=%v want %v", got, want)
			}
			if _, known := p("DeclaredValue"); known != want {
				t.Fatal("unknown dependency admitted")
			}
		})
	}
}
