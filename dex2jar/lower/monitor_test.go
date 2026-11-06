package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

// monitorFixture builds
//
//	static int sync(int value, Object lock) {
//	    synchronized (lock) { return value + 1; }
//	}
//
// including the catch-all handler that releases the monitor and rethrows.
func monitorFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Monitor;"
	protos := []model.Proto{{ReturnType: "I", Parameters: []string{"I", "Ljava/lang/Object;"}, ParameterTypes: []uint32{0, 1}}}
	methods := []model.MethodID{{Class: owner, Name: "sync", ProtoIndex: 0}}
	file := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: "I"}, {Descriptor: "Ljava/lang/Object;"}},
		Protos:  protos,
		Methods: methods,
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexStatic,
			Method:      methods[0],
			Code: &model.Code{
				RegistersSize: 3, InsSize: 2,
				Instructions: []uint16{
					0x021d,         // monitor-enter v2
					0x01d8, 0x0101, // add-int/lit8 v1, v1, #1
					0x021e, // monitor-exit v2
					0x010f, // return v1
					0x000d, // move-exception v0
					0x021e, // monitor-exit v2
					0x0027, // throw v0
				},
				Tries: []model.Try{{
					StartAddress:     1,
					InstructionCount: 2,
					Handler:          model.CatchHandler{CatchAllAddress: func() *uint32 { value := uint32(5); return &value }()},
				}},
			},
		}},
	}
	return file, class
}

func TestLowerMonitorsVerifyAndExecute(t *testing.T) {
	file, class := monitorFixture()
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
	path := filepath.Join(dir, "dex", "Monitor.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        Object lock = new Object();
        if (dex.Monitor.sync(1, lock) != 2) throw new AssertionError("synchronized body");
        if (dex.Monitor.sync(-5, lock) != -4) throw new AssertionError("synchronized body 2");
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac driver: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("monitor driver execution: %v\n%s", err, output)
	}
}
