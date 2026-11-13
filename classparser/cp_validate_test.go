package javaclassparser

import (
	"strings"
	"testing"
)

func TestConstantPoolLinksRejectMalformedUnusedConstants(t *testing.T) {
	tests := []struct {
		name   string
		major  uint16
		append func(*ClassObject)
		want   string
	}{
		{
			name: "string index", major: 52, want: "CONSTANT_String.string_index",
			append: func(object *ClassObject) {
				object.ConstantPool = append(object.ConstantPool, &ConstantStringInfo{StringIndex: 0xffff})
			},
		},
		{
			name: "method type version", major: 50, want: "requires classfile major 51",
			append: func(object *ClassObject) {
				object.ConstantPool = append(object.ConstantPool, &ConstantMethodTypeInfo{DescriptorIndex: 1})
			},
		},
		{
			name: "dynamic version", major: 54, want: "requires classfile major 55",
			append: func(object *ClassObject) {
				object.ConstantPool = append(object.ConstantPool, &ConstantDynamicInfo{})
			},
		},
		{
			name: "module version", major: 52, want: "requires classfile major 53",
			append: func(object *ClassObject) {
				object.ConstantPool = append(object.ConstantPool, &ConstantModuleInfo{NameIndex: 1})
			},
		},
		{
			name: "method handle kind", major: 52, want: "invalid method handle reference kind",
			append: func(object *ClassObject) {
				object.ConstantPool = append(object.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 0, ReferenceIndex: 1})
			},
		},
	}
	seed := readValidationSeed(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object, err := Parse(seed)
			if err != nil {
				t.Fatal(err)
			}
			object.MajorVersion = test.major
			test.append(object)
			_, err = Parse(object.Bytes())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Parse error = %v, want %q", err, test.want)
			}
		})
	}
}
