// Package input loads raw DEX files, multidex APK archives, Android split
// bundles (APKS/XAPK/APKM), and Android App Bundles (AAB).
package input

import (
	"archive/zip"
	"fmt"
	"io"
	"regexp"

	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
	"github.com/yaklang/javajive/internal/dexparser"
)

const (
	DefaultMaxDEXSize      int64 = 256 << 20
	DefaultMaxAPKSize      int64 = 512 << 20
	DefaultMaxTotalDEXSize int64 = 512 << 20
	DefaultMaxTotalAPKSize int64 = 2 << 30
	DefaultMaxDEXFiles           = 128
	DefaultMaxNestedAPKs         = 64
)

var dexEntryPattern = regexp.MustCompile(`^classes([0-9]*)\.dex$`)

// Options controls bounded input reads. Non-positive values use the defaults.
type Options struct {
	MaxDEXSize      int64
	MaxAPKSize      int64
	MaxTotalDEXSize int64
	// MaxTotalAPKSize bounds the sum of nested APK sizes in APKS/XAPK/APKM
	// containers. Non-positive values use DefaultMaxTotalAPKSize.
	MaxTotalAPKSize int64
	MaxDEXFiles     int
	// MaxNestedAPKs bounds the number of nested APK entries in a split bundle.
	// Non-positive values use DefaultMaxNestedAPKs.
	MaxNestedAPKs int
}

func ParseDEX(data []byte, entry string) (*model.File, error) {
	return ParseDEXWithOptions(data, entry, Options{})
}

func ParseDEXWithOptions(data []byte, entry string, options Options) (*model.File, error) {
	limit := options.maxDEXSize()
	if int64(len(data)) > limit {
		return nil, report.New(entry, "", "", 0, "input", fmt.Errorf("DEX size %d exceeds limit %d", len(data), limit))
	}
	return dexparser.Parse(data, entry)
}

func ParseAPK(data []byte) (*model.Application, error) {
	return ParseAPKWithOptions(data, Options{})
}

func ParseAPKWithOptions(data []byte, options Options) (*model.Application, error) {
	sources, err := collectAPK(data, "", options, false)
	if err != nil {
		return nil, err
	}
	return parseDEXSources(sources, options, true)
}

func readZipEntry(file *zip.File, limit int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	limited := io.LimitReader(reader, limit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("decompressed data exceeds limit %d", limit)
	}
	if uint64(len(data)) != file.UncompressedSize64 {
		return nil, fmt.Errorf("uncompressed size mismatch: header=%d actual=%d", file.UncompressedSize64, len(data))
	}
	return data, nil
}

func (o Options) maxDEXSize() int64 {
	if o.MaxDEXSize <= 0 {
		return DefaultMaxDEXSize
	}
	return o.MaxDEXSize
}

func (o Options) maxAPKSize() int64 {
	if o.MaxAPKSize <= 0 {
		return DefaultMaxAPKSize
	}
	return o.MaxAPKSize
}

func (o Options) maxTotalDEXSize() int64 {
	if o.MaxTotalDEXSize <= 0 {
		return DefaultMaxTotalDEXSize
	}
	return o.MaxTotalDEXSize
}

func (o Options) maxTotalAPKSize() int64 {
	if o.MaxTotalAPKSize <= 0 {
		return DefaultMaxTotalAPKSize
	}
	return o.MaxTotalAPKSize
}

func (o Options) maxDEXFiles() int {
	if o.MaxDEXFiles <= 0 {
		return DefaultMaxDEXFiles
	}
	return o.MaxDEXFiles
}

func (o Options) maxNestedAPKs() int {
	if o.MaxNestedAPKs <= 0 {
		return DefaultMaxNestedAPKs
	}
	return o.MaxNestedAPKs
}
