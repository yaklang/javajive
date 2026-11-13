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

func TestRecoverPrevalidatesMethodSymbolBeforeStubCode(t *testing.T) {
	for _, test := range []struct {
		name       string
		methodName string
	}{
		{name: "grammar", methodName: "bad/name"},
		{name: "UTF limit", methodName: strings.Repeat("m", 65536)},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, class := recoveryFixture()
			class.DirectMethods[1].Method.Name = test.methodName
			result, err := ClassWithPolicy(file, class, ClassOptions{}, Recover)
			if err != nil {
				t.Fatalf("ClassWithPolicy Recover: %v", err)
			}
			if !result.ClassMinimal {
				t.Fatalf("invalid symbol did not fall back to a minimal shell: %#v", result)
			}
			parsed, err := javaclassparser.Parse(result.Bytes)
			if err != nil {
				t.Fatalf("parse minimal class: %v", err)
			}
			if len(parsed.Methods) != 0 {
				t.Fatalf("minimal shell methods = %d", len(parsed.Methods))
			}
		})
	}
}

func TestRecoverShellsPreserveInterfaceAndAnnotationKind(t *testing.T) {
	const annotationInterface = "Ljava/lang/annotation/Annotation;"
	for _, test := range []struct {
		name       string
		descriptor string
		flags      uint32
		interfaces []string
	}{
		{name: "interface", descriptor: "Ldex/RecoveredInterface;", flags: dexPublic | dexInterface | dexAbstract},
		{name: "annotation", descriptor: "Ldex/RecoveredAnnotation;", flags: dexPublic | dexInterface | dexAbstract | dexAnnotation, interfaces: []string{annotationInterface}},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := &model.File{Entry: "classes.dex", Protos: []model.Proto{{ReturnType: "V"}}}
			class := &model.Class{
				Descriptor: test.descriptor, Superclass: "Ldex/IllegalInterfaceSuper;", AccessFlags: test.flags, Interfaces: test.interfaces,
				VirtualMethods: []model.EncodedMethod{{
					AccessFlags: dexPublic | dexAbstract,
					Method:      model.MethodID{Class: test.descriptor, Name: "run", ProtoIndex: 0},
				}},
			}
			result, err := ClassWithPolicy(file, class, ClassOptions{}, Recover)
			if err != nil {
				t.Fatalf("ClassWithPolicy Recover: %v", err)
			}
			if result.ClassMinimal {
				t.Fatalf("legal declaration shell became minimal: %#v", result)
			}
			assertRecoveredInterfaceShape(t, result.Bytes, test.flags)
		})
	}
}

func TestRecoverInvalidAnnotationHierarchyFallsBackToMinimalKind(t *testing.T) {
	const owner = "Ldex/InvalidAnnotationHierarchy;"
	file := &model.File{Entry: "classes.dex", Protos: []model.Proto{{ReturnType: "V"}}}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;",
		AccessFlags: dexPublic | dexInterface | dexAbstract | dexAnnotation,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexStatic,
			Method:      model.MethodID{Class: owner, Name: "broken", ProtoIndex: 0},
			Code:        &model.Code{Instructions: []uint16{0x00ff}},
		}},
	}
	result, err := ClassWithPolicy(file, class, ClassOptions{}, Recover)
	if err != nil {
		t.Fatalf("ClassWithPolicy Recover: %v", err)
	}
	if !result.ClassMinimal {
		t.Fatalf("result = %#v", result)
	}
	assertRecoveredInterfaceShape(t, result.Bytes, class.AccessFlags)
}

func TestRecoverMinimalShellsPreserveInterfaceAndAnnotationKind(t *testing.T) {
	for _, test := range []struct {
		name       string
		descriptor string
		flags      uint32
	}{
		{name: "interface", descriptor: "Ldex/MinimalInterface;", flags: dexPublic | dexInterface | dexAbstract},
		{name: "interface unknown flag", descriptor: "Ldex/MinimalOddInterface;", flags: dexPublic | dexInterface | dexAbstract | 0x40000000},
		{name: "annotation", descriptor: "Ldex/MinimalAnnotation;", flags: dexPublic | dexInterface | dexAbstract | dexAnnotation},
		{name: "annotation unknown flag", descriptor: "Ldex/MinimalOddAnnotation;", flags: dexPublic | dexInterface | dexAbstract | dexAnnotation | 0x40000000},
	} {
		t.Run(test.name, func(t *testing.T) {
			field := model.EncodedField{
				AccessFlags: dexPublic | dexStatic | dexFinal,
				Field:       model.FieldID{Class: test.descriptor, Name: "VALUE", Type: "I"},
			}
			file := &model.File{Entry: "classes.dex"}
			class := &model.Class{
				Descriptor: test.descriptor, Superclass: "Ldex/IllegalInterfaceSuper;", AccessFlags: test.flags,
				StaticFields: []model.EncodedField{field, field},
			}
			result, err := ClassWithPolicy(file, class, ClassOptions{}, Recover)
			if err != nil {
				t.Fatalf("ClassWithPolicy Recover: %v", err)
			}
			if !result.ClassMinimal {
				t.Fatalf("duplicate declarations did not require a minimal shell: %#v", result)
			}
			assertRecoveredInterfaceShape(t, result.Bytes, test.flags)
		})
	}
}

