package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

// invokeResultFixture builds String run() { return String.valueOf(7); }. The
// move-result destination v0 holds no earlier value, so its category can only
// come from the move-result definition itself.
func invokeResultFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Ref;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: "I"},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "[Ljava/lang/String;"},
		},
		Protos: []model.Proto{
			{ReturnType: "Ljava/lang/String;"},
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{2}},
			{ReturnType: "Ljava/lang/String;", Parameters: []string{"I"}, ParameterTypes: []uint32{0}},
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/String;", Name: "valueOf", ProtoIndex: 2},
			{Class: owner, Name: "run", ProtoIndex: 0},
			{Class: owner, Name: "main", ProtoIndex: 1},
		},
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1],
				Code: &model.Code{RegistersSize: 2, OutsSize: 1, Instructions: []uint16{
					0x7112,                 // const/4 v1, #7
					0x1071, 0x0000, 0x0001, // invoke-static {v1}, String.valueOf(I)String
					0x000c, // move-result-object v0
					0x0011, // return-object v0
				}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[2],
				Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 0, Instructions: []uint16{
					0x0071, 0x0001, 0x0000, // invoke-static {}, Ref.run()String
					0x000c, // move-result-object v0
					0x000e, // return-void
				}},
			},
		},
	}
	return file, class
}

func TestInvokeResultUsesMoveResultCategory(t *testing.T) {
	file, class := invokeResultFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	var code []byte
	for _, method := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(method.NameIndex)).Value == "run" {
			code = method.Attributes[0].(*javaclassparser.CodeAttribute).Code
		}
	}
	if code == nil {
		t.Fatal("run method has no code")
	}
	// invokestatic is a three-byte instruction; the result must be stored with
	// astore, not istore.
	found := false
	for index := 0; index+4 <= len(code); index++ {
		if code[index] != 0xb8 {
			continue
		}
		store := code[index+3]
		if store != 0x3a && (store < 0x4b || store > 0x4e) {
			t.Fatalf("invoke result stored with opcode 0x%02x, want astore", store)
		}
		found = true
	}
	if !found {
		t.Fatal("no invokestatic in generated code")
	}

	java, javaErr := exec.LookPath("java")
	if javaErr != nil {
		t.Skip("java is unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "Ref.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "dex.Ref").CombinedOutput(); err != nil {
		t.Fatalf("java verification: %v\n%s", err, output)
	}
}
