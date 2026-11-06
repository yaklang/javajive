package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

// tryCatchFixture builds the d8 shape for
//
//	static int safeDivide(int a, int b) { try { return a / b; } catch (ArithmeticException e) { return -1; } }
func tryCatchFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Guard;"
	protos := []model.Proto{
		{ReturnType: "I", Parameters: []string{"I", "I"}, ParameterTypes: []uint32{0, 0}},
	}
	methods := []model.MethodID{{Class: owner, Name: "safeDivide", ProtoIndex: 0}}
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
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexStatic,
			Method:      methods[0],
			Code: &model.Code{
				RegistersSize: 3, InsSize: 2,
				Instructions: []uint16{
					0x0093, 0x0201, // div-int v0, v1, v2
					0x000f, // return v0
					0x000d, // move-exception v0
					0xf012, // const/4 v0, #-1
					0x000f, // return v0
				},
				Tries: []model.Try{{
					StartAddress:     0,
					InstructionCount: 2,
					Handler: model.CatchHandler{Handlers: []model.TypedHandler{{
						Type:    "Ljava/lang/ArithmeticException;",
						Address: 3,
					}}},
				}},
			},
		}},
	}
	return file, class
}

func TestLowerTryCatchVerifiesAndExecutes(t *testing.T) {
	file, class := tryCatchFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	var code *javaclassparser.CodeAttribute
	for _, method := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(method.NameIndex)).Value != "safeDivide" {
			continue
		}
		code = method.Attributes[0].(*javaclassparser.CodeAttribute)
	}
	if code == nil || len(code.ExceptionTable) != 1 {
		t.Fatalf("exception table = %#v", code)
	}
	entry := code.ExceptionTable[0]
	// JVM pcs include the parameter prologue, so only the ordering and the
	// catch type are asserted here; behaviour below proves the range.
	if entry.StartPc >= entry.EndPc || entry.HandlerPc < entry.EndPc || entry.CatchType == 0 {
		t.Fatalf("exception table entry = %#v", entry)
	}

	javac, javacErr := exec.LookPath("javac")
	java, javaErr := exec.LookPath("java")
	if javacErr != nil || javaErr != nil {
		t.Skip("javac/java unavailable")
	}
	dir := t.TempDir()
	classPath := filepath.Join(dir, "dex", "Guard.class")
	if err := os.MkdirAll(filepath.Dir(classPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(classPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        if (dex.Guard.safeDivide(6, 3) != 2) throw new AssertionError("normal path");
        if (dex.Guard.safeDivide(1, 0) != -1) throw new AssertionError("catch path");
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac driver: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("driver execution: %v\n%s", err, output)
	}
}

// catchAllFixture builds a finally-style catch-all around a division by zero.
func catchAllFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Finally;"
	protos := []model.Proto{{ReturnType: "I"}}
	methods := []model.MethodID{{Class: owner, Name: "run", ProtoIndex: 0}}
	file := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: "I"}},
		Protos:  protos,
		Methods: methods,
	}
	catchAll := uint32(4)
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexStatic,
			Method:      methods[0],
			Code: &model.Code{
				RegistersSize: 2, OutsSize: 1,
				Instructions: []uint16{
					0x0012,         // const/4 v0, #0
					0x0093, 0x0000, // div-int v0, v0, v0
					0x000f, // return v0
					0x010d, // move-exception v1
					0xf012, // const/4 v0, #-1
					0x000f, // return v0
				},
				Tries: []model.Try{{
					StartAddress:     0,
					InstructionCount: 3,
					Handler:          model.CatchHandler{CatchAllAddress: &catchAll},
				}},
			},
		}},
	}
	return file, class
}

func TestLowerCatchAllHandlerVerifiesAndExecutes(t *testing.T) {
	file, class := catchAllFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	var code *javaclassparser.CodeAttribute
	for _, method := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(method.NameIndex)).Value == "run" {
			code = method.Attributes[0].(*javaclassparser.CodeAttribute)
		}
	}
	if code == nil || len(code.ExceptionTable) != 1 || code.ExceptionTable[0].CatchType != 0 {
		t.Fatalf("catch-all entry = %#v", code)
	}
	javac, javacErr := exec.LookPath("javac")
	java, javaErr := exec.LookPath("java")
	if javacErr != nil || javaErr != nil {
		t.Skip("javac/java unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "Finally.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        if (dex.Finally.run() != -1) throw new AssertionError("catch-all path did not run");
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac driver: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("driver execution: %v\n%s", err, output)
	}
}

// discardHandlerFixture builds
//
//	static int safeDivide(int v) { try { return 10 / v; } catch (ArithmeticException e) { return -1; } }
//
// with a handler that never binds the exception. d8 emits exactly this shape
// when the catch block does not read the caught value: the handler starts with
// the catch body instead of move-exception.
func discardHandlerFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Discard;"
	protos := []model.Proto{{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{0}}}
	methods := []model.MethodID{{Class: owner, Name: "safeDivide", ProtoIndex: 0}}
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
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexStatic,
			Method:      methods[0],
			Code: &model.Code{
				RegistersSize: 2, InsSize: 1,
				Instructions: []uint16{
					0x0013, 0x000a, // const/16 v0, #10
					0x0093, 0x0100, // div-int v0, v0, v1
					0x000f, // return v0
					0xf012, // const/4 v0, #-1
					0x000f, // return v0
				},
				Tries: []model.Try{{
					StartAddress:     0,
					InstructionCount: 4,
					Handler: model.CatchHandler{Handlers: []model.TypedHandler{{
						Type:    "Ljava/lang/ArithmeticException;",
						Address: 5,
					}}},
				}},
			},
		}},
	}
	return file, class
}

func TestHandlerWithoutMoveExceptionDiscardsCaughtValue(t *testing.T) {
	file, class := discardHandlerFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	var code *javaclassparser.CodeAttribute
	for _, method := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(method.NameIndex)).Value == "safeDivide" {
			code = method.Attributes[0].(*javaclassparser.CodeAttribute)
		}
	}
	if code == nil || len(code.ExceptionTable) != 1 {
		t.Fatalf("exception table = %#v", code)
	}
	handlerPC := int(code.ExceptionTable[0].HandlerPc)
	if handlerPC >= len(code.Code) || code.Code[handlerPC] != 0x57 {
		t.Fatalf("handler pc %d does not start with pop: % x", handlerPC, code.Code)
	}

	javac, javacErr := exec.LookPath("javac")
	java, javaErr := exec.LookPath("java")
	if javacErr != nil || javaErr != nil {
		t.Skip("javac/java unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "Discard.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        if (dex.Discard.safeDivide(10) != 1) throw new AssertionError("normal path");
        if (dex.Discard.safeDivide(0) != -1) throw new AssertionError("discarding handler path");
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac driver: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("driver execution: %v\n%s", err, output)
	}
}
