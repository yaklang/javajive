package dex2jar

import (
	"archive/zip"
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

func policyFixture() *model.Application {
	file := &model.File{
		Entry:  "classes.dex",
		Protos: []model.Proto{{ReturnType: "V"}},
		Classes: []model.Class{{
			Descriptor:  "Lexample/Policy;",
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: 0x0001,
			DirectMethods: []model.EncodedMethod{
				{
					AccessFlags: 0x0009,
					Method:      model.MethodID{Class: "Lexample/Policy;", Name: "ok", ProtoIndex: 0},
					Code:        &model.Code{Instructions: []uint16{0x000e}},
				},
				{
					AccessFlags: 0x0009,
					Method:      model.MethodID{Class: "Lexample/Policy;", Name: "unsupported", ProtoIndex: 0},
					Code:        &model.Code{Instructions: []uint16{0x00fa, 0, 0, 0}},
				},
			},
		}},
	}
	return &model.Application{Files: []*model.File{file}}
}

func TestApplicationStrictRejectsUnsupportedMethod(t *testing.T) {
	jar, detailed, err := applicationToJarWithPolicy(policyFixture(), FailureStrict)
	if err == nil {
		t.Fatal("strict conversion accepted an unsupported method")
	}
	if jar != nil {
		t.Fatalf("strict conversion returned %d JAR bytes", len(jar))
	}
	if detailed.Status != "failed" {
		t.Fatalf("status = %q, want failed", detailed.Status)
	}
}

func TestContainerPredicatesAndMagicBranches(t *testing.T) {
	for _, kind := range []ContainerKind{ContainerAPK, ContainerAPKS, ContainerXAPK, ContainerAPKM, ContainerAAB} {
		if !IsAndroidContainer(kind) {
			t.Fatalf("%s is not classified as Android", kind)
		}
	}
	if IsAndroidContainer(ContainerJAR) {
		t.Fatal("JAR was classified as Android")
	}
	for _, kind := range []ContainerKind{ContainerJMOD, ContainerJImage, ContainerJAR, ContainerWAR, ContainerEAR, ContainerAAR, ContainerPAR, ContainerZIP} {
		if !IsJavaArchiveContainer(kind) {
			t.Fatalf("%s is not classified as a Java archive", kind)
		}
	}
	if IsJavaArchiveContainer(ContainerAPK) {
		t.Fatal("APK was classified as a Java archive")
	}

	tests := []struct {
		name string
		data []byte
		want InputKind
	}{
		{"dex", []byte("dex\n035\x00"), InputDEX},
		{"class", []byte{0xca, 0xfe, 0xba, 0xbe}, InputClass},
		{"zip", []byte("PK\x03\x04"), InputZIP},
		{"jmod", []byte{'J', 'M', 1, 0}, InputJMOD},
		{"jimage", []byte{0xda, 0xda, 0xfe, 0xca}, InputJIMAGE},
		{"unknown", []byte("not an artifact"), InputUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DetectInputKind(test.data); got != test.want {
				t.Fatalf("DetectInputKind = %v, want %v", got, test.want)
			}
		})
	}
}

func TestConversionPolicyRejectsUnknownPolicyAndMissingFile(t *testing.T) {
	jar, detailed, err := DexToJarWithOptions([]byte("not an artifact"), ConvertOptions{Policy: FailurePolicy("unknown")})
	if err == nil || !strings.Contains(err.Error(), "unknown failure policy") || jar != nil || detailed.Status != "failed" {
		t.Fatalf("unknown policy: jar=%d report=%#v err=%v", len(jar), detailed, err)
	}
	report, err := DexFileToJarWithOptions(filepath.Join(t.TempDir(), "missing.dex"), filepath.Join(t.TempDir(), "out.jar"), ConvertOptions{})
	if err == nil || report.Status != "failed" {
		t.Fatalf("missing file: report=%#v err=%v", report, err)
	}
}

