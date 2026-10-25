package javaclassparser

import (
	"bytes"
	"testing"
)

// Removing a proved fresh array helper must not consume the independent field
// metadata obligation. Only an original synthetic marker can be pending; every
// executable field flag and the complete declaration bijection remain exact.
func TestAdversarialEnumArrayFieldContractComposesWithoutWaivingFlags(t *testing.T) {
	original, compiled, artifact := enumArrayLoweringInputs(t)
	for _, variant := range []string{"lost marker", "visibility", "static", "final", "volatile", "new marker", "descriptor", "extra field"} {
		t.Run(variant, func(t *testing.T) {
			object := mustParseLowering(t, bytes.Clone(compiled))
			var backing, constant *MemberInfo
			for _, field := range object.Fields {
				if field.AccessFlags == 0x101a {
					backing = field
				}
				if field.AccessFlags == 0x4019 {
					constant = field
				}
			}
			if backing == nil || constant == nil {
				t.Fatal("independent native enum field inventory")
			}
			backing.AccessFlags &^= syntheticFieldFlag
			switch variant {
			case "visibility":
				backing.AccessFlags = backing.AccessFlags&^2 | 1
			case "static":
				backing.AccessFlags &^= 8
			case "final":
				backing.AccessFlags &^= 16
			case "volatile":
				backing.AccessFlags = backing.AccessFlags&^16 | 64
			case "new marker":
				constant.AccessFlags |= syntheticFieldFlag
			case "descriptor":
				backing.DescriptorIndex = uint16(object.ConstantPoolManager.AddUtf8Info("[Ljava/lang/Enum;"))
			case "extra field":
				copy := *backing
				copy.NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("additionalStorage"))
				object.Fields = append(object.Fields, &copy)
			}
			input := object.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("field mutant must reach semantic proof", err)
			}
			before := bytes.Clone(input)
			got, err := artifact.source.jar.sourceConstructorLowering.lowerEnumArrayHelper("PacketValues", original, input, nil)
			if !bytes.Equal(before, input) {
				t.Fatal("input mutated")
			}
			if variant != "lost marker" {
				if err == nil && !bytes.Equal(got, input) || err != nil && got != nil {
					t.Fatal("unproved field contract exposed partially completed output", err)
				}
				return
			}
			if err != nil || bytes.Equal(got, input) {
				t.Fatal("independent array proof blocked by a pending field marker", err)
			}
			intermediate := mustParseLowering(t, got)
			for _, field := range intermediate.Fields {
				name, _ := sourceBridgeUTF8(intermediate, field.NameIndex)
				if name == "$VALUES" && field.AccessFlags&syntheticFieldFlag != 0 {
					t.Fatal("array edit incorrectly discharged unrelated field metadata")
				}
			}
			finished, err := artifact.FinishClass("PacketValues.class", input)
			if err != nil {
				t.Fatal("complete composed field and method obligations", err)
			}
			wantFields, _ := sourceFieldIdentities(mustParseLowering(t, original), nil)
			gotFields, _ := sourceFieldIdentities(mustParseLowering(t, finished), nil)
			for key, flags := range wantFields {
				if gotFields[key] != flags {
					t.Fatal("final field metadata drift", key)
				}
			}
			if again, err := artifact.FinishClass("PacketValues.class", finished); err != nil || !bytes.Equal(again, finished) {
				t.Fatal("composed completion not idempotent", err)
			}
		})
	}
}
