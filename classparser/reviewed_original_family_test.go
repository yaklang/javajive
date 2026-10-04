package javaclassparser

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// A synthetic static member needs the original inherited namespace, which a
// standalone class fixture cannot supply. Read the locked original family;
// compare the fixture itself byte for byte before using any declarations.
// These historical classes are parsed, never executed.
func reviewedOriginalFamilySources(t *testing.T, raw []byte, jarRelative, flag string, check func(string)) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	jarBytes, err := os.ReadFile(filepath.Join(home, ".m2", "repository", jarRelative))
	if err != nil {
		t.Fatal(err)
	}
	lockBytes, err := os.ReadFile("../test/cross/testdata/historical-jars.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Artifacts map[string]string `json:"artifacts"`
	}
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(jarBytes)
	if expected := lock.Artifacts[jarRelative]; expected == "" || expected != hex.EncodeToString(digest[:]) {
		t.Fatalf("original family JAR does not match the corpus lock: %s", jarRelative)
	}
	archive, err := zip.NewReader(bytes.NewReader(jarBytes), int64(len(jarBytes)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]*zip.File{}
	for _, entry := range archive.File {
		entries[entry.Name] = entry
	}
	resolver := func(name string) ([]byte, bool) {
		entry := entries[name+".class"]
		if entry == nil {
			return nil, false
		}
		reader, err := entry.Open()
		if err != nil {
			return nil, false
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		return data, err == nil
	}
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	original, ok := resolver(object.GetClassName())
	if !ok || !bytes.Equal(original, raw) {
		t.Fatal("native fixture differs from the locked original JAR")
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv(flag, setting)
		source, err := DecompileWithResolver(raw, resolver)
		if err != nil {
			t.Fatal(err)
		}
		check(source)
	}
}
