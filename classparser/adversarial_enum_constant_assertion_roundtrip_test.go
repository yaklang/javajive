package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func enumConstantAssertionFixture(enabled bool) (string, string) {
	fixture := strings.Replace(enumAssertionInitializationFixture, "XOR(2){long apply(long x,int width){return", "XOR(2){long apply(long x,int width){assert EnumAssertionEffects.condition(width):EnumAssertionEffects.message();return", 1)
	fixture = strings.Replace(fixture, `enabled?(fails?"CM":"C"):""`, `enabled?(fails?"CM":mode==choices[1]?"CC":"C"):""`, 1)
	expected := "120:enum:assertions:false:word:identity:initialization\n"
	if enabled {
		fixture = strings.Replace(fixture, "boolean enabled=false;", "boolean enabled=true;", 1)
		expected = "120:enum:assertions:true:word:identity:initialization\n"
	}
	return fixture, expected
}
func TestAdversarialEnumConstantDisabledAssertionRoundTrip(t *testing.T) {
	fixture, expected := enumConstantAssertionFixture(false)
	testSourceTargetReleaseFamilyFixture(t, fixture, "EnumAssertionOwner", "EnumAssertionDriver", expected, "8", []int{8, 11})
}
func TestAdversarialEnumConstantEnabledAssertionRoundTrip(t *testing.T) {
	fixture, expected := enumConstantAssertionFixture(true)
	testSourceTargetReleaseFamilyFixture(t, fixture, "EnumAssertionOwner", "EnumAssertionDriver", expected, "8", []int{8, 11})
}

// Reproduce the legal Java-8 compiler profile independently of library bytes:
// static/final anonymous enum rows and absent optional parameter attributes.
func TestAdversarialJava8EnumConstantCompilerMetadataRoundTrip(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		fixture, expected := enumConstantAssertionFixture(enabled)
		for _, selfFlags := range []uint16{0x4000, 0x4008, 0x4018} {
			for _, owner := range []string{"EnumAssertionOwner", "ReboundEnumAssertionOwner"} {
				t.Run(fmt.Sprintf("%s/%x/%t", owner, selfFlags, enabled), func(t *testing.T) {
					source := strings.ReplaceAll(fixture, "EnumAssertionOwner", owner)
					testNativePrivateSetterCompiledFixture(t, owner, "EnumAssertionDriver", expected, func(t *testing.T, debug string) map[string][]byte {
						files := nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
						objects := map[string]*ClassObject{}
						bodies := map[string]bool{}
						for name, raw := range files {
							obj, err := Parse(append([]byte(nil), raw...))
							if err != nil {
								t.Fatal(err)
							}
							objects[name] = obj
							if obj.AccessFlags == 0x4030 && obj.GetSupperClassName() == owner+"$Mode" {
								bodies[obj.GetClassName()] = true
							}
						}
						if len(bodies) != 2 {
							t.Fatal("original enum allocation classes")
						}
						for name, obj := range objects {
							for _, a := range obj.Attributes {
								if table, ok := a.(*InnerClassesAttribute); ok {
									for _, row := range table.Classes {
										n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
										if bodies[n] {
											row.InnerClassAccessFlags = selfFlags
										}
									}
								}
							}
							if bodies[obj.GetClassName()] {
								for _, m := range obj.Methods {
									n, _ := sourceBridgeUTF8(obj, m.NameIndex)
									if n != "<init>" {
										continue
									}
									attrs := []AttributeInfo{}
									for _, a := range m.Attributes {
										if p, ok := a.(*UnparsedAttribute); ok && p.Name == "MethodParameters" {
											continue
										}
										attrs = append(attrs, a)
									}
									m.Attributes = attrs
								}
							}
							files[name] = obj.Bytes()
						}
						return files
					})
				})
			}
		}

	}
}
