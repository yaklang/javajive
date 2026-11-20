package javaclassparser

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func readSourceJDKInvocationCatalog(t *testing.T, target any) {
	t.Helper()
	raw, err := os.ReadFile("jdk_metadata_catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

func TestJDKCompressedCatalogMatchesSource(t *testing.T) {
	var projected struct {
		Schema   int `json:"schema"`
		Profiles []struct {
			Release          int                        `json:"release"`
			Classes          map[string]json.RawMessage `json:"classes"`
			ThrowableParents map[string][]string        `json:"throwable_hierarchy"`
			ReferenceParents map[string][]string        `json:"reference_hierarchy"`
		} `json:"profiles"`
	}
	readSourceJDKInvocationCatalog(t, &projected)
	want, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	got, err := decoder.DecodeAll(jdkInvocationCatalogZstd, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("embedded JDK catalog differs from its reviewed source projection; run go generate ./classparser")
	}
}
