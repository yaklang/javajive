package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

// switchFixture builds two methods that use DEX packed and sparse switches.
// DEX switches fall through on an unmatched value, so the instruction after the
// switch is the default case.
func switchFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Switches;"
	protos := []model.Proto{
		{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{0}},
	}
	methods := []model.MethodID{
		{Class: owner, Name: "packed", ProtoIndex: 0},
		{Class: owner, Name: "sparse", ProtoIndex: 0},
	}
	file := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: "I"}},
		Protos:  protos,
		Methods: methods,
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[0],
				Code: &model.Code{RegistersSize: 2, InsSize: 1, Instructions: []uint16{
					0x012b, 0x000e, 0x0000, // packed-switch v1, payload@14
					0xf012,         // const/4 v0, #-1
					0x000f,         // return v0
					0x0013, 0x000a, // const/16 v0, #10
					0x000f,         // return v0
					0x0013, 0x0014, // const/16 v0, #20
					0x000f,
					0x0013, 0x001e, // const/16 v0, #30
					0x000f,
					0x0100, 0x0003, // packed payload: 3 cases
					0x000a, 0x0000, // first key 10
					0x0005, 0x0000, // key 10 -> offset 5
					0x0008, 0x0000, // key 11 -> offset 8
					0x000b, 0x0000, // key 12 -> offset 11
				}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[1],
				Code: &model.Code{RegistersSize: 2, InsSize: 1, Instructions: []uint16{
					0x012c, 0x0009, 0x0000, // sparse-switch v1, payload@9
					0x0012, // const/4 v0, #0
					0x000f, // return v0
					0x1012, // const/4 v0, #1
					0x000f,
					0x2012, // const/4 v0, #2
					0x000f,
					0x0200, 0x0002, // sparse payload: 2 cases
					0x0005, 0x0000, // key 5
					0x0005, 0x0000, // target offset 5
					0x0064, 0x0000, // key 100
					0x0007, 0x0000, // target offset 7
				}},
			},
		},
	}
	return file, class
}

func TestLowerSwitchesVerifyAndExecute(t *testing.T) {
	file, class := switchFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	if _, err := javaclassparser.Parse(raw); err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	javac, javacErr := exec.LookPath("javac")
	java, javaErr := exec.LookPath("java")
	if javacErr != nil || javaErr != nil {
		t.Skip("javac/java unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "Switches.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        if (dex.Switches.packed(10) != 10) throw new AssertionError("case 10");
        if (dex.Switches.packed(11) != 20) throw new AssertionError("case 11");
        if (dex.Switches.packed(12) != 30) throw new AssertionError("case 12");
        if (dex.Switches.packed(0) != -1) throw new AssertionError("default");
        if (dex.Switches.sparse(5) != 1) throw new AssertionError("sparse 5");
        if (dex.Switches.sparse(100) != 2) throw new AssertionError("sparse 100");
        if (dex.Switches.sparse(7) != 0) throw new AssertionError("sparse default");
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac driver: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("switch driver execution: %v\n%s", err, output)
	}
}
