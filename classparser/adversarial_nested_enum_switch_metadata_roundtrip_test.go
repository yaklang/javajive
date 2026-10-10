package javaclassparser

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// A switch over a separately emitted member enum references that enum's original
// declaration rows without acquiring lexical ownership of those declarations.
// Check the real compiler packet and metadata, then compare unchanged JVM
// overflow/null/capture behavior, original ABI and helper instruction identity.
func TestAdversarialNestedEnumTablePreservesCapturedLexicalFamily(t *testing.T) {
	for _, root := range []string{"NestedTableOwner", "OtherTableScope"} {
		for _, deep := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deep=%v", root, deep), func(t *testing.T) {
				sources := nativeNestedEnumReferenceSources(root, deep)
				testNativePrivateSetterCompiledFixture(t, root, "SwitchFamilyDriver", "75:enum:lexical:captured:case:null:overflow\n", func(t *testing.T, debug string) map[string][]byte {
					files := nativeCompileSourceReleaseClasses(t, sources, debug, "8")
					count := 0
					for _, raw := range files {
						obj, err := Parse(raw)
						if err != nil {
							t.Fatal(err)
						}
						packet := nativeEnumSwitchTableProof(obj, nil)
						if packet == nil {
							continue
						}
						count++
						rows := nativeNestedEnumMetadataShape(t, obj)
						required := 2
						if deep {
							required = 3
						}
						if len(rows) != required {
							t.Fatalf("original enum table declarations: %v", rows)
						}
					}
					if count != 1 {
						t.Fatal("original compiler helper inventory", count)
					}
					return files
				}, func(t *testing.T, name string, original, rebuilt []byte) {
					a, err := Parse(original)
					if err != nil {
						t.Fatal(err)
					}
					if a.AccessFlags&0x1000 == 0 {
						return
					}
					b, err := Parse(rebuilt)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(nativeEnumSwitchOriginalPacketShape(t, a), nativeEnumSwitchOriginalPacketShape(t, b)) {
						t.Fatal("original executable helper packet changed", name)
					}
					if !reflect.DeepEqual(nativeNestedEnumMetadataShape(t, a), nativeNestedEnumMetadataShape(t, b)) {
						t.Fatal("original referenced declaration metadata changed", name)
					}
				})
			})
		}
	}
}

// Independent normalized compiler metadata oracle; never calls the source proof.
func nativeNestedEnumMetadataShape(t *testing.T, obj *ClassObject) []string {
	t.Helper()
	var rows []string
	for _, a := range obj.Attributes {
		table, ok := a.(*InnerClassesAttribute)
		if !ok {
			continue
		}
		if table == nil {
			t.Fatal("nil original metadata")
		}
		for _, row := range table.Classes {
			if row == nil {
				t.Fatal("nil original row")
			}
			inner, ok := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
			if !ok {
				t.Fatal("invalid inner reference")
			}
			outer, local := "", ""
			if row.OuterClassInfoIndex != 0 {
				var valid bool
				outer, valid = sourceBridgeClassName(obj, row.OuterClassInfoIndex)
				if !valid {
					t.Fatal("invalid outer reference")
				}
			}
			if row.InnerNameIndex != 0 {
				var valid bool
				local, valid = sourceBridgeUTF8(obj, row.InnerNameIndex)
				if !valid {
					t.Fatal("invalid local name")
				}
			}
			rows = append(rows, fmt.Sprintf("%s:%s:%s:%x", inner, outer, local, row.InnerClassAccessFlags))
		}
	}
	sort.Strings(rows)
	return rows
}

func nativeNestedEnumReferenceSources(root string, deep bool) map[string]string {
	sources := nativeEnumSwitchFamilySources(root)
	host := "EnumDomain" + root
	name, decl := host+".Mode", "public enum Mode{A,B,C}"
	if deep {
		name, decl = host+".Scope.Mode", "public static class Scope{public enum Mode{A,B,C}}"
	}
	sources[root+".java"] = strings.ReplaceAll(sources[root+".java"], root+"Mode", name)
	delete(sources, root+"Mode.java")
	sources[host+".java"] = "public class " + host + "{" + decl + "}"
	return sources
}
