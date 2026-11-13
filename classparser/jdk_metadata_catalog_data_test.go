package javaclassparser

import (
	"bytes"
	"os"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestJDKCompressedCatalogMatchesSource(t *testing.T) {
	want, err := os.ReadFile("jdk_metadata_catalog.json")
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
		t.Fatal("embedded JDK catalog differs from its reviewed source; run go generate ./classparser")
	}
}
