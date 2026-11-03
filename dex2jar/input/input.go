// Package input loads raw DEX files and multidex APK archives.
package input

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"

	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
	"github.com/yaklang/javajive/internal/dexparser"
)

const (
	DefaultMaxDEXSize      int64 = 256 << 20
	DefaultMaxAPKSize      int64 = 512 << 20
	DefaultMaxTotalDEXSize int64 = 512 << 20
	DefaultMaxDEXFiles           = 128
)

var dexEntryPattern = regexp.MustCompile(`^classes([0-9]*)\.dex$`)

// Options controls bounded input reads. Non-positive values use the defaults.
type Options struct {
	MaxDEXSize      int64
	MaxAPKSize      int64
	MaxTotalDEXSize int64
	MaxDEXFiles     int
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
	if int64(len(data)) > options.maxAPKSize() {
		return nil, report.New("", "", "", -1, "APK", fmt.Errorf("archive size %d exceeds limit %d", len(data), options.maxAPKSize()))
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, report.New("", "", "", -1, "APK", err)
	}
	type entry struct {
		index int
		file  *zip.File
	}
	entries := make([]entry, 0)
	seenIndex := make(map[int]string)
	for _, file := range reader.File {
		match := dexEntryPattern.FindStringSubmatch(file.Name)
		if match == nil {
			continue
		}
		index := 1
		if match[1] != "" {
			parsed, parseErr := strconv.ParseInt(match[1], 10, 32)
			if parseErr != nil || parsed < 2 || match[1][0] == '0' {
				if parseErr == nil {
					parseErr = fmt.Errorf("index must be canonical and at least 2")
				}
				return nil, report.New(file.Name, "", "", -1, "APK entry", fmt.Errorf("invalid multidex index: %w", parseErr))
			}
			index = int(parsed)
		}
		if previous, exists := seenIndex[index]; exists {
			return nil, report.New(file.Name, "", "", -1, "APK entry", fmt.Errorf("duplicate multidex index %d also used by %q", index, previous))
		}
		seenIndex[index] = file.Name
		entries = append(entries, entry{index: index, file: file})
	}
	if len(entries) == 0 {
		return nil, report.New("", "", "", -1, "APK", errors.New("no classes*.dex entries"))
	}
	if len(entries) > options.maxDEXFiles() {
		return nil, report.New("", "", "", -1, "APK", fmt.Errorf("DEX file count %d exceeds limit %d", len(entries), options.maxDEXFiles()))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].index < entries[j].index })
	app := &model.Application{Files: make([]*model.File, 0, len(entries))}
	descriptors := make(map[string]string)
	var totalDEXSize uint64
	for _, item := range entries {
		if item.file.UncompressedSize64 > uint64(options.maxDEXSize()) || item.file.UncompressedSize64 > uint64(math.MaxInt) {
			return nil, report.New(item.file.Name, "", "", -1, "APK entry", fmt.Errorf("uncompressed size %d exceeds limit", item.file.UncompressedSize64))
		}
		if item.file.UncompressedSize64 > uint64(options.maxTotalDEXSize())-totalDEXSize {
			return nil, report.New(item.file.Name, "", "", -1, "APK entry", fmt.Errorf("total DEX size exceeds limit %d", options.maxTotalDEXSize()))
		}
		totalDEXSize += item.file.UncompressedSize64
		contents, readErr := readZipEntry(item.file, options.maxDEXSize())
		if readErr != nil {
			return nil, report.New(item.file.Name, "", "", -1, "APK entry", readErr)
		}
		file, parseErr := ParseDEXWithOptions(contents, item.file.Name, options)
		if parseErr != nil {
			return nil, parseErr
		}
		for _, class := range file.Classes {
			if previous, exists := descriptors[class.Descriptor]; exists {
				return nil, report.New(item.file.Name, class.Descriptor, "", -1, "duplicate class", fmt.Errorf("descriptor already defined in %q", previous))
			}
			descriptors[class.Descriptor] = item.file.Name
		}
		app.Files = append(app.Files, file)
	}
	return app, nil
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

func (o Options) maxDEXFiles() int {
	if o.MaxDEXFiles <= 0 {
		return DefaultMaxDEXFiles
	}
	return o.MaxDEXFiles
}
