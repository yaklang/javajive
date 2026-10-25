package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A field expression uses Java's dotted class spelling, while declaration
// lookup uses JVM binary names. A default-package fixture cannot detect this
// boundary. Keep the original driver and providers unchanged, including all
// primitive widths, reference identity, volatile reads, and initialization.
func TestAdversarialPackagedForeignFieldInitializationKeepsOriginalABI(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, namespace := range []string{"example.fields", "renamed.scope"} {
		t.Run(namespace, func(t *testing.T) {
			binary := strings.ReplaceAll(namespace, ".", "/") + "/"
			source := "package " + namespace + ";\n" + interfaceForeignFieldInitializerFixture
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return localGetterCompileJavac8(t, map[string]string{binary + "InitPacketDriver.java": source}, debug)
			}, ModernJavac, modern, []string{binary + "InitPacketContract"}, namespace+".InitPacketDriver", "foreign:field:initialization:identity:abi\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func TestPackagedInitializerFieldLookupRequiresExactBinaryDeclaration(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"metadata/owner/Field$Origin.java": `package metadata.owner; class Field$Origin {static final int VALUE=Integer.parseInt("37");}`}, "none", "8")
	for _, variant := range []string{"source spelling", "binary spelling", "wrong namespace", "nested alias", "missing owner", "changed identity", "constant variable", "changed descriptor"} {
		t.Run(variant, func(t *testing.T) {
			object := mustParseLowering(t, files["metadata/owner/Field$Origin.class"])
			field := &values.JavaClassMember{Name: "metadata.owner.Field$Origin", Member: "VALUE", Description: "I"}
			switch variant {
			case "binary spelling":
				field.Name = "metadata/owner/Field$Origin"
			case "wrong namespace":
				field.Name = "other.owner.Field$Origin"
			case "nested alias":
				field.Name = "metadata.owner.Field.Origin"
			case "changed identity":
				object.ConstantPool[object.ThisClass-1].(*ConstantClassInfo).NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("other/owner/Field$Origin"))
			case "constant variable":
				object.Fields[0].Attributes = append(object.Fields[0].Attributes, &ConstantValueAttribute{})
			case "changed descriptor":
				field.Description = "J"
			}
			resolve := func(name string) (*ClassObject, bool) {
				if name == "metadata/owner/Field$Origin" && variant != "missing owner" {
					return object, true
				}
				return nil, false
			}
			want := variant == "source spelling" || variant == "binary spelling"
			if nativeAnonymousInitializerFieldDeclaration(field, true, resolve, nil) != want {
				t.Fatal("source spelling weakened binary declaration or nonconstant proof")
			}
		})
	}
}
