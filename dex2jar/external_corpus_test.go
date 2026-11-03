package dex2jar_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/dex2jar"
	"github.com/yaklang/javajive/dex2jar/report"
)

func TestExternalRejectionCorpus(t *testing.T) {
	tests := []struct {
		name           string
		file           string
		sha256         string
		size           int
		reasonCategory string
		entry          string
		class          string
		method         string
		offset         int64
		op             string
		reason         string
	}{
		{
			name:           "Arsylk dexfile tiny annotations",
			file:           "arsylk-dexfile-tiny.dex",
			sha256:         "613fc5ffac5c463f250d86be2355acdd2c261b052d2cdb12ce63efb479469145",
			size:           1072,
			reasonCategory: "annotations",
			entry:          "classes.dex",
			class:          "Lfixtures/tiny/Hello;",
			offset:         0x13c,
			op:             "class_defs",
			reason:         "unsupported annotations_off=0x354; must be zero",
		},
		{
			name:           "Arsylk dexfile multidex annotations",
			file:           "arsylk-dexfile-multidex.apk",
			sha256:         "eb3c68fa2b12819e8388000422963ed13d08dce5b2dde4b78472239356036577",
			size:           278319,
			reasonCategory: "annotations",
			entry:          "classes.dex",
			class:          "Lfixtures/Arith;",
			offset:         0x41b4,
			op:             "class_defs",
			reason:         "unsupported annotations_off=0x42694; must be zero",
		},
		{
			name:           "pxb1988 dex2jar DEX 038",
			file:           "pxb1988-dex2jar-dex038.dex",
			sha256:         "368dd6e8ea8aaee26ad46438ccbef42d28295a4f4957379e34843472b2f13ea4",
			size:           1376,
			reasonCategory: "version",
			entry:          "classes.dex",
			offset:         4,
			op:             "header",
			reason:         `unsupported DEX version "038"; only "035" is supported`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join("testdata", "external", test.file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read corpus file: %v", err)
			}
			if len(data) != test.size {
				t.Fatalf("size = %d, want %d", len(data), test.size)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != test.sha256 {
				t.Fatalf("SHA-256 = %s, want %s", got, test.sha256)
			}

			jar, conversionReport, err := dex2jar.DexToJar(data, dex2jar.Options{})
			if jar != nil {
				t.Fatalf("DexToJar returned %d bytes for rejected input", len(jar))
			}
			if conversionReport != (dex2jar.Report{}) {
				t.Fatalf("DexToJar report = %#v, want zero", conversionReport)
			}
			assertExternalCorpusDiagnostic(t, err, test.entry, test.class, test.method, test.offset, test.op, test.reasonCategory, test.reason)

			dir := t.TempDir()
			destination := filepath.Join(dir, "existing.jar")
			original := []byte("preserve existing destination")
			if err := os.WriteFile(destination, original, 0o644); err != nil {
				t.Fatalf("write destination: %v", err)
			}
			fileReport, err := dex2jar.DexFileToJar(path, destination, dex2jar.Options{})
			if fileReport != (dex2jar.Report{}) {
				t.Fatalf("DexFileToJar report = %#v, want zero", fileReport)
			}
			assertExternalCorpusDiagnostic(t, err, test.entry, test.class, test.method, test.offset, test.op, test.reasonCategory, test.reason)
			got, err := os.ReadFile(destination)
			if err != nil {
				t.Fatalf("read destination: %v", err)
			}
			if !bytes.Equal(got, original) {
				t.Fatalf("destination changed after rejection: %q", got)
			}
		})
	}
}

func assertExternalCorpusDiagnostic(t *testing.T, err error, entry, class, method string, offset int64, op, reasonCategory, reason string) {
	t.Helper()
	if err == nil {
		t.Fatal("conversion unexpectedly succeeded")
	}
	diagnostic, ok := report.AsDiagnostic(err)
	if !ok {
		t.Fatalf("error %T is not report.Diagnostic: %v", err, err)
	}
	if diagnostic.Entry != entry || diagnostic.Class != class || diagnostic.Method != method || diagnostic.Offset != offset || diagnostic.Op != op {
		t.Fatalf("diagnostic context = %#v, want entry=%q class=%q method=%q offset=%#x op=%q", diagnostic, entry, class, method, offset, op)
	}
	if diagnostic.Err == nil || diagnostic.Err.Error() != reason {
		t.Fatalf("diagnostic reason = %v, want %q", diagnostic.Err, reason)
	}
	if got := externalCorpusReasonCategory(reason); got != reasonCategory {
		t.Fatalf("diagnostic reason category = %q, want %q", got, reasonCategory)
	}
}

func externalCorpusReasonCategory(reason string) string {
	switch {
	case strings.HasPrefix(reason, "unsupported annotations_off="):
		return "annotations"
	case strings.HasPrefix(reason, `unsupported DEX version "`):
		return "version"
	default:
		return "unknown"
	}
}