func TestBestEffortRejectsUnsupportedDEXVersion(t *testing.T) {
	data := make([]byte, 0x70)
	copy(data, []byte("dex\n040\x00"))
	jar, detailed, err := DexToJarWithOptions(data, ConvertOptions{Policy: FailureStubUnsupportedMethods})
	if err == nil || !strings.Contains(err.Error(), `unsupported DEX version "040"`) {
		t.Fatalf("DexToJarWithOptions error = %v", err)
	}
	if jar != nil || detailed.Status != "failed" {
		t.Fatalf("jar=%d report=%#v", len(jar), detailed)
	}
}

func TestApplicationBestEffortDoesNotStubMalformedMethod(t *testing.T) {
	app := policyFixture()
	app.Files[0].Classes[0].DirectMethods[1].Code.Instructions = []uint16{0x00ff}
	jar, detailed, err := applicationToJarWithPolicy(app, FailureStubUnsupportedMethods)
	if err == nil {
		t.Fatal("best-effort conversion accepted malformed bytecode")
	}
	if jar != nil {
		t.Fatalf("best-effort conversion returned %d JAR bytes after malformed input", len(jar))
	}
	if detailed.Status != "failed" || len(detailed.Diagnostics) != 1 || detailed.Diagnostics[0].Category != "invalid" {
		t.Fatalf("detailed report = %#v", detailed)
	}
}

func TestApplicationRecoverEmitsClassShellAndDiagnostics(t *testing.T) {
	app := policyFixture()
	app.Files[0].Classes[0].DirectMethods[1].Code.Instructions = []uint16{0x00ff}
	jar, detailed, err := applicationToJarWithPolicy(app, FailureRecover)
	if err != nil {
		t.Fatalf("recover conversion: %v", err)
	}
	if detailed.Status != "partial" || detailed.StubbedClasses != 1 || detailed.StubbedMethods != 2 || detailed.Summary.Warnings == 0 {
		t.Fatalf("detailed = %#v", detailed)
	}
	reader, err := zip.NewReader(bytes.NewReader(jar), int64(len(jar)))
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string][]byte)
	for _, file := range reader.File {
		stream, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		var body bytes.Buffer
		_, copyErr := body.ReadFrom(stream)
		stream.Close()
		if copyErr != nil {
			t.Fatal(copyErr)
		}
		entries[file.Name] = body.Bytes()
	}
	if len(entries["example/Policy.class"]) == 0 || len(entries["META-INF/javajive/diagnostics.json"]) == 0 {
		t.Fatalf("entries = %#v", entries)
	}
	if !bytes.Contains(entries["example/Policy.class"], []byte("JavaJiveRecovery")) || !bytes.Contains(entries["META-INF/javajive/diagnostics.json"], []byte("class.body_recovery")) {
		t.Fatal("recovery evidence is missing from class or JAR diagnostics")
	}
	second, secondReport, err := applicationToJarWithPolicy(app, FailureRecover)
	if err != nil || !bytes.Equal(second, jar) || !reflect.DeepEqual(secondReport, detailed) {
		t.Fatalf("recover output is nondeterministic: err=%v reports=%#v/%#v", err, detailed, secondReport)
	}
}

