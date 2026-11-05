package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCachePathRejectsEscapes(t *testing.T) {
	cache := t.TempDir()
	for _, path := range []string{"../escape.dex", "/absolute.dex", "a/../../escape.dex", ""} {
		if _, err := cachePath(cache, path); err == nil {
			t.Errorf("cachePath(%q) accepted unsafe path", path)
		}
	}
	got, err := cachePath(cache, "malformed/empty.dex")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cache, "malformed", "empty.dex")
	if got != want {
		t.Fatalf("cachePath = %q, want %q", got, want)
	}
}

func TestVerifyFileRejectsWrongHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact.dex")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	size := int64(4)
	wrongHash := strings.Repeat("0", 64)
	err := verifyFile(path, artifact{Size: &size, SHA256: &wrongHash})
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("verifyFile wrong hash error = %v", err)
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		err      error
		stage    string
		category string
	}{
		{errors.New("dex2jar: unsupported input magic (want DEX or APK/ZIP)"), "identify", "magic"},
		{errors.New(`unsupported DEX version "041"; only "035" is supported`), "translate", "version"},
		{errors.New("range [0xfffffff0,0x100000010) exceeds file size 0x430"), "translate", "bounds"},
		{errors.New("unsupported annotations_off=0x354; must be zero"), "translate", "annotations"},
	}
	for _, test := range tests {
		stage, category := classify(test.err)
		if stage != test.stage || category != test.category {
			t.Errorf("classify(%q) = %s/%s, want %s/%s", test.err, stage, category, test.stage, test.category)
		}
	}
}
