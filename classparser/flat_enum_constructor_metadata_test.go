package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// Constructor erasure owns the forwarding instructions, not unrelated body
// bootstraps or parameter names. Admitting those tables still requires their
// original typed CP edges, physical slots and complete byte encoding.
func TestAdversarialFlatEnumConstructorMetadataRejectsBrokenTables(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"MetadataPacket.java": `class MetadataPacket{enum Mode{ONE(String.class,4L){long apply(long x){java.util.function.LongUnaryOperator f=v->v+1;return f.applyAsLong(x);}};Mode(Class<?> type,long value){}abstract long apply(long x);}}`}, "source,lines,vars", "8")
	for _, variant := range []string{"control", "bootstrap length", "bootstrap count", "bootstrap kind", "bootstrap target", "bootstrap argument", "argument count", "debug length", "debug count", "debug scope", "debug slot", "wide tail", "debug name", "debug descriptor", "generic signature", "duplicate slot"} {
		t.Run(variant, func(t *testing.T) {
			object := mustParseLowering(t, bytes.Clone(files["MetadataPacket$Mode$1.class"]))
			var code *CodeAttribute
			var parameters []string
			var local, generic *UnparsedAttribute
			var bootstrap *BootstrapMethodsAttribute
			for _, method := range object.Methods {
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				if name == "<init>" {
					desc, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
					parameters, _, _ = callbinding.Descriptor(desc)
					code = constructorLoweringCode(method)
				}
			}
			for _, attr := range code.Attributes {
				if table, ok := attr.(*UnparsedAttribute); ok {
					if table.Name == "LocalVariableTable" {
						local = table
					}
					if table.Name == "LocalVariableTypeTable" {
						generic = table
					}
				}
			}
			for _, attr := range object.Attributes {
				if table, ok := attr.(*BootstrapMethodsAttribute); ok {
					bootstrap = table
				}
			}
			if code == nil || local == nil || generic == nil || bootstrap == nil || len(local.Info) < 22 {
				t.Fatal("incomplete original fixture")
			}
			switch variant {
			case "bootstrap length":
				bootstrap.AttributeLength++
			case "bootstrap count":
				bootstrap.NumBootstrapMethods++
			case "bootstrap kind":
				object.ConstantPool[bootstrap.BootstrapMethods[0].BootstrapMethodRef-1].(*ConstantMethodHandleInfo).ReferenceKind = 5
			case "bootstrap target":
				object.ConstantPool[bootstrap.BootstrapMethods[0].BootstrapMethodRef-1].(*ConstantMethodHandleInfo).ReferenceIndex = object.ThisClass
			case "bootstrap argument":
				bootstrap.BootstrapMethods[0].BootstrapArguments[0] = 0
			case "argument count":
				bootstrap.BootstrapMethods[0].NumBootstrapArguments++
			case "debug length":
				local.Length++
			case "debug count":
				local.Info[1]++
			case "debug scope":
				local.Info[3]++
			case "debug slot":
				binary.BigEndian.PutUint16(local.Info[10:], 200)
			case "wide tail":
				binary.BigEndian.PutUint16(local.Info[10:], uint16(nativeMemberParameterWidth(parameters)))
			case "debug name":
				binary.BigEndian.PutUint16(local.Info[6:], sourceBridgePoolString(t, object, "bad/name"))
			case "debug descriptor":
				binary.BigEndian.PutUint16(local.Info[8:], sourceBridgePoolString(t, object, "I"))
			case "generic signature":
				binary.BigEndian.PutUint16(generic.Info[8:], sourceBridgePoolString(t, object, "TT;"))
			case "duplicate slot":
				copy(local.Info[20:22], local.Info[10:12])
			}
			valid := flatEnumBodyBootstrapTable(object, bootstrap, nil) && flatEnumConstructorDebugTable(object, code, local, parameters, nil) && flatEnumConstructorDebugTable(object, code, generic, parameters, nil)
			if valid != (variant == "control") {
				t.Fatalf("table proof=%v", valid)
			}
		})
	}
}

func TestAdversarialFlatEnumConstructorParameterRetainsNamesAndFlags(t *testing.T) {
	object := mustParseLowering(t, nativeCompileSourceReleaseClasses(t, map[string]string{"ParameterPacket.java": "class ParameterPacket{}"}, "none", "8")["ParameterPacket.class"])
	for _, tc := range []struct {
		name     string
		flags    uint16
		expected byte
		valid    bool
	}{
		{"", 0x1000, 0x10, true}, {"arg$0", 0x1010, 0x10, true}, {"value", 0x10, 0, true},
		{"value", 0x8000, 0, false}, {"value", 0x1000, 0, false}, {"value", 1, 0, false}, {"this", 0, 0, false}, {"bad/name", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := make([]byte, 4)
			if tc.name != "" {
				binary.BigEndian.PutUint16(row, sourceBridgePoolString(t, object, tc.name))
			}
			binary.BigEndian.PutUint16(row[2:], tc.flags)
			if flatEnumConstructorParameter(object, row, tc.expected, nil) != tc.valid {
				t.Fatal("wrong named parameter admission")
			}
		})
	}
}
