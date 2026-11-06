package inspect_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	artifactinspect "github.com/yaklang/javajive/inspect"
)

func TestAnalyzeAcceptedMultidexAPK(t *testing.T) {
	path := filepath.Join("..", "dex2jar", "testdata", "accept", "multidex.apk")
	report, err := artifactinspect.AnalyzeFile(path)
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}
	if report.Status != artifactinspect.StatusComplete {
		t.Fatalf("status = %q, diagnostics = %#v", report.Status, report.Diagnostics)
	}
	if report.Artifact.Format != "apk" || report.Artifact.Magic != "zip" || report.Artifact.Size == 0 || len(report.Artifact.SHA256) != 64 {
		t.Fatalf("artifact = %#v", report.Artifact)
	}
	if report.Archive == nil {
		t.Fatal("archive inventory is nil")
	}
	wantEntries := []string{"AndroidManifest.xml", "assets/ignored.dex", "classes.dex", "classes2.dex", "res/raw/placeholder.txt"}
	if got := entryNames(report.Archive.Entries); !reflect.DeepEqual(got, wantEntries) {
		t.Fatalf("archive entries = %v, want %v", got, wantEntries)
	}
	wantResources := []string{"AndroidManifest.xml", "assets/ignored.dex", "res/raw/placeholder.txt"}
	if got := entryNames(report.Archive.Resources); !reflect.DeepEqual(got, wantResources) {
		t.Fatalf("resources = %v, want %v", got, wantResources)
	}
	for _, entry := range report.Archive.Entries {
		if entry.Directory || entry.Size == 0 {
			continue
		}
		if len(entry.SHA256) != 64 {
			t.Fatalf("entry %q SHA-256 = %q", entry.Name, entry.SHA256)
		}
	}

	if len(report.DEXFiles) != 2 {
		t.Fatalf("DEX files = %d, want 2", len(report.DEXFiles))
	}
	for index, dex := range report.DEXFiles {
		wantIndex := index + 1
		wantEntry := "classes.dex"
		if wantIndex > 1 {
			wantEntry = "classes2.dex"
		}
		if dex.Status != artifactinspect.StatusComplete || dex.Provenance.Container != "apk" || !dex.Provenance.Canonical || dex.Provenance.MultidexIndex != wantIndex || dex.Provenance.Entry != wantEntry {
			t.Fatalf("DEX[%d] = %#v", index, dex)
		}
		if dex.Version != "035" || len(dex.SHA256) != 64 || len(dex.Classes) != 1 {
			t.Fatalf("DEX[%d] metadata = %#v", index, dex)
		}
		if len(dex.Classes[0].Methods) != 2 {
			t.Fatalf("DEX[%d] methods = %d, want 2", index, len(dex.Classes[0].Methods))
		}
		for _, method := range dex.Classes[0].Methods {
			if method.DisassemblyStatus != "complete" || len(method.CodeUnits) == 0 || len(method.Instructions) == 0 {
				t.Fatalf("method %s%s disassembly = %#v", method.Name, method.Descriptor, method)
			}
			for _, instruction := range method.Instructions {
				if instruction.Name == "" || instruction.Format == "" || instruction.Width == 0 || len(instruction.Raw) == 0 {
					t.Fatalf("method %s instruction = %#v", method.Name, instruction)
				}
			}
		}
	}

	first, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	secondReport, err := artifactinspect.AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(secondReport)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("JSON report is not byte-for-byte stable")
	}
}

// Debug info and annotations are decoded now, so this real-world fixture must
// analyze as complete rather than stopping at the metadata gate.
func TestAnalyzeExternalDEXCompletesAfterMetadataSupport(t *testing.T) {
	path := filepath.Join("..", "dex2jar", "testdata", "external", "arsylk-dexfile-tiny.dex")
	report, err := artifactinspect.AnalyzeFile(path)
	if err != nil {
		t.Fatalf("best-effort analysis returned error: %v", err)
	}
	if report.Status != artifactinspect.StatusComplete || report.Artifact.Format != "dex" || report.Artifact.Magic != "dex" {
		t.Fatalf("report status/artifact = %q / %#v", report.Status, report.Artifact)
	}
	if len(report.DEXFiles) != 1 {
		t.Fatalf("DEX files = %d, want 1", len(report.DEXFiles))
	}
	dex := report.DEXFiles[0]
	if dex.Status != artifactinspect.StatusComplete || dex.Version != "035" || dex.Size != 1072 || len(dex.SHA256) != 64 {
		t.Fatalf("DEX summary = %#v", dex)
	}
	if len(dex.Diagnostics) != 0 || len(report.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v / %#v", dex.Diagnostics, report.Diagnostics)
	}
	if len(dex.Classes) != 1 || dex.Classes[0].Descriptor != "Lfixtures/tiny/Hello;" {
		t.Fatalf("classes = %#v", dex.Classes)
	}
	if len(dex.Classes[0].Methods) == 0 || len(dex.Classes[0].Methods[0].Instructions) == 0 {
		t.Fatalf("method disassembly missing: %#v", dex.Classes[0].Methods)
	}
}

