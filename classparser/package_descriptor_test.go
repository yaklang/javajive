package javaclassparser

import (
	"bytes"
	"math"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
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

func TestPackageDescriptorHistoricalProjectionRequiresClosedDependencies(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{
		"meta/package-info.java": `@Labels.Tag(kind=Labels.Kind.ONE) package meta;`,
		"meta/Labels.java": `package meta;import java.lang.annotation.*;public class Labels {
 public enum Kind {ONE,TWO}
 @Retention(RetentionPolicy.RUNTIME) @Target(ElementType.PACKAGE) public @interface Tag {Kind kind();}
}`,
	}, "none", "8")
	for _, variant := range []string{"control", "reordered rows", "modern abstract", "modern missing abstract", "public descriptor", "enum descriptor", "future version", "preview version", "missing dependency", "duplicate dependency", "dependency owner", "dependency name", "dependency flags", "anonymous dependency", "descriptor member", "duplicate table", "budget"} {
		t.Run(variant, func(t *testing.T) {
			old := mustParseLowering(t, files["meta/package-info.class"])
			old.MajorVersion, old.AccessFlags = 49, 0x0200
			fresh := mustParseLowering(t, files["meta/package-info.class"])
			var table *InnerClassesAttribute
			for _, attr := range fresh.Attributes {
				if a, ok := attr.(*InnerClassesAttribute); ok {
					table = a
				}
			}
			if table == nil || len(table.Classes) != 2 {
				t.Fatal("nested annotation and enum rows")
			}
			var work *workbudget.Budget
			switch variant {
			case "reordered rows":
				table.Classes[0], table.Classes[1] = table.Classes[1], table.Classes[0]
			case "modern abstract":
				old.MajorVersion, old.AccessFlags = 52, 0x0600
			case "modern missing abstract":
				old.MajorVersion = 50
			case "public descriptor":
				old.AccessFlags |= 1
			case "enum descriptor":
				old.AccessFlags |= 0x4000
			case "future version":
				old.MajorVersion = 66
			case "preview version":
				old.MinorVersion = 65535
			case "missing dependency":
				table.Classes = table.Classes[:1]
				table.NumberOfClasses = 1
			case "duplicate dependency":
				table.Classes[1] = table.Classes[0]
			case "dependency owner":
				table.Classes[0].OuterClassInfoIndex = fresh.SuperClass
			case "dependency name":
				table.Classes[0].InnerNameIndex = uint16(fresh.ConstantPoolManager.AddUtf8Info("Different"))
			case "dependency flags":
				table.Classes[0].InnerClassAccessFlags ^= 1
			case "anonymous dependency":
				table.Classes[0].OuterClassInfoIndex, table.Classes[0].InnerNameIndex = 0, 0
			case "descriptor member":
				table.Classes[0].OuterClassInfoIndex = fresh.ThisClass
			case "duplicate table":
				fresh.Attributes = append(fresh.Attributes, table)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			input := fresh.Bytes()
			saved := bytes.Clone(input)
			out, err := finishPackageDescriptor(old.Bytes(), input, work)
			if !bytes.Equal(input, saved) {
				t.Fatal("publication changed caller bytes")
			}
			if variant != "control" && variant != "reordered rows" && variant != "modern abstract" {
				if err == nil || out != nil {
					t.Fatal("unproved declaration published", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := mustParseLowering(t, out)
			if got.MajorVersion != fresh.MajorVersion || got.AccessFlags != 0x0600 {
				t.Fatal("loaded flags or target version changed")
			}
			// Exactly the two-byte access_flags word may differ. In particular,
			// no original pool, annotation table or opaque attribute is copied.
			r, _ := NewClassReaderWithBudget(input, nil)
			layout := &compiledMetadataLayout{}
			if _, err := parseWithMetadataLayout(r, nil, layout); err != nil {
				t.Fatal(err)
			}
			if len(out) != len(input) || !bytes.Equal(out[:layout.poolEnd], input[:layout.poolEnd]) || !bytes.Equal(out[layout.poolEnd+2:], input[layout.poolEnd+2:]) {
				t.Fatal("unrelated compiler bytes changed")
			}
		})
	}
}
