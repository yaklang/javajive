package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// Hidden name/ordinal identity alone does not establish initialization order.
// The two complete allocation/store packets are swapped without changing
// fields, constructor arguments or the values array. The original JVM proves
// that the source compiler's implicit declaration order would change effects.
func TestNativeEnumRefusesChangedOriginalConstantInitializationOrder(t *testing.T) {
	const fixture = `class InitEffects{static int count;}enum OrderChoice{LEFT,RIGHT;final int seen;OrderChoice(){seen=++InitEffects.count;}}class OrderDriver{public static void main(String[]args){if(OrderChoice.LEFT.seen!=2||OrderChoice.RIGHT.seen!=1||OrderChoice.LEFT.ordinal()!=0||OrderChoice.RIGHT.ordinal()!=1||OrderChoice.values()[0]!=OrderChoice.LEFT||OrderChoice.values()[1]!=OrderChoice.RIGHT)throw new AssertionError("original construction order");System.out.println("order:effects:ordinal:values");}}`
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			obj, err := Parse(files["OrderChoice.class"])
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n != "<clinit>" {
					continue
				}
				ops, known := nativeEnumMethodOps(obj, m, nil)
				if !known || len(ops) < 13 || !nativeEnumOpcode(ops[0], core.OP_NEW) || !nativeEnumOpcode(ops[6], core.OP_NEW) {
					t.Fatal("original constructor packets")
				}
				split, end := int(ops[6].CurrentOffset), int(ops[12].CurrentOffset)
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code := append([]byte(nil), c.Code[split:end]...)
						code = append(code, c.Code[:split]...)
						code = append(code, c.Code[end:]...)
						c.Code, c.Attributes = code, nil
						c.AttrLen = uint32(12 + len(code))
						found = true
					}
				}
			}
			if !found {
				t.Fatal("missing initializer")
			}
			files["OrderChoice.class"] = obj.Bytes()
			original := t.TempDir()
			for n, raw := range files {
				if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "OrderDriver"); got != "order:effects:ordinal:values\n" {
				t.Fatalf("original oracle %q", got)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					defer z.Close()
					raw, err := z.ReadFile("OrderChoice.class")
					if err != nil || !strings.Contains(string(raw), "enum regeneration") || !strings.Contains(string(raw), "decompile dump failed") {
						t.Fatalf("changed order must be explicitly refused:%v\n%s", err, raw)
					}
				})
			}
		})
	}
}
