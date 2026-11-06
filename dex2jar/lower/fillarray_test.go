package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

// fillArrayFixture builds array literals for int[], byte[] and double[]. Each
// method allocates the array, fills it from a fill-array-data payload, and
// returns one element.
func fillArrayFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Fill;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: "[I"}, {Descriptor: "[B"}, {Descriptor: "[D"}, {Descriptor: "I"},
		},
		Protos: []model.Proto{
			{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{3}},
			{ReturnType: "B", Parameters: []string{"I"}, ParameterTypes: []uint32{3}},
			{ReturnType: "D", Parameters: []string{"I"}, ParameterTypes: []uint32{3}},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "intAt", ProtoIndex: 0},
			{Class: owner, Name: "byteAt", ProtoIndex: 1},
			{Class: owner, Name: "doubleAt", ProtoIndex: 2},
		},
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[0],
				Code: &model.Code{RegistersSize: 3, InsSize: 1, Instructions: []uint16{
					0x3112,         // const/4 v1, #3
					0x1023, 0x0000, // new-array v0, v1, [I
					0x0026, 0x0006, 0x0000, // fill-array-data v0, payload@9
					0x0144, 0x0200, // aget v1, v0, v2
					0x010f,                         // return v1
					0x0300, 0x0004, 0x0003, 0x0000, // fill payload: width 4, size 3
					0x000a, 0x0000, // 10
					0x0014, 0x0000, // 20
					0x001e, 0x0000, // 30
				}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1],
				Code: &model.Code{RegistersSize: 3, InsSize: 1, Instructions: []uint16{
					0x3112,         // const/4 v1, #3
					0x1023, 0x0001, // new-array v0, v1, [B
					0x0026, 0x0006, 0x0000, // fill-array-data v0, payload@9
					0x0148, 0x0200, // aget-byte v1, v0, v2
					0x010f,                         // return v1
					0x0300, 0x0001, 0x0003, 0x0000, // fill payload: width 1, size 3
					0x0201, 0x0003, // bytes 1, 2, 3
				}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[2],
				Code: &model.Code{RegistersSize: 5, InsSize: 1, Instructions: []uint16{
					0x3312,         // const/4 v3, #2
					0x3223, 0x0002, // new-array v2, v3, [D
					0x0226, 0x0006, 0x0000, // fill-array-data v2, payload@9
					0x0045, 0x0402, // aget-wide v0, v2, v4
					0x0010,                         // return-wide v0
					0x0300, 0x0008, 0x0002, 0x0000, // fill payload: width 8, size 2
					0x0000, 0x0000, 0x0000, 0x3ff8, // 1.5
					0x0000, 0x0000, 0x0000, 0x4004, // 2.5
				}},
			},
		},
	}
	return file, class
}

func TestLowerFillArrayDataVerifyAndExecute(t *testing.T) {
	file, class := fillArrayFixture()
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
	path := filepath.Join(dir, "dex", "Fill.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        if (dex.Fill.intAt(0) != 10) throw new AssertionError("int element 0");
        if (dex.Fill.intAt(2) != 30) throw new AssertionError("int element 2");
        if (dex.Fill.byteAt(1) != 2) throw new AssertionError("byte element");
        if (dex.Fill.doubleAt(0) != 1.5) throw new AssertionError("double element 0");
        if (dex.Fill.doubleAt(1) != 2.5) throw new AssertionError("double element 1");
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac driver: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("fill-array driver execution: %v\n%s", err, output)
	}
}