func TestApplicationRecoverKeepsGoodClassesAroundBadClass(t *testing.T) {
	app := policyFixture()
	bad := app.Files[0].Classes[0]
	bad.Descriptor = "Lexample/Bad;"
	for index := range bad.DirectMethods {
		bad.DirectMethods[index].Method.Class = bad.Descriptor
	}
	bad.DirectMethods[1].Code.Instructions = []uint16{0x00ff}
	goodClass := func(descriptor string) model.Class {
		return model.Class{
			Descriptor: descriptor, Superclass: "Ljava/lang/Object;", AccessFlags: 0x0001,
			DirectMethods: []model.EncodedMethod{{
				AccessFlags: 0x0009,
				Method:      model.MethodID{Class: descriptor, Name: "ok", ProtoIndex: 0},
				Code:        &model.Code{Instructions: []uint16{0x000e}},
			}},
		}
	}
	app.Files[0].Classes = []model.Class{goodClass("Lexample/Before;"), bad, goodClass("Lexample/After;")}

	if jar, _, err := applicationToJarWithPolicy(app, FailureStrict); err == nil || jar != nil {
		t.Fatalf("strict conversion: jar=%d err=%v", len(jar), err)
	}
	if jar, _, err := applicationToJarWithPolicy(app, FailureStubUnsupportedMethods); err == nil || jar != nil {
		t.Fatalf("method-only conversion: jar=%d err=%v", len(jar), err)
	}

	jar, detailed, err := applicationToJarWithPolicy(app, FailureRecover)
	if err != nil {
		t.Fatalf("recover conversion: %v", err)
	}
	if detailed.Status != "partial" || detailed.AttemptedClasses != 3 || detailed.StubbedClasses != 1 {
		t.Fatalf("detailed = %#v", detailed)
	}
	reader, err := zip.NewReader(bytes.NewReader(jar), int64(len(jar)))
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string][]byte)
	for _, entry := range reader.File {
		stream, openErr := entry.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		var body bytes.Buffer
		_, copyErr := body.ReadFrom(stream)
		closeErr := stream.Close()
		if copyErr != nil {
			t.Fatal(copyErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		entries[entry.Name] = body.Bytes()
	}
	for _, name := range []string{"example/Before.class", "example/Bad.class", "example/After.class"} {
		body := entries[name]
		if len(body) == 0 {
			t.Fatalf("missing %s; entries=%v", name, entries)
		}
		parsed, parseErr := javaclassparser.Parse(body)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		if name == "example/Bad.class" {
			source, dumpErr := parsed.Dump()
			if dumpErr != nil {
				t.Fatalf("dump %s: %v", name, dumpErr)
			}
			if !strings.HasPrefix(source, "// javajive recovery:") {
				t.Fatalf("bad class recovery comments are missing:\n%s", source)
			}
		}
	}
	if !bytes.Contains(entries["META-INF/javajive/diagnostics.json"], []byte("Lexample/Bad;")) {
		t.Fatal("diagnostics do not identify the recovered class")
	}
}

func TestApplicationRecoverPropagatesMinimalDependency(t *testing.T) {
	const parentDescriptor = "Lexample/MinimalParent;"
	const childDescriptor = "Lexample/DependentChild;"
	duplicate := model.EncodedField{
		AccessFlags: 0x0001,
		Field:       model.FieldID{Class: parentDescriptor, Name: "value", Type: "I"},
	}
	file := &model.File{
		Entry:  "classes.dex",
		Protos: []model.Proto{{ReturnType: "V"}},
		Classes: []model.Class{
			{
				Descriptor: childDescriptor, Superclass: parentDescriptor, AccessFlags: 0x0001,
				DirectMethods: []model.EncodedMethod{{
					AccessFlags: 0x0009,
					Method:      model.MethodID{Class: childDescriptor, Name: "ok", ProtoIndex: 0},
					Code:        &model.Code{Instructions: []uint16{0x000e}},
				}},
			},
			{
				Descriptor: parentDescriptor, Superclass: "Ljava/lang/Object;", AccessFlags: 0x0001,
				InstanceFields: []model.EncodedField{duplicate, duplicate},
			},
		},
	}
	jar, detailed, err := applicationToJarWithPolicy(&model.Application{Files: []*model.File{file}}, FailureRecover)
	if err != nil {
		t.Fatalf("recover conversion: %v", err)
	}
	if len(jar) == 0 || detailed.StubbedClasses != 2 {
		t.Fatalf("jar=%d detailed=%#v", len(jar), detailed)
	}
	var parentMinimal, childDependency bool
	for _, diagnostic := range detailed.Diagnostics {
		if diagnostic.Class == parentDescriptor && diagnostic.Code == "class.minimal_recovery" {
			parentMinimal = true
		}
		if diagnostic.Class == childDescriptor && diagnostic.Code == "class.body_recovery" && strings.Contains(diagnostic.Message, parentDescriptor) {
			childDependency = true
		}
	}
	if !parentMinimal || !childDependency {
		t.Fatalf("diagnostics = %#v", detailed.Diagnostics)
	}
}

func TestApplicationRecoverDoesNotCountMinimalShellMethodsAsTranslated(t *testing.T) {
	app := policyFixture()
	class := &app.Files[0].Classes[0]
	class.DirectMethods[1].Method.Name = "bad/name"
	jar, detailed, err := applicationToJarWithPolicy(app, FailureRecover)
	if err != nil {
		t.Fatalf("recover conversion: %v", err)
	}
	if len(jar) == 0 || detailed.StubbedClasses != 1 || detailed.Summary.Methods != 2 || detailed.Summary.TranslatedMethods != 0 {
		t.Fatalf("jar=%d detailed=%#v", len(jar), detailed)
	}
	for _, diagnostic := range detailed.Diagnostics {
		if diagnostic.Code == "class.minimal_recovery" {
			return
		}
	}
	t.Fatalf("minimal recovery diagnostic missing: %#v", detailed.Diagnostics)
}

func TestConversionDiagnosticsAreBoundedAndExplicitlyTruncated(t *testing.T) {
	input := make([]ConversionDiagnostic, maxConversionDiagnostics+100)
	for index := range input {
		input[index] = ConversionDiagnostic{
			Entry: strings.Repeat("entry\n", 100), Class: "Lexample/Many;", Method: fmt.Sprintf("m%d", index),
			Category: "recovery", Action: "stubbed", Message: strings.Repeat("message", 100),
		}
	}
	payload, diagnostics, err := encodeConversionDiagnostics(Report{Warnings: len(input)}, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > maxDiagnosticsPayload || len(diagnostics) > maxConversionDiagnostics {
		t.Fatalf("payload=%d diagnostics=%d", len(payload), len(diagnostics))
	}
	last := diagnostics[len(diagnostics)-1]
	if last.Code != "diagnostics.truncated" || !bytes.Contains(payload, []byte("diagnostics.truncated")) {
		t.Fatalf("last=%#v payload=%s", last, payload)
	}
	if strings.Contains(diagnostics[0].Entry, "\n") || len([]rune(diagnostics[0].Message)) > maxDiagnosticFieldRunes {
		t.Fatalf("diagnostic was not sanitized: %#v", diagnostics[0])
	}
	second, secondDiagnostics, err := encodeConversionDiagnostics(Report{Warnings: len(input)}, input)
	if err != nil || !bytes.Equal(payload, second) || !reflect.DeepEqual(diagnostics, secondDiagnostics) {
		t.Fatalf("bounded diagnostics are nondeterministic: %v", err)
	}
}

func TestApplicationBestEffortStubsAndReports(t *testing.T) {
	jar, detailed, err := applicationToJarWithPolicy(policyFixture(), FailureStubUnsupportedMethods)
	if err != nil {
		t.Fatalf("best-effort conversion: %v", err)
	}
	if detailed.Status != "partial" {
		t.Fatalf("status = %q, want partial", detailed.Status)
	}
	if detailed.Summary.Classes != 1 || detailed.Summary.Methods != 2 ||
		detailed.Summary.TranslatedMethods != 1 || detailed.Summary.Warnings != 1 {
		t.Fatalf("summary = %#v", detailed.Summary)
	}
	if len(detailed.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", detailed.Diagnostics)
	}
	diagnostic := detailed.Diagnostics[0]
	if diagnostic.Method != "unsupported()V" || diagnostic.Category != "unsupported" ||
		diagnostic.Action != "stubbed" || diagnostic.Feature == "" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	reader, err := zip.NewReader(bytes.NewReader(jar), int64(len(jar)))
	if err != nil {
		t.Fatalf("open JAR: %v", err)
	}
	var classEntry *zip.File
	var diagnostics bool
	for _, entry := range reader.File {
		switch entry.Name {
		case "example/Policy.class":
			classEntry = entry
		case "META-INF/javajive/diagnostics.json":
			diagnostics = true
		}
	}
	if len(reader.File) != 3 || classEntry == nil || !diagnostics {
		t.Fatalf("JAR entries = %#v", reader.File)
	}
	stream, err := classEntry.Open()
	if err != nil {
		t.Fatal(err)
	}
	var class bytes.Buffer
	if _, err := class.ReadFrom(stream); err != nil {
		stream.Close()
		t.Fatal(err)
	}
	stream.Close()
	body := class.Bytes()
	if !bytes.Contains(body, []byte("UnsupportedOperationException")) {
		t.Fatal("stubbed class does not reference UnsupportedOperationException")
	}
	if !strings.Contains(diagnostic.Message, "unsupported") {
		t.Fatalf("diagnostic message = %q", diagnostic.Message)
	}
}
