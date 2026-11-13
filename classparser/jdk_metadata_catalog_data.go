package javaclassparser

import (
	_ "embed"
	"encoding/json"

	"github.com/klauspost/compress/zstd"
)

// Keep the reviewed JSON source for regeneration and provenance checks. Only
// its compressed representation is linked into applications.
//
//go:generate go run ../tools/compressjdk -input jdk_metadata_catalog.json
//go:embed jdk_metadata_catalog.json.zst
var jdkInvocationCatalogZstd []byte

// The production caller runs this inside loadJDKInvocationCatalog's sync.Once.
// Neither package initialization nor unrelated class lookups decompress the
// catalog, and the temporary JSON buffer is released after parsing.
func decodeJDKInvocationCatalog(target any) error {
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true))
	if err != nil {
		return err
	}
	defer decoder.Close()
	raw, err := decoder.DecodeAll(jdkInvocationCatalogZstd, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}