func assertRecoveredInterfaceShape(t *testing.T, raw []byte, flags uint32) {
	t.Helper()
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse recovered interface: %v", err)
	}
	if parsed.GetSupperClassName() != "java/lang/Object" {
		t.Fatalf("superclass = %q", parsed.GetSupperClassName())
	}
	const kindMask = dexInterface | dexAbstract | dexAnnotation
	if uint32(parsed.AccessFlags)&kindMask != flags&kindMask {
		t.Fatalf("access flags = 0x%x, want kind 0x%x", parsed.AccessFlags, flags&kindMask)
	}
	if flags&dexAnnotation != 0 {
		interfaces := parsed.GetInterfacesName()
		if len(interfaces) != 1 || interfaces[0] != "java/lang/annotation/Annotation" {
			t.Fatalf("annotation interfaces = %v", interfaces)
		}
	}
}

func TestRecoverDeclarationShellPreservesParameterAnnotations(t *testing.T) {
	file, class := classMetadataFixture()
	result, err := ClassWithPolicy(file, class, ClassOptions{ForceClassShell: true, RecoveryReason: "test shell"}, Recover)
	if err != nil {
		t.Fatalf("ClassWithPolicy Recover: %v", err)
	}
	parsed, err := javaclassparser.Parse(result.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(method.NameIndex)).Value != "cloneIt" {
			continue
		}
		for _, attribute := range method.Attributes {
			parameter, ok := attribute.(*javaclassparser.RuntimeVisibleParameterAnnotationsAttribute)
			if ok && !parameter.IsInvisible && len(parameter.ParameterAnnotations) == 1 && len(parameter.ParameterAnnotations[0]) == 1 {
				return
			}
		}
		t.Fatal("cloneIt parameter annotation was not preserved")
	}
	t.Fatal("cloneIt method is missing")
}

func TestRecoverMinimalShellReportsDroppedParameterAnnotations(t *testing.T) {
	file, class := classMetadataFixture()
	class.DirectMethods[0].Method.Name = "bad/name"
	result, err := ClassWithPolicy(file, class, ClassOptions{}, Recover)
	if err != nil {
		t.Fatalf("ClassWithPolicy Recover: %v", err)
	}
	if !result.ClassMinimal {
		t.Fatalf("result = %#v", result)
	}
	for _, recovery := range result.Recoveries {
		if recovery.Code == "metadata.parameter_annotation_dropped" {
			return
		}
	}
	t.Fatalf("recoveries = %#v", result.Recoveries)
}

func TestRecoverDeclarationShellReportsDroppedParameterAnnotations(t *testing.T) {
	file, class := classMetadataFixture()
	class.ParameterAnnotations[0].Parameters = append(class.ParameterAnnotations[0].Parameters, nil)
	result, err := ClassWithPolicy(file, class, ClassOptions{ForceClassShell: true, RecoveryReason: "test shell"}, Recover)
	if err != nil {
		t.Fatalf("ClassWithPolicy Recover: %v", err)
	}
	for _, recovery := range result.Recoveries {
		if recovery.Code == "metadata.parameter_annotation_dropped" && recovery.Member == "cloneIt" && recovery.Descriptor == "([I)Ljava/lang/Object;" {
			return
		}
	}
	t.Fatalf("recoveries = %#v", result.Recoveries)
}

func TestRecoverChildFirstInterfaceShellsVerifyOnJVM(t *testing.T) {
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java unavailable")
	}
	const parent = "Ldex/ZRecoveredInterface;"
	field := model.EncodedField{
		AccessFlags: dexPublic | dexStatic | dexFinal,
		Field:       model.FieldID{Class: parent, Name: "VALUE", Type: "I"},
	}
	parentResult, err := ClassWithPolicy(&model.File{Entry: "classes.dex"}, &model.Class{
		Descriptor: parent, Superclass: "Ldex/IllegalInterfaceSuper;", AccessFlags: dexPublic | dexInterface | dexAbstract,
		StaticFields: []model.EncodedField{field, field},
	}, ClassOptions{}, Recover)
	if err != nil {
		t.Fatal(err)
	}
	const child = "Ldex/ARecoveredChild;"
	childFile := &model.File{Entry: "classes.dex", Protos: []model.Proto{{ReturnType: "V"}}}
	childClass := &model.Class{
		Descriptor: child, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic, Interfaces: []string{parent},
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexStatic,
			Method:      model.MethodID{Class: child, Name: "broken", ProtoIndex: 0},
			Code:        &model.Code{Instructions: []uint16{0x00ff}},
		}},
	}
	childResult, err := ClassWithPolicy(childFile, childClass, ClassOptions{}, Recover)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, raw := range map[string][]byte{
		"dex/ARecoveredChild.class":     childResult.Bytes,
		"dex/ZRecoveredInterface.class": parentResult.Bytes,
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	output, runErr := exec.Command(java, "-Xverify:all", "-cp", dir, "dex.ARecoveredChild").CombinedOutput()
	if runErr == nil || bytes.Contains(output, []byte("VerifyError")) || bytes.Contains(output, []byte("ClassFormatError")) || bytes.Contains(output, []byte("IncompatibleClassChangeError")) {
		t.Fatalf("child-first interface verification: err=%v output=%s", runErr, output)
	}
}
