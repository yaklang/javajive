package dex2jar

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"strings"
	"testing"

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
	if len(reader.File) != 2 || reader.File[1].Name != "example/Policy.class" {
		t.Fatalf("JAR entries = %#v", reader.File)
	}
	stream, err := reader.File[1].Open()
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
