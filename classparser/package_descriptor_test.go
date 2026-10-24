package javaclassparser

import (
	"bytes"
	"math"
	"testing"
)

func TestPackageDescriptorPublicationRejectsChangedDeclarationAndAnnotationGraphs(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{
		"meta/package-info.java": `@Tag(text="a\u0000b",words={1,2},zero=-0.0,notNumber=Double.NaN) package meta;`,
		"meta/Tag.java":          `package meta;import java.lang.annotation.*;@Retention(RetentionPolicy.RUNTIME)@Target(ElementType.PACKAGE)public @interface Tag{String text();int[] words();double zero();double notNumber();}`,
	}, "none", "8")
	original := files["meta/package-info.class"]
	table := func(o *ClassObject) *RuntimeVisibleAnnotationsAttribute {
		for _, a := range o.Attributes {
			if a, ok := a.(*RuntimeVisibleAnnotationsAttribute); ok {
				return a
			}
		}
		t.Fatal("missing fixture table")
		return nil
	}
	element := func(o *ClassObject, name string) *ElementValuePairAttribute {
		for _, e := range table(o).Annotations[0].ElementValuePairs {
			if e.Name == name {
				return e
			}
		}
		t.Fatal("missing fixture element", name)
		return nil
	}
	for _, tt := range []struct {
		name   string
		change func(*ClassObject)
	}{
		{"changed flags", func(o *ClassObject) { o.AccessFlags &^= 0x1000 }},
		{"missing annotation", func(o *ClassObject) { table(o).Annotations = nil }},
		{"changed retention", func(o *ClassObject) { table(o).IsInvisible = true }},
		{"duplicate table", func(o *ClassObject) { o.Attributes = append(o.Attributes, table(o)) }},
		{"duplicate annotation", func(o *ClassObject) { a := table(o); a.Annotations = append(a.Annotations, a.Annotations[0]) }},
		{"changed string units", func(o *ClassObject) { element(o, "text").Value = "a\ufffdb" }},
		{"changed signed zero", func(o *ClassObject) { element(o, "zero").Value.(*ConstantDoubleInfo).Value = 0 }},
		{"changed NaN payload", func(o *ClassObject) {
			element(o, "notNumber").Value.(*ConstantDoubleInfo).Value = math.Float64frombits(0x7ff8000000000001)
		}},
		{"changed array order", func(o *ClassObject) {
			x := element(o, "words").Value.([]*ElementValuePairAttribute)
			x[0], x[1] = x[1], x[0]
		}},
		{"invented field", func(o *ClassObject) {
			o.Fields = []*MemberInfo{{AccessFlags: 0x19, NameIndex: uint16(o.ConstantPoolManager.AddUtf8Info("extra")), DescriptorIndex: uint16(o.ConstantPoolManager.AddUtf8Info("I"))}}
		}},
		{"invented method", func(o *ClassObject) {
			o.Methods = []*MemberInfo{{AccessFlags: 0x401, NameIndex: uint16(o.ConstantPoolManager.AddUtf8Info("extra")), DescriptorIndex: uint16(o.ConstantPoolManager.AddUtf8Info("()V"))}}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := mustParseLowering(t, bytes.Clone(original))
			tt.change(o)
			fresh := o.Bytes()
			before := bytes.Clone(fresh)
			if bytes.Equal(fresh, original) {
				t.Fatal("mutation did not change fixture")
			}
			if result, err := finishPackageDescriptor(original, fresh, nil); err == nil || result != nil {
				t.Fatal("changed metadata was published", err)
			}
			if !bytes.Equal(fresh, before) {
				t.Fatal("failed proof mutated compiler input")
			}
		})
	}
	// Constant-pool layout and element ordering are compiler choices. Reorder
	// elements while retaining every typed value, including the NaN and -0 bits.
	o := mustParseLowering(t, bytes.Clone(original))
	e := table(o).Annotations[0].ElementValuePairs
	e[0], e[len(e)-1] = e[len(e)-1], e[0]
	fresh := o.Bytes()
	done, err := finishPackageDescriptor(original, fresh, nil)
	if err != nil || !bytes.Equal(done, fresh) {
		t.Fatal("equivalent reordered metadata must keep fresh bytes", err)
	}
	if again, err := finishPackageDescriptor(original, done, nil); err != nil || !bytes.Equal(again, done) {
		t.Fatal("publication not idempotent", err)
	}
}
