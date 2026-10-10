package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are valid JVM enums whose hidden constructor operands do not match
// Java's implicit names/ordinals. Compilation alone would silently fix the
// original program rather than reconstruct it. Validate the original first.
func TestNativeEnumRefusesChangedHiddenNameAndOrdinal(t *testing.T) {
	_, java := t04Tools(t)
	for _, variant := range []string{"name", "ordinal"} {
		expected, condition := "name:RIGHT:0", "!CounterChoice.LEFT.name().equals(\"RIGHT\")||CounterChoice.LEFT.ordinal()!=0"
		if variant == "ordinal" {
			expected, condition = "ordinal:LEFT:1", "!CounterChoice.LEFT.name().equals(\"LEFT\")||CounterChoice.LEFT.ordinal()!=1"
		}
		source := `enum CounterChoice{LEFT,RIGHT;}class CounterDriver{public static void main(String[]args){if(` + condition + `)throw new AssertionError("original hidden inputs");System.out.println("` + expected + `");}}`
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(variant+"/"+debug, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, source, debug)
				obj, err := Parse(files["CounterChoice.class"])
				if err != nil {
					t.Fatal(err)
				}
				modified := false
				for _, method := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, method.NameIndex)
					if n != "<clinit>" {
						continue
					}
					ops, known := nativeEnumMethodOps(obj, method, nil)
					if !known || len(ops) < 12 {
						t.Fatal("original initializer packets")
					}
					for _, attr := range method.Attributes {
						if code, ok := attr.(*CodeAttribute); ok {
							if variant == "name" {
								index, known := nativeEnumCPIndex(ops[8])
								if !known || !nativeEnumOpcode(ops[2], core.OP_LDC) || index > 255 {
									t.Fatal("original string literal")
								}
								code.Code[int(ops[2].CurrentOffset)+1] = byte(index)
							} else {
								if !nativeEnumOpcode(ops[3], core.OP_ICONST_0) {
									t.Fatal("original ordinal literal")
								}
								code.Code[int(ops[3].CurrentOffset)] = byte(core.OP_ICONST_1)
							}
							modified = true
						}
					}
				}
				if !modified {
					t.Fatal("original initializer absent")
				}
				files["CounterChoice.class"] = obj.Bytes()
				original := t.TempDir()
				for n, raw := range files {
					if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if got := t04RunJava(t, java, original, "CounterDriver"); got != expected+"\n" {
					t.Fatalf("original JVM=%q", got)
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
						raw, err := z.ReadFile("CounterChoice.class")
						if err != nil || !strings.Contains(string(raw), "enum regeneration") || !strings.Contains(string(raw), "decompile dump failed") {
							t.Fatalf("changed hidden inputs must not be silently regenerated:%v\n%s", err, raw)
						}
					})
				}
			})
		}
	}
}
