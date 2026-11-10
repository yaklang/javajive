package inspect_test

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"strings"
	"testing"

	artifactinspect "github.com/yaklang/javajive/inspect"
	"github.com/yaklang/javajive/internal/jimagefixture"
)

func containerZIP(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := file.Write(body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

func containerClassFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../classparser/jarwar/testdata/plexus-cipher-2.0.jar")
	if err != nil {
		t.Fatalf("read class fixture jar: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("open class fixture jar: %v", err)
	}
	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, ".class") {
			body, err := fs.ReadFile(reader, file.Name)
			if err != nil {
				t.Fatalf("read class fixture: %v", err)
			}
			return body
		}
	}
	t.Fatal("class fixture jar has no .class entry")
	return nil
}

func TestAnalyzeAPKS(t *testing.T) {
	apk, err := os.ReadFile("../dex2jar/testdata/accept/multidex.apk")
	if err != nil {
		t.Fatal(err)
	}
	resourceSplit := containerZIP(t, map[string][]byte{"res/raw/config.txt": []byte("config")})
	bundle := containerZIP(t, map[string][]byte{
		"base.apk":  apk,
		"split.apk": resourceSplit,
	})
	report := artifactinspect.Analyze(bundle, "app.apks")
	if report.Status != artifactinspect.StatusComplete || report.Artifact.Format != "apks" {
		t.Fatalf("report = %#v", report)
	}
	if len(report.DEXFiles) != 2 {
		t.Fatalf("DEX files = %d, want 2", len(report.DEXFiles))
	}
	wantEntries := []string{"base.apk!/classes.dex", "base.apk!/classes2.dex"}
	for index, dex := range report.DEXFiles {
		if dex.Provenance.Container != "apks" || dex.Provenance.Entry != wantEntries[index] ||
			dex.Provenance.MultidexIndex != index+1 || !dex.Provenance.Canonical {
			t.Fatalf("DEX[%d] provenance = %#v", index, dex.Provenance)
		}
	}
	for _, entry := range report.Archive.Entries {
		if entry.Name == "base.apk" && entry.Kind != "apk" {
			t.Fatalf("base.apk kind = %q, want apk", entry.Kind)
		}
	}
}

func TestAnalyzeXAPK(t *testing.T) {
	apk, err := os.ReadFile("../dex2jar/testdata/accept/multidex.apk")
	if err != nil {
		t.Fatal(err)
	}
	bundle := containerZIP(t, map[string][]byte{
		"manifest.json": []byte(`{"xapk_version":2}`),
		"base.apk":      apk,
	})
	report := artifactinspect.Analyze(bundle, "app.xapk")
	if report.Status != artifactinspect.StatusComplete || report.Artifact.Format != "xapk" {
		t.Fatalf("report = %#v", report)
	}
	if len(report.DEXFiles) != 2 || report.DEXFiles[0].Provenance.Container != "xapk" {
		t.Fatalf("DEX provenance = %#v", report.DEXFiles)
	}
}

func TestAnalyzeAAB(t *testing.T) {
	dex, err := os.ReadFile("../dex2jar/testdata/accept/arithmetic.dex")
	if err != nil {
		t.Fatal(err)
	}
	bundle := containerZIP(t, map[string][]byte{
		"BundleConfig.pb":      {1},
		"base/dex/classes.dex": dex,
	})
	report := artifactinspect.Analyze(bundle, "app.aab")
	if report.Status != artifactinspect.StatusComplete || report.Artifact.Format != "aab" {
		t.Fatalf("report = %#v", report)
	}
	if len(report.DEXFiles) != 1 {
		t.Fatalf("DEX files = %d, want 1", len(report.DEXFiles))
	}
	provenance := report.DEXFiles[0].Provenance
	if provenance.Container != "aab" || provenance.Entry != "base/dex/classes.dex" ||
		provenance.MultidexIndex != 1 || !provenance.Canonical {
		t.Fatalf("provenance = %#v", provenance)
	}
}

