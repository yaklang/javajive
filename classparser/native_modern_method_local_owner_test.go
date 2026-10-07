package javaclassparser

import (
	"encoding/binary"
	"testing"
)

func TestNativeModernMethodLocalNestRequiresExactOriginalEnclosingDeclaration(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ModernLocalOwner.java": modernLocalCaptureFixture}, "none", "11")
	for _, kind := range []string{"original", "missing enclosing", "duplicate enclosing", "malformed enclosing", "initializer instead of method", "foreign enclosing class", "unknown method", "wrong descriptor", "duplicate declaration", "missing parent registration", "changed parent flags"} {
		t.Run(kind, func(t *testing.T) {
			objects := modernNestTestObjects(t, files)
			root, local := objects["ModernLocalOwner"], objects["ModernLocalOwner$1Entry"]
			raw := modernNestTestAttribute(local, "EnclosingMethod")
			if raw == nil {
				t.Fatal("original local EnclosingMethod absent")
			}
			index := binary.BigEndian.Uint16(raw.Info[2:])
			nt, ok := local.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
			if !ok {
				t.Fatal("original local declaration descriptor absent")
			}
			switch kind {
			case "missing enclosing":
				modernNestTestRemoveAttribute(local, "EnclosingMethod")
			case "duplicate enclosing":
				local.Attributes = append(local.Attributes, raw)
			case "malformed enclosing":
				raw.Length++
			case "initializer instead of method":
				binary.BigEndian.PutUint16(raw.Info[2:], 0)
			case "foreign enclosing class":
				cp := NewConstantPoolWithConstant(&local.ConstantPool)
				binary.BigEndian.PutUint16(raw.Info, uint16(cp.AddNewClassInfo("ForeignOwner")))
			case "unknown method":
				local.ConstantPool = append(local.ConstantPool, &ConstantUtf8Info{Value: "unknown"})
				nt.NameIndex = uint16(len(local.ConstantPool))
			case "wrong descriptor":
				local.ConstantPool = append(local.ConstantPool, &ConstantUtf8Info{Value: "()Ljava/lang/Object;"})
				nt.DescriptorIndex = uint16(len(local.ConstantPool))
			case "duplicate declaration":
				for _, method := range root.Methods {
					if name, _ := sourceBridgeUTF8(root, method.NameIndex); name == "make" {
						root.Methods = append(root.Methods, method)
						break
					}
				}
			case "missing parent registration":
				for i, a := range root.Attributes {
					if _, ok := a.(*InnerClassesAttribute); ok {
						root.Attributes = append(root.Attributes[:i], root.Attributes[i+1:]...)
						break
					}
				}
			case "changed parent flags":
				for _, a := range root.Attributes {
					if rows, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range rows.Classes {
							if name, known := sourceBridgeClassName(root, row.InnerClassInfoIndex); known && name == local.GetClassName() {
								row.InnerClassAccessFlags ^= 0x10
							}
						}
					}
				}
			}
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 11
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if object := objects[name]; object != nil {
					return object.Bytes(), true
				}
				return nil, false
			}
			_, known := d.nativeModernNestOriginalScope()
			if known != (kind == "original") {
				t.Fatalf("method-local original whole-nest ownership=%v", known)
			}
		})
	}
}
