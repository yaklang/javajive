//go:build javajive_full_jdk

package javaclassparser

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"testing"
)

func TestJDKCompressedConstructorCodeMatchesSource(t *testing.T) {
	zr, err := zip.OpenReader("jdk_constructor_code.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}
	mf, err := files["manifest.json"].Open()
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := io.ReadAll(mf)
	mf.Close()
	if err != nil {
		t.Fatal(err)
	}
	var doc constructorCodeDocument
	if err := json.Unmarshal(manifestBytes, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Profiles) == 0 {
		t.Fatal("empty source constructor profiles")
	}

	// Trigger lazy initialization and verify profile/entry count matches source.
	if _, ok := jdkConstructorClassBytes("java/lang/Object", doc.Profiles[0].Release); !ok {
		t.Fatal("failed to load java/lang/Object from compressed bundle")
	}
	if len(jdkConstructorCode.entries) != len(doc.Profiles) {
		t.Fatalf("profile count mismatch: got %d want %d", len(jdkConstructorCode.entries), len(doc.Profiles))
	}

	for _, profile := range doc.Profiles {
		gotProfile := jdkConstructorCode.entries[profile.Release]
		if len(gotProfile) != len(profile.Classes) {
			t.Fatalf("release %d class count mismatch: got %d want %d", profile.Release, len(gotProfile), len(profile.Classes))
		}
		for name := range profile.Classes {
			zf := files[fmt.Sprintf("%d/%s.class", profile.Release, name)]
			if zf == nil {
				t.Fatalf("missing source zip entry %d/%s.class", profile.Release, name)
			}
			rc, err := zf.Open()
			if err != nil {
				t.Fatal(err)
			}
			want, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			got, ok := jdkConstructorClassBytes(name, profile.Release)
			if !ok {
				t.Fatalf("missing %d/%s in compressed bundle", profile.Release, name)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("byte mismatch for %d/%s; run go generate ./classparser", profile.Release, name)
			}
		}
	}
}
