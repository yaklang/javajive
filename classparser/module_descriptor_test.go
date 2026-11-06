package javaclassparser

import (
	"strings"
	"testing"
)

func moduleAttributeFixture() []byte {
	var data []byte
	u2 := func(value uint16) {
		data = append(data, byte(value>>8), byte(value))
	}
	u2(1) // module_name_index
	u2(0) // module_flags
	u2(0) // module_version_index
	u2(2) // requires_count
	u2(3)
	u2(0x8000) // java.base, mandated
	u2(0)
	u2(5)
	u2(0x0060) // transitive | static
	u2(0)
	u2(1) // exports_count
	u2(7)
	u2(0)
	u2(1)
	u2(9)
	u2(1) // opens_count
	u2(11)
	u2(0)
	u2(0)
	u2(1) // uses_count
	u2(13)
	u2(1) // provides_count
	u2(13)
	u2(1)
	u2(15)
	return data
}

func moduleAttributeCP() []ConstantInfo {
	return []ConstantInfo{
		&ConstantModuleInfo{NameIndex: 2},
		&ConstantUtf8Info{Value: "m.example"},
		&ConstantModuleInfo{NameIndex: 4},
		&ConstantUtf8Info{Value: "java.base"},
		&ConstantModuleInfo{NameIndex: 6},
		&ConstantUtf8Info{Value: "other.mod"},
		&ConstantPackageInfo{NameIndex: 8},
		&ConstantUtf8Info{Value: "pkg/internal"},
		&ConstantModuleInfo{NameIndex: 10},
		&ConstantUtf8Info{Value: "friend.mod"},
		&ConstantPackageInfo{NameIndex: 12},
		&ConstantUtf8Info{Value: "pkg/open"},
		&ConstantClassInfo{NameIndex: 14},
		&ConstantUtf8Info{Value: "pkg/Service"},
		&ConstantClassInfo{NameIndex: 16},
		&ConstantUtf8Info{Value: "pkg/Impl"},
	}
}

func TestParseModuleDescriptor(t *testing.T) {
	obj := NewClassObject()
	obj.ConstantPool = moduleAttributeCP()

	descriptor, ok := parseModuleDescriptor(obj, moduleAttributeFixture())
	if !ok {
		t.Fatal("valid Module attribute rejected")
	}
	want := `module m.example {
    requires transitive static other.mod;
    exports pkg.internal to friend.mod;
    opens pkg.open;
    uses pkg.Service;
    provides pkg.Service with pkg.Impl;
}
`
	if got := descriptor.source(); got != want {
		t.Fatalf("module source:\n%s\nwant:\n%s", got, want)
	}

	var open []byte
	u2 := func(value uint16) { open = append(open, byte(value>>8), byte(value)) }
	u2(1)    // module_name_index
	u2(0x20) // ACC_OPEN
	u2(0)    // version
	for i := 0; i < 5; i++ {
		u2(0) // requires/exports/opens/uses/provides counts
	}
	opened, ok := parseModuleDescriptor(obj, open)
	if !ok {
		t.Fatal("open module rejected")
	}
	if got := opened.source(); got != "open module m.example {\n}\n" {
		t.Fatalf("open module source:\n%s", got)
	}

	if _, ok := parseModuleDescriptor(obj, moduleAttributeFixture()[:10]); ok {
		t.Fatal("truncated Module attribute accepted")
	}
	if _, ok := parseModuleDescriptor(obj, moduleAttributeFixture()[:len(moduleAttributeFixture())-2]); ok {
		t.Fatal("Module attribute with missing provides target accepted")
	}
}

func TestModuleDescriptorFallsBackWithoutAttribute(t *testing.T) {
	dumper := NewClassObjectDumper(NewClassObject())
	if source, ok := dumper.renderModuleDescriptor(); ok {
		t.Fatalf("missing Module attribute rendered as:\n%s", source)
	}
	obj := NewClassObject()
	obj.ConstantPool = moduleAttributeCP()
	obj.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "Module", Info: moduleAttributeFixture()}}
	dumper.obj = obj
	source, ok := dumper.renderModuleDescriptor()
	if !ok || !strings.Contains(source, "module m.example") {
		t.Fatalf("Module attribute not rendered: ok=%v source=%q", ok, source)
	}
}
