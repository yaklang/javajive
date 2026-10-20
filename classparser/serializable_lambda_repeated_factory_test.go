package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestNativeSerializableRepeatedFactoriesCannotCrossOriginalReturnRoutes(t *testing.T) {
	fixture := strings.Replace(serializableStaticReferenceFixture, "class SerializableReferences {", `class SerializableReferences {
 static java.util.function.Function<String,String> alias(){return (java.util.function.Function<String,String>&java.io.Serializable)ExternalOperations::Aa;}
`, 1)
	files := nativeCompileDebugClasses(t, fixture, "none")
	for _, change := range []string{"original", "different alias target", "inverted alias guard", "missing source alias"} {
		t.Run(change, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["SerializableReferences.class"]))
			if err != nil {
				t.Fatal(err)
			}
			plan := buildSerializableReferenceProtocol(object, nil)
			if plan == nil || len(plan.sites) != 3 {
				t.Fatal("original repeated-factory decision protocol missing")
			}
			code, ops, known := serializableOriginalCode(object, plan.helper, nil)
			if !known {
				t.Fatal("original repeated helper code")
			}
			patched := false
			switch change {
			case "different alias target":
				var other uint16
				for _, site := range plan.sites {
					if site.tuple.implementation.name == "BB" {
						other = site.tuple.index
					}
				}
				for _, op := range ops {
					if nativeEnumOpcode(op, core.OP_INVOKEDYNAMIC) {
						binary.BigEndian.PutUint16(code.Code[int(op.CurrentOffset)+1:], other)
						patched = true
						break
					}
				}
			case "inverted alias guard":
				for _, op := range ops {
					if nativeEnumOpcode(op, core.OP_IF_ICMPNE) {
						code.Code[int(op.CurrentOffset)] = core.OP_IF_ICMPEQ
						patched = true
						break
					}
				}
			case "missing source alias":
				for i, m := range object.Methods {
					if name, _ := sourceBridgeUTF8(object, m.NameIndex); name == "alias" {
						object.Methods = append(object.Methods[:i], object.Methods[i+1:]...)
						patched = true
						break
					}
				}
			}
			if change != "original" && !patched {
				t.Fatal("negative control did not change the original protocol")
			}
			if known := buildSerializableReferenceProtocol(object, nil) != nil; known != (change == "original") {
				t.Fatal("different original return path borrowed repeated-factory authority", change, known)
			}
		})
	}
}