func TestAnalyzeExternalAPKKeepsInventoryAndProvenance(t *testing.T) {
	path := filepath.Join("..", "dex2jar", "testdata", "external", "arsylk-dexfile-multidex.apk")
	report, err := artifactinspect.AnalyzeFile(path)
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}
	if report.Status != artifactinspect.StatusPartial || report.Artifact.Format != "apk" || report.Archive == nil {
		t.Fatalf("report = %#v", report)
	}
	if len(report.Archive.Entries) == 0 || len(report.DEXFiles) == 0 {
		t.Fatalf("inventory was lost: archive=%d dex=%d", len(report.Archive.Entries), len(report.DEXFiles))
	}
	if report.DEXFiles[0].Provenance.Entry != "classes.dex" || report.DEXFiles[0].Provenance.MultidexIndex != 1 || !report.DEXFiles[0].Provenance.Canonical {
		t.Fatalf("provenance = %#v", report.DEXFiles[0].Provenance)
	}
	if len(report.DEXFiles[0].Classes) == 0 {
		t.Fatal("classes.dex inventory was lost")
	}
	if len(report.Diagnostics) == 0 {
		t.Fatal("partial analysis reported no diagnostics")
	}
	for _, diagnostic := range report.Diagnostics {
		if !strings.Contains(diagnostic.Message, "payload") && !strings.Contains(diagnostic.Message, "already defined") {
			t.Fatalf("unexpected diagnostic = %#v", diagnostic)
		}
	}
}

func TestAnalyzeUnsupportedMagic(t *testing.T) {
	report := artifactinspect.Analyze([]byte("not an artifact"), "renamed.apk")
	if report.Status != artifactinspect.StatusUnsupported || report.Artifact.Magic != "unknown" || report.Artifact.Format != "unknown" {
		t.Fatalf("report = %#v", report)
	}
	if len(report.Diagnostics) != 1 || report.Diagnostics[0].Message != "unsupported input magic" {
		t.Fatalf("diagnostics = %#v", report.Diagnostics)
	}
}

func entryNames(entries []artifactinspect.ArchiveEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name
	}
	return names
}

// Disabling disassembly must not change inventory, verdicts or diagnostics; it
// only omits the per-instruction payload that dominates large reports.
func TestAnalyzeDisassemblyOptionPreservesInventory(t *testing.T) {
	path := filepath.Join("..", "dex2jar", "testdata", "accept", "arithmetic.dex")
	full, err := artifactinspect.AnalyzeFileWithOptions(path, artifactinspect.Options{Disassembly: true})
	if err != nil {
		t.Fatal(err)
	}
	lean, err := artifactinspect.AnalyzeFileWithOptions(path, artifactinspect.Options{Disassembly: false})
	if err != nil {
		t.Fatal(err)
	}
	if full.Status != lean.Status || len(full.DEXFiles) != len(lean.DEXFiles) {
		t.Fatalf("status/inventory changed: %q vs %q", full.Status, lean.Status)
	}
	fullClasses := full.DEXFiles[0].Classes
	leanClasses := lean.DEXFiles[0].Classes
	if len(fullClasses) != len(leanClasses) {
		t.Fatalf("classes changed: %d vs %d", len(fullClasses), len(leanClasses))
	}
	for index := range fullClasses {
		if fullClasses[index].Translation != leanClasses[index].Translation {
			t.Fatalf("translation verdict changed for %s", fullClasses[index].Descriptor)
		}
		if len(fullClasses[index].Methods) != len(leanClasses[index].Methods) {
			t.Fatalf("methods changed for %s", fullClasses[index].Descriptor)
		}
		for methodIndex := range fullClasses[index].Methods {
			method := leanClasses[index].Methods[methodIndex]
			if len(method.Instructions) != 0 || len(method.CodeUnits) != 0 {
				t.Fatalf("disassembly was not skipped for %s", method.Name)
			}
		}
	}
	if len(fullClasses[0].Methods[0].Instructions) == 0 {
		t.Fatal("full report has no disassembly")
	}
}
