package javaclassparser

import (
	"os"
	"path/filepath"
	"testing"
)

// JVM constructors may write a final field more than once. Java source cannot
// do so. Execute the verifier-valid original before requiring refusal; a
// malformed classfile alone would not establish this semantic boundary.
func TestNativeAnonymousInitializerRepeatedFinalWritesRefuseProjection(t *testing.T) {
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, nativeAnonymousRepeatedStores, debug)
			obj, e := Parse(files["RepeatedInitOwner$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			changed := 0
			for _, field := range obj.Fields {
				name, _ := sourceBridgeUTF8(obj, field.NameIndex)
				if name == "stage" {
					field.AccessFlags |= 0x10
					changed++
				}
			}
			if changed != 1 {
				t.Fatal("one actual repeated field")
			}
			files["RepeatedInitOwner$1.class"] = obj.Bytes()
			dir := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(dir, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if got := t04RunJava(t, java, dir, "RepeatedInitDriver"); got != "6:repeated:stores:partial:identity:order\n" {
				t.Fatalf("valid original JVM=%q", got)
			}
			parsed, e := Parse(files["RepeatedInitOwner$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			if child := nativeAnonymousConstructor(parsed, "RepeatedInitOwner", "make", nil); child != nil {
				t.Fatal("repeated final write cannot obey Java definite assignment")
			}
		})
	}
}
