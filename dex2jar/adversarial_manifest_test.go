package dex2jar_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type adversarialManifest struct {
	Schema        string `json:"$schema"`
	SchemaVersion int    `json:"schema_version"`
	Source        struct {
		Repository string `json:"repository"`
		Commit     string `json:"commit"`
	} `json:"source"`
	Artifacts []struct {
		ID           string   `json:"id"`
		Path         string   `json:"path"`
		SourcePath   *string  `json:"source_path"`
		URL          *string  `json:"url"`
		Availability string   `json:"availability"`
		Format       string   `json:"format"`
		Version      *string  `json:"version"`
		Size         *int64   `json:"size"`
		SHA256       *string  `json:"sha256"`
		License      string   `json:"license"`
		LicenseFile  string   `json:"license_file"`
		Features     []string `json:"features"`
		Expected     struct {
			Disposition string `json:"disposition"`
			Stage       string `json:"stage"`
			Category    string `json:"category"`
		} `json:"expected"`
	} `json:"artifacts"`
}

func TestAdversarialManifestSchemaAndPins(t *testing.T) {
	base := filepath.Join("testdata", "adversarial")
	manifestData, err := os.ReadFile(filepath.Join(base, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest adversarialManifest
	decoder := json.NewDecoder(strings.NewReader(string(manifestData)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Schema != "schema.json" || manifest.SchemaVersion != 1 {
		t.Fatalf("schema = %q version %d", manifest.Schema, manifest.SchemaVersion)
	}
	if manifest.Source.Repository != "https://github.com/Arsylk/dexfile" || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(manifest.Source.Commit) {
		t.Fatalf("invalid source pin: %#v", manifest.Source)
	}
	if schema, err := os.ReadFile(filepath.Join(base, manifest.Schema)); err != nil {
		t.Fatalf("read schema: %v", err)
	} else if !json.Valid(schema) {
		t.Fatal("schema.json is not valid JSON")
	}
	licenseData, err := os.ReadFile(filepath.Join(base, "licenses", "Arsylk-dexfile-MIT.txt"))
	if err != nil {
		t.Fatal(err)
	}
	licenseHash := sha256.Sum256(licenseData)
	if got, want := hex.EncodeToString(licenseHash[:]), "c7baba0f3384a232a3a7f334b1a78a7c98cdbfcdf153e191d924ca9dda179ead"; got != want {
		t.Fatalf("license SHA-256 = %s, want %s", got, want)
	}

	wantIDs := []string{
		"compact-dex", "container041", "malformed-bad-endian-tag", "malformed-bad-magic",
		"malformed-bad-offsets", "malformed-empty", "malformed-huge-sizes",
		"malformed-truncated-header", "malformed-truncated-mid", "malformed-wrong-file-size",
		"multidex", "release", "tiny", "v035", "v037", "v038", "v039", "v040", "v041",
	}
	ids := make([]string, 0, len(manifest.Artifacts))
	seenID := make(map[string]bool)
	seenPath := make(map[string]bool)
	commitSegment := "/" + manifest.Source.Commit + "/"
	for _, artifact := range manifest.Artifacts {
		ids = append(ids, artifact.ID)
		if seenID[artifact.ID] || seenPath[artifact.Path] {
			t.Fatalf("duplicate id or path: %q %q", artifact.ID, artifact.Path)
		}
		seenID[artifact.ID], seenPath[artifact.Path] = true, true
		if artifact.ID == "" || artifact.Path == "" || filepath.IsAbs(artifact.Path) || filepath.Clean(artifact.Path) != artifact.Path || strings.HasPrefix(artifact.Path, "..") {
			t.Fatalf("unsafe artifact path: %#v", artifact)
		}
		if artifact.License != "MIT" || artifact.LicenseFile != "licenses/Arsylk-dexfile-MIT.txt" {
			t.Fatalf("invalid license metadata for %s", artifact.ID)
		}
		if _, err := os.Stat(filepath.Join(base, artifact.LicenseFile)); err != nil {
			t.Fatalf("license file for %s: %v", artifact.ID, err)
		}
		if len(artifact.Features) == 0 || artifact.Expected.Disposition == "" || artifact.Expected.Stage == "" || artifact.Expected.Category == "" {
			t.Fatalf("incomplete labels or expectation for %s", artifact.ID)
		}
		validFormat := map[string]bool{"dex": true, "apk": true, "dex-container": true, "compact-dex": true}
		if !validFormat[artifact.Format] {
			t.Fatalf("invalid format %q for %s", artifact.Format, artifact.ID)
		}
		if artifact.Version != nil && !regexp.MustCompile(`^[0-9]{3}$`).MatchString(*artifact.Version) {
			t.Fatalf("invalid version %q for %s", *artifact.Version, artifact.ID)
		}
		switch artifact.Availability {
		case "available":
			if artifact.Expected.Disposition == "pending" || artifact.Expected.Stage == "availability" {
				t.Fatalf("available artifact %s has pending expectation", artifact.ID)
			}
			if artifact.URL == nil || artifact.SourcePath == nil || artifact.Size == nil || artifact.SHA256 == nil {
				t.Fatalf("available artifact %s lacks a complete pin", artifact.ID)
			}
			parsed, err := url.Parse(*artifact.URL)
			if err != nil || parsed.Scheme != "https" || parsed.Host != "raw.githubusercontent.com" || !strings.Contains(parsed.Path, commitSegment) || !strings.HasSuffix(parsed.Path, "/"+*artifact.SourcePath) {
				t.Fatalf("artifact %s has non-immutable URL %q", artifact.ID, *artifact.URL)
			}
			if *artifact.Size < 0 || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(*artifact.SHA256) {
				t.Fatalf("artifact %s has invalid size/hash", artifact.ID)
			}
		case "pending":
			if artifact.URL != nil || artifact.SourcePath != nil || artifact.Size != nil || artifact.SHA256 != nil {
				t.Fatalf("pending artifact %s must not carry an invented pin", artifact.ID)
			}
			if artifact.Expected.Disposition != "pending" || artifact.Expected.Stage != "availability" || artifact.Expected.Category != "artifact-unavailable" {
				t.Fatalf("pending artifact %s has invalid expectation", artifact.ID)
			}
		default:
			t.Fatalf("artifact %s has invalid availability %q", artifact.ID, artifact.Availability)
		}
	}
	sort.Strings(ids)
	if fmt.Sprint(ids) != fmt.Sprint(wantIDs) {
		t.Fatalf("artifact IDs = %v, want %v", ids, wantIDs)
	}
	if !seenID["v040"] || !seenID["compact-dex"] {
		t.Fatal("DEX 040 and CompactDex must remain explicit")
	}
}
