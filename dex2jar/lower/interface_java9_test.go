package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

func TestLowerInterfaceJava9PrivateMethodsVerifyAndDecompile(t *testing.T) {
	const owner = "Ldex/j9/Java9Face;"
	file := &model.File{
		Entry:  "classes.dex",
		Types:  []model.Type{{Descriptor: "I"}},
		Protos: []model.Proto{{ReturnType: "I"}},
		Methods: []model.MethodID{
			{Class: owner, Name: "contextual$lambda$0", ProtoIndex: 0}, // 0: private static ()I
			{Class: owner, Name: "helperInst", ProtoIndex: 0},          // 1: private ()I
			{Class: owner, Name: "callPrivate", ProtoIndex: 0},         // 2: default ()I
		},
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic | dexInterface | dexAbstract,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPrivate | dexStatic,
				Method:      file.Methods[0],
				Code: &model.Code{RegistersSize: 1, Instructions: []uint16{
					0x5012, // const/4 v0, #5
					0x000f, // return v0
				}},
			},
			{
				AccessFlags: dexPrivate,
				Method:      file.Methods[1],
				Code: &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{
					0x7012, // const/4 v0, #7
					0x000f, // return v0
				}},
			},
		},
		VirtualMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic,
				Method:      file.Methods[2],
				Code: &model.Code{RegistersSize: 3, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x1070, 0x0001, 0x0002, // invoke-direct {v2}, Java9Face.helperInst()I
					0x000a,                 // move-result v0
					0x0071, 0x0000, 0x0000, // invoke-static {}, Java9Face.contextual$lambda$0()I
					0x000a, // move-result v1 (Wait: 0x010a -> v1!)
					0x000e, // placeholder replaced below
				}},
			},
		},
	}
	// Encode callPrivate cleanly:
	// v2 = this
	// 0..2: invoke-direct {v2}, helperInst() -> 0x1070, 0x0001, 0x0002
	// 3: move-result v0 -> 0x000a
	// 4..6: invoke-static {}, contextual$lambda$0() -> 0x0071, 0x0000, 0x0000
	// 7: move-result v1 -> 0x010a
	// 8..9: add-int v0, v0, v1 -> 0x0090, 0x0100
	// 10: return v0 -> 0x000f
	class.VirtualMethods[0].Code.Instructions = []uint16{
		0x1070, 0x0001, 0x0002,
		0x000a,
		0x0071, 0x0000, 0x0000,
		0x010a,
		0x0090, 0x0100,
		0x000f,
	}

	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("Parse generated Java 9 interface: %v", err)
	}
	if parsed.MajorVersion != 53 {
		t.Fatalf("class major version = %d, want 53", parsed.MajorVersion)
	}

	// Stage B decompilation must succeed on Java 9 private interface methods.
	dec, err := javaclassparser.DecompileWithOptions(raw, javaclassparser.DecompileOptions{
		Mode:           javaclassparser.Compatibility,
		EnableShadowIR: true,
	})
	if err != nil {
		t.Fatalf("DecompileWithOptions: %v", err)
	}
	if !strings.Contains(dec.Source, "contextual$lambda$0") || !strings.Contains(dec.Source, "helperInst") {
		t.Fatalf("decompiled interface missing private methods:\n%s", dec.Source)
	}

	// Real JVM -Xverify:all + execution check.
	if javac, err1 := exec.LookPath("javac"); err1 == nil {
		if java, err2 := exec.LookPath("java"); err2 == nil {
			dir := t.TempDir()
			outPath := filepath.Join(dir, "dex", "j9", "Java9Face.class")
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(outPath, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			driver := filepath.Join(dir, "Driver.java")
			source := `public class Driver {
    public static void main(String[] args) {
        dex.j9.Java9Face f = new dex.j9.Java9Face() {};
        if (f.callPrivate() != 12) throw new AssertionError("callPrivate=" + f.callPrivate());
    }
}`
			if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
				t.Fatalf("javac driver: %v\n%s", err, output)
			}
			if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
				t.Fatalf("java -Xverify:all failed: %v\n%s", err, output)
			}
		}
	}

	// Illegal private interface shapes must still be rejected in Strict mode.
	badCases := []struct {
		name  string
		flags uint32
		code  *model.Code
		want  string
	}{
		{
			name:  "private abstract",
			flags: dexPrivate | dexAbstract,
			want:  "abstract method has incompatible access flags",
		},
		{
			name:  "public and private",
			flags: dexPublic | dexPrivate,
			code:  &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
			want:  "multiple visibility flags are set",
		},
		{
			name:  "private without code",
			flags: dexPrivate,
			want:  "concrete method has no code",
		},
		{
			name:  "protected on Java 9 interface",
			flags: dexProtected | dexAbstract,
			want:  "Java 9 interface method has illegal access flags",
		},
	}
	for _, tc := range badCases {
		t.Run(tc.name, func(t *testing.T) {
			f := &model.File{
				Entry:   "classes.dex",
				Protos:  []model.Proto{{ReturnType: "V"}},
				Methods: []model.MethodID{{Class: owner, Name: "bad", ProtoIndex: 0}, {Class: owner, Name: "priv", ProtoIndex: 0}},
			}
			c := &model.Class{
				Descriptor:  owner,
				Superclass:  "Ljava/lang/Object;",
				AccessFlags: dexPublic | dexInterface | dexAbstract,
				DirectMethods: []model.EncodedMethod{
					{
						AccessFlags: dexPrivate,
						Method:      f.Methods[1],
						Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
					},
					{
						AccessFlags: tc.flags,
						Method:      f.Methods[0],
						Code:        tc.code,
					},
				},
			}
			if _, err := Class(f, c); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Class error = %v, want %q", err, tc.want)
			}
		})
	}
}