func TestAnalyzeJMOD(t *testing.T) {
	classBytes := containerClassFixture(t)
	payload := containerZIP(t, map[string][]byte{
		"classes/demo/Hello.class": classBytes,
		"conf/example.properties":  []byte("key=value\n"),
	})
	jmod := append([]byte{'J', 'M', 1, 0}, payload...)
	report := artifactinspect.Analyze(jmod, "java.base.jmod")
	if report.Status != artifactinspect.StatusComplete || report.Artifact.Magic != "jmod" || report.Artifact.Format != "jmod" {
		t.Fatalf("report = %#v", report)
	}
	if report.Archive == nil {
		t.Fatal("JMOD archive inventory is nil")
	}
	names := entryNames(report.Archive.Entries)
	want := []string{"classes/demo/Hello.class", "conf/example.properties"}
	if len(names) != len(want) {
		t.Fatalf("entries = %v, want %v", names, want)
	}
	for index := range want {
		if names[index] != want[index] {
			t.Fatalf("entries = %v, want %v", names, want)
		}
	}
}

func TestAnalyzeAAR(t *testing.T) {
	classBytes := containerClassFixture(t)
	classesJAR := containerZIP(t, map[string][]byte{"lib/Nested.class": classBytes})
	aar := containerZIP(t, map[string][]byte{
		"AndroidManifest.xml": []byte("<manifest/>\n"),
		"classes.jar":         classesJAR,
	})
	report := artifactinspect.Analyze(aar, "app.aar")
	if report.Status != artifactinspect.StatusComplete || report.Artifact.Format != "aar" {
		t.Fatalf("report = %#v", report)
	}
	if len(report.DEXFiles) != 0 {
		t.Fatalf("AAR unexpectedly produced DEX files: %#v", report.DEXFiles)
	}
	for _, entry := range report.Archive.Entries {
		if entry.Name == "classes.jar" && entry.Kind != "archive" {
			t.Fatalf("classes.jar kind = %q, want archive", entry.Kind)
		}
	}
}

func TestAnalyzeJImage(t *testing.T) {
	classBytes := containerClassFixture(t)
	image, err := jimagefixture.Build([]jimagefixture.Entry{
		{Path: "/test/demo/Hello.class", Data: classBytes, Compressed: true},
		{Path: "/test/conf/app.properties", Data: []byte("key=value\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	report := artifactinspect.Analyze(image, "modules")
	if report.Status != artifactinspect.StatusComplete || report.Artifact.Format != "jimage" || report.Artifact.Magic != "jimage" {
		t.Fatalf("report = %#v", report)
	}
	if report.Archive == nil || len(report.Archive.Entries) != 2 || len(report.Archive.Resources) != 1 {
		t.Fatalf("archive = %#v", report.Archive)
	}
	var classEntry, resourceEntry *artifactinspect.ArchiveEntry
	for index := range report.Archive.Entries {
		entry := &report.Archive.Entries[index]
		switch entry.Name {
		case "test/demo/Hello.class":
			classEntry = entry
		case "test/conf/app.properties":
			resourceEntry = entry
		}
	}
	if classEntry == nil || classEntry.Kind != "class" ||
		classEntry.CompressedSize == 0 || classEntry.Compression != 8 || len(classEntry.SHA256) != 64 {
		t.Fatalf("class entry = %#v", classEntry)
	}
	if resourceEntry == nil || resourceEntry.Kind != "resource" || len(resourceEntry.SHA256) != 64 {
		t.Fatalf("resource entry = %#v", resourceEntry)
	}
}

func TestAnalyzeJImageUnsupportedCompressor(t *testing.T) {
	classBytes := containerClassFixture(t)
	image, err := jimagefixture.Build([]jimagefixture.Entry{
		{Path: "/test/demo/Hello.class", Data: classBytes, Compressed: true, Compressor: "compact-cp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	report := artifactinspect.Analyze(image, "modules")
	if report.Status != artifactinspect.StatusPartial {
		t.Fatalf("status = %q, want partial", report.Status)
	}
	found := false
	for _, diagnostic := range report.Diagnostics {
		if strings.Contains(diagnostic.Message, "compact-cp") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v", report.Diagnostics)
	}
}

func TestAnalyzeGenericZIP(t *testing.T) {
	report := artifactinspect.Analyze(containerZIP(t, map[string][]byte{"data.txt": []byte("x")}), "data.zip")
	if report.Status != artifactinspect.StatusUnsupported || report.Artifact.Format != "zip" {
		t.Fatalf("report = %#v", report)
	}
	if len(report.Diagnostics) != 1 || !strings.Contains(report.Diagnostics[0].Message, "not identifiable") {
		t.Fatalf("diagnostics = %#v", report.Diagnostics)
	}
}
