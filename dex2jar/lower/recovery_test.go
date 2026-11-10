package lower

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

func recoveryFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Recovered;"
	file := &model.File{
		Entry:  "classes.dex",
		Protos: []model.Proto{{ReturnType: "V"}, {ReturnType: "I"}},
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexConstructor,
				Method:      model.MethodID{Class: owner, Name: "<init>", ProtoIndex: 0},
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      model.MethodID{Class: owner, Name: "bad", ProtoIndex: 1},
				Code:        &model.Code{Instructions: []uint16{0x00ff}},
			},
		},
	}
	return file, class
}

func TestRecoveryPayloadBoundsRemainReadable(t *testing.T) {
	file, class := recoveryFixture()
	records := make([]Recovery, recoveryMaxRecords+10)
	for index := range records {
		records[index] = Recovery{Code: "method.body_stubbed", Action: "stub_method", Member: fmt.Sprintf("method%03d", index), Descriptor: "()V", Message: strings.Repeat("reason", 80)}
	}
	payload, err := recoveryPayload(file, class, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > recoveryMaxPayload {
		t.Fatalf("payload length = %d", len(payload))
	}
	var decoded []RecoveryRecord
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) > recoveryMaxRecords || decoded[len(decoded)-1].Code != "recovery.records_truncated" {
		t.Fatalf("records = %#v", decoded)
	}
}

func TestRecoverEmitsFreshDeclarationShell(t *testing.T) {
	file, class := recoveryFixture()
	result, err := ClassWithPolicy(file, class, ClassOptions{}, Recover)
	if err != nil {
		t.Fatalf("ClassWithPolicy Recover: %v", err)
	}
	if !result.ClassStubbed || len(result.Recoveries) < 3 || len(result.StubbedMethods) != 2 {
		t.Fatalf("result = %#v", result)
	}
	parsed, err := javaclassparser.Parse(result.Bytes)
	if err != nil {
		t.Fatalf("parse recovered class: %v", err)
	}
	if parsed.GetClassName() != "dex/Recovered" || len(parsed.Methods) != 2 {
		t.Fatalf("class=%q methods=%d", parsed.GetClassName(), len(parsed.Methods))
	}
	source, err := parsed.Dump()
	if err != nil {
		t.Fatalf("dump recovered class: %v", err)
	}
	if !strings.HasPrefix(source, "// javajive recovery:") || !strings.Contains(source, "class.body_recovery") || !strings.Contains(source, "method.body_stubbed") {
		t.Fatalf("recovery source:\n%s", source)
	}
	if !bytes.Contains(result.Bytes, []byte(RecoveryAttributeName)) {
		t.Fatal("recovered class has no JavaJiveRecovery attribute")
	}
}

func TestRecoverFailsClosedForIncompatibleWideInitializer(t *testing.T) {
	const owner = "Ldex/RecoveredWideStatic;"
	file := &model.File{Entry: "classes.dex"}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic,
		StaticFields: []model.EncodedField{{
			AccessFlags: dexPublic | dexStatic,
			Field:       model.FieldID{Class: owner, Name: "VALUE", Type: "J"},
		}},
		StaticValues: []model.EncodedValue{{Type: model.ValueFloat, Unsigned: 0x3f800000}},
	}
	result, err := ClassWithPolicy(file, class, ClassOptions{}, Recover)
	if err != nil {
		t.Fatalf("ClassWithPolicy Recover: %v", err)
	}
	if !result.ClassStubbed || !bytes.Contains(result.Bytes, []byte("<clinit>")) {
		t.Fatalf("result = %#v", result)
	}
	var field, initializer bool
	for _, recovery := range result.Recoveries {
		field = field || recovery.Action == "stub_initializer" && recovery.Scope == "field"
		initializer = initializer || recovery.Action == "stub_initializer" && recovery.Member == "<clinit>"
	}
	if !field || !initializer {
		t.Fatalf("recoveries = %#v", result.Recoveries)
	}
}

func TestRecoverFailsClosedForUnsupportedStaticInitializer(t *testing.T) {
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java unavailable")
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac unavailable")
	}
	const owner = "Ldex/RecoveredStatic;"
	file := &model.File{Entry: "classes.dex", Strings: []string{"not-an-object-constant"}}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic,
		StaticFields: []model.EncodedField{{
			AccessFlags: dexPublic | dexStatic,
			Field:       model.FieldID{Class: owner, Name: "VALUE", Type: "Ljava/lang/Object;"},
		}},
		StaticValues: []model.EncodedValue{{Type: model.ValueString, Unsigned: 0}},
	}
	result, err := ClassWithPolicy(file, class, ClassOptions{}, Recover)
	if err != nil {
		t.Fatalf("ClassWithPolicy Recover: %v", err)
	}
	var initializer, field bool
	for _, recovery := range result.Recoveries {
		if recovery.Action == "stub_initializer" && recovery.Member == "<clinit>" {
			initializer = true
		}
		if recovery.Action == "stub_initializer" && recovery.Member == "VALUE" {
			field = true
		}
	}
	if !initializer || !field {
		t.Fatalf("recoveries = %#v", result.Recoveries)
	}

	dir := t.TempDir()
	classPath := filepath.Join(dir, "dex", "RecoveredStatic.class")
	if err := os.MkdirAll(filepath.Dir(classPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(classPath, result.Bytes, 0o644); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) throws Exception {
        Class.forName("dex.RecoveredStatic", false, Driver.class.getClassLoader());
        try {
            Class.forName("dex.RecoveredStatic");
            throw new AssertionError("class initialized");
        } catch (ExceptionInInitializerError expected) {
        }
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, output)
	}
}

func TestRecoverClassVerifiesWithoutInitialization(t *testing.T) {
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java unavailable")
	}
	file, class := recoveryFixture()
	result, err := ClassWithPolicy(file, class, ClassOptions{}, Recover)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "Recovered.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, result.Bytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "dex.Recovered").CombinedOutput(); err == nil || bytes.Contains(output, []byte("VerifyError")) || bytes.Contains(output, []byte("ClassFormatError")) {
		t.Fatalf("java verification result: err=%v output=%s", err, output)
	}
}
