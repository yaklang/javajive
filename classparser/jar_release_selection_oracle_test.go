package javaclassparser

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
)

type jarReleaseTestEntry struct {
	name string
	data []byte
}

func jarReleaseTestZIP(t *testing.T, entries []jarReleaseTestEntry) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, entry := range entries {
		// Resource-only oracle archives need exact names/order and bytes,
		// without spending the CI budget deflating 65,536 empty entries.
		file, err := w.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func jarReleaseTestLegacy(t *testing.T, raw []byte, target int) *JarFS {
	t.Helper()
	zipFS, err := filesys.NewZipFSRaw(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	z := NewJarFS(zipFS)
	if target != 0 {
		z.archive = &archiveFSState{targetRelease: target, active: true}
	}
	return z
}
func jarReleaseTestHex(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type jarReleaseOracleEntry struct{ name, hex string }
type jarReleaseOracleWorld struct {
	name       string
	entries    []jarReleaseOracleEntry
	multi      bool
	payload    string
	absentBase bool
	unknown    string
}

// Frozen independent OpenJDK21.0.2 observations: 54+14+3 authored worlds.
// Original result SHA256s: 355f44f0198f1dadcf307eaa4c7f1cdfd18a9d46399b62df911f5547a283ee66,
// 222771f133a50297948000d08b24c2a3dbd9fe92cf41da7b4acb1ab96eeeda90,
// 137d623723451e20a6aad3714b18da3ae0468640638a9d574b78c8f234422810.
// Unknown entries document the finite parser frontier; they do not count as
// exact boolean equivalence for malformed main attributes or duplicate-ZIP rules.
// Valid 512-byte line boundaries and EOF-discarded fragments follow the
// observed JDK FastInputStream/Attributes semantics without changing any oracle value.
var jarReleaseJDK21Worlds = []jarReleaseOracleWorld{
	{name: "missing-manifest", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{}},
	{name: "missing-flag", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a0d0a"}}},
	{name: "empty-manifest", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: ""}}},
	{name: "true-crlf", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "true-lf", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300a4d756c74692d52656c656173653a20747275650a0a"}}},
	{name: "true-cr", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d4d756c74692d52656c656173653a20747275650d0d"}}},
	{name: "mixed-key-value-case", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a6d556c54692d72456c456153653a20547255650d0a0d0a"}}},
	{name: "false", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}}},
	{name: "false-mixed", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2046614c73450d0a0d0a"}}},
	{name: "continued-value", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074720d0a2075650d0a0d0a"}}},
	{name: "continued-value-lf", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074720a2075650a0a"}}},
	{name: "continued-empty-first-fragment", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a200d0a20747275650d0a0d0a"}}},
	{name: "continued-double-space", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074720d0a202075650d0a0d0a"}}},
	{name: "trailing-value-space", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074727565200d0a0d0a"}}},
	{name: "leading-value-space", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2020747275650d0a0d0a"}}},
	{name: "trailing-value-tab", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074727565090d0a0d0a"}}},
	{name: "empty-continuation", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a200d0a0d0a"}}},
	{name: "no-final-blank", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a"}}},
	{name: "no-final-newline", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074727565"}}},
	{name: "no-delimiter-space", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a747275650d0a0d0a"}}},
	{name: "tab-delimiter", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a09747275650d0a0d0a"}}},
	{name: "space-in-key", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c65617365203a20747275650d0a0d0a"}}},
	{name: "unicode-in-key", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173c3a93a20747275650d0a0d0a"}}},
	{name: "folded-name", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52650d0a206c656173653a20747275650d0a0d0a"}}},
	{name: "suffix-key", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a582d4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "value-contains-marker", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a582d546573743a204d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "no-manifest-version", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "section-only-true", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a0d0a4e616d653a207468696e670d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "main-false-entry-true", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a4e616d653a207468696e670d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "main-true-entry-false", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a4e616d653a207468696e670d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}}},
	{name: "extra-leading-blank", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "0d0a4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "name-attribute-still-main", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4e616d653a207468696e670d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "duplicate-attribute-false-true", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "duplicate-attribute-true-false", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}}},
	{name: "duplicate-case-attribute-true-false", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a6d756c74692d72656c656173653a2066616c73650d0a0d0a"}}},
	{name: "misplaced-continuation", multi: false, payload: "", absentBase: false, unknown: "misplaced continuation", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "206a756e6b0d0a4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "bad-header-after-main-flag", multi: false, payload: "", absentBase: false, unknown: "header", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a4241440d0a0d0a"}}},
	{name: "bad-entry-after-main-true", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a4241440d0a"}}},
	{name: "bad-entry-after-main-false", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a4241440d0a"}}},
	{name: "unterminated-extra-header", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a583a2076616c7565"}}},
	{name: "physical-line-509", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a583a2061616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161610a4d756c74692d52656c656173653a20747275650a0a"}}},
	{name: "physical-line-510", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a583a206161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161610a4d756c74692d52656c656173653a20747275650a0a"}}},
	{name: "physical-line-511", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a583a20616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161610a4d756c74692d52656c656173653a20747275650a0a"}}},
	{name: "physical-line-512", multi: false, payload: "", absentBase: false, unknown: "physical line bound", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a583a2061616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161610a4d756c74692d52656c656173653a20747275650a0a"}}},
	{name: "physical-line-513", multi: false, payload: "", absentBase: false, unknown: "physical line bound", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a583a206161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161610a4d756c74692d52656c656173653a20747275650a0a"}}},
	{name: "lowercase-manifest-path", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "meta-inf/manifest.mf", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "mixedcase-manifest-path", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "Meta-Inf/Manifest.Mf", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "double-slash-path", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF//MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "duplicate-manifest-true-false", multi: false, payload: "BASE", absentBase: false, unknown: "duplicate physical manifests", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}}},
	{name: "duplicate-manifest-false-true", multi: true, payload: "V9", absentBase: false, unknown: "duplicate physical manifests", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}, {name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "duplicate-case-path-true-false", multi: false, payload: "BASE", absentBase: false, unknown: "duplicate physical manifests", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "meta-inf/manifest.mf", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}}},
	{name: "duplicate-case-path-false-true", multi: true, payload: "V9", absentBase: false, unknown: "duplicate physical manifests", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}, {name: "meta-inf/manifest.mf", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "duplicate-manifest-valid-invalid", multi: false, payload: "BASE", absentBase: false, unknown: "duplicate physical manifests", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "META-INF/MANIFEST.MF", hex: "4241440d0a"}}},
	{name: "duplicate-manifest-invalid-valid", multi: true, payload: "V9", absentBase: false, unknown: "duplicate physical manifests", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4241440d0a"}, {name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "fold-true-entry-raw-true", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074720d0a2075650d0a0d0a4e616d653a207468696e670d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "fold-true-main-unrelated-raw-true", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074720d0a2075650d0a583a204d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "fold-true-entry-unrelated-raw-true", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074720d0a2075650d0a0d0a4e616d653a207468696e670d0a583a204d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "fold-false-entry-raw-true", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066610d0a206c73650d0a0d0a4e616d653a207468696e670d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "duplicate-false-then-fold-true", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a4d756c74692d52656c656173653a2074720d0a2075650d0a0d0a"}}},
	{name: "duplicate-true-then-fold-true", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a4d756c74692d52656c656173653a2074720d0a2075650d0a0d0a"}}},
	{name: "lower-first-true-canonical-last-false", multi: false, payload: "BASE", absentBase: false, unknown: "duplicate physical manifests", entries: []jarReleaseOracleEntry{{name: "meta-inf/manifest.mf", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}}},
	{name: "lower-first-false-canonical-last-true", multi: true, payload: "V9", absentBase: false, unknown: "duplicate physical manifests", entries: []jarReleaseOracleEntry{{name: "meta-inf/manifest.mf", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}, {name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "fold-true-raw-in-invalid-entry", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074720d0a2075650d0a0d0a424144204d756c74692d52656c656173653a20747275650d0a"}}},
	{name: "nul-value-after-true", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2074727565000d0a0d0a"}}},
	{name: "crlf-physical-line-510", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a583a206161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161610d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "crlf-physical-line-511", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a583a20616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161610d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "only-versioned-nonmr", multi: false, payload: "", absentBase: true, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a0d0a"}}},
	{name: "only-versioned-mr", multi: true, payload: "V9", absentBase: true, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "no-root-manifest-versioned-true", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/versions/9/META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "root-false-versioned-true", multi: false, payload: "BASE", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}, {name: "META-INF/versions/9/META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}}},
	{name: "root-true-versioned-false", multi: true, payload: "V9", absentBase: false, unknown: "", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "META-INF/versions/9/META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}}},
}

func TestAdversarialJarReleaseSelectionJDK21ManifestOracle(t *testing.T) {
	for _, world := range jarReleaseJDK21Worlds {
		t.Run(world.name, func(t *testing.T) {
			entries := []jarReleaseTestEntry{}
			for _, e := range world.entries {
				entries = append(entries, jarReleaseTestEntry{e.name, jarReleaseTestHex(t, e.hex)})
			}
			if !world.absentBase {
				entries = append(entries, jarReleaseTestEntry{"probe.txt", []byte("BASE")})
			}
			entries = append(entries, jarReleaseTestEntry{"META-INF/versions/9/probe.txt", []byte("V9")})
			raw := jarReleaseTestZIP(t, entries)
			z := jarReleaseTestLegacy(t, raw, 21)
			multi, err := z.RuntimeMultiRelease()
			if world.unknown != "" {
				if err == nil || !strings.Contains(err.Error(), world.unknown) {
					t.Fatalf("expected explicit unknown %q; flag=%v err=%v (JDK flag=%v payload=%q)", world.unknown, multi, err, world.multi, world.payload)
				}
				if got, err := z.readJarEntry("probe.txt"); err == nil {
					t.Fatalf("unknown metadata silently selected %q", got)
				}
			} else {
				if err != nil || multi != world.multi {
					t.Fatalf("flag=%v err=%v want JDK=%v", multi, err, world.multi)
				}
				got, err := z.readJarEntry("probe.txt")
				if world.payload == "" {
					if err == nil {
						t.Fatalf("missing original base silently found %q", got)
					}
				} else if err != nil || string(got) != world.payload {
					t.Fatalf("logical=%q err=%v want JDK URL payload=%q", got, err, world.payload)
				}
			}
			// A physical request and the base view have independent read authority.
			got, err := z.readJarEntry("META-INF/versions/9/probe.txt")
			if err != nil || string(got) != "V9" {
				t.Fatalf("metadata status contaminated explicit physical read: %q %v", got, err)
			}
			base := jarReleaseTestLegacy(t, raw, 0)
			got, err = base.readJarEntry("probe.txt")
			if world.absentBase {
				if err == nil {
					t.Fatalf("base view invented missing base %q", got)
				}
			} else if err != nil || string(got) != "BASE" {
				t.Fatalf("default base view %q %v", got, err)
			}
			t.Logf("original JDK flag=%v runtime=%q; selector unknown=%q", world.multi, world.payload, world.unknown)
		})
	}
}

func jarReleaseTestReadOpen(t *testing.T, open func(string) (io.ReadCloser, error), name, want string) {
	t.Helper()
	f, err := open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil || string(raw) != want {
		t.Fatalf("Open(%s)=%q %v want %q", name, raw, err, want)
	}
}
