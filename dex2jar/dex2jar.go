// Package dex2jar translates raw DEX files and APK archives into deterministic
// Java archives using the native, pure-Go translation pipeline.
package dex2jar

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/javajive/dex2jar/input"
	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/lower"
	"github.com/yaklang/javajive/dex2jar/model"
)

const manifest = "Manifest-Version: 1.0\r\n\r\n"

var archiveTimestamp = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

// Options controls DEX and APK input bounds. Non-positive values use defaults.
type Options struct {
	MaxDEXSize      int64
	MaxAPKSize      int64
	MaxTotalDEXSize int64
	MaxDEXFiles     int
}

// Report summarizes a successful conversion. The current strict translator
// fails rather than skipping unsupported input, so Warnings is normally zero.
type Report struct {
	DEXFiles          int
	Classes           int
	Methods           int
	TranslatedMethods int
	Warnings          int
}

type jarEntry struct {
	name string
	data []byte
}

// InputKind identifies reverse-toolchain input by content magic.
type InputKind uint8

const (
	InputUnknown InputKind = iota
	InputClass
	InputDEX
	InputZIP
)

// DetectInputKind classifies supported reverse-toolchain input by content magic.
func DetectInputKind(data []byte) InputKind {
	switch {
	case hasDEXMagic(data):
		return InputDEX
	case hasZIPMagic(data):
		return InputZIP
	case len(data) >= 4 && data[0] == 0xca && data[1] == 0xfe && data[2] == 0xba && data[3] == 0xbe:
		return InputClass
	default:
		return InputUnknown
	}
}

// DexToJar translates either a raw DEX file or an APK, selected by input magic.
// On any parse or translation failure it returns no JAR bytes.
func DexToJar(data []byte, options Options) ([]byte, Report, error) {
	app, err := parseApplication(data, options)
	if err != nil {
		return nil, Report{}, err
	}
	return applicationToJar(app)
}

// APKToJar translates an APK containing classes*.dex entries.
func APKToJar(data []byte, options Options) ([]byte, Report, error) {
	if !hasZIPMagic(data) {
		return nil, Report{}, errors.New("dex2jar: input is not an APK/ZIP archive")
	}
	app, err := input.ParseAPKWithOptions(data, inputOptions(options))
	if err != nil {
		return nil, Report{}, err
	}
	return applicationToJar(app)
}

// DexFileToJar reads a raw DEX or APK and writes the complete JAR to a
// same-directory temporary file before replacing dst. An existing destination
// is left untouched when conversion or temporary-file writing fails.
func DexFileToJar(src, dst string, options Options) (Report, error) {
	info, err := os.Stat(src)
	if err != nil {
		return Report{}, fmt.Errorf("dex2jar: stat %q: %w", src, err)
	}
	if !info.Mode().IsRegular() {
		return Report{}, fmt.Errorf("dex2jar: input %q is not a regular file", src)
	}
	file, err := os.Open(src)
	if err != nil {
		return Report{}, fmt.Errorf("dex2jar: open %q: %w", src, err)
	}
	var magic [8]byte
	read, readErr := io.ReadFull(file, magic[:])
	_ = file.Close()
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return Report{}, fmt.Errorf("dex2jar: read magic from %q: %w", src, readErr)
	}
	maxSize := options.MaxDEXSize
	if hasZIPMagic(magic[:read]) {
		maxSize = options.MaxAPKSize
	}
	if maxSize <= 0 {
		if hasZIPMagic(magic[:read]) {
			maxSize = input.DefaultMaxAPKSize
		} else {
			maxSize = input.DefaultMaxDEXSize
		}
	}
	if info.Size() > maxSize {
		return Report{}, fmt.Errorf("dex2jar: input size %d exceeds limit %d", info.Size(), maxSize)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return Report{}, fmt.Errorf("dex2jar: read %q: %w", src, err)
	}
	jar, conversionReport, err := DexToJar(data, options)
	if err != nil {
		return conversionReport, err
	}
	if err := atomicWriteFile(dst, jar, 0o644); err != nil {
		return conversionReport, fmt.Errorf("dex2jar: write %q: %w", dst, err)
	}
	return conversionReport, nil
}

func inputOptions(options Options) input.Options {
	return input.Options{
		MaxDEXSize:      options.MaxDEXSize,
		MaxAPKSize:      options.MaxAPKSize,
		MaxTotalDEXSize: options.MaxTotalDEXSize,
		MaxDEXFiles:     options.MaxDEXFiles,
	}
}

func parseApplication(data []byte, options Options) (*model.Application, error) {
	inputOptions := inputOptions(options)
	switch DetectInputKind(data) {
	case InputDEX:
		file, err := input.ParseDEXWithOptions(data, "classes.dex", inputOptions)
		if err != nil {
			return nil, err
		}
		return &model.Application{Files: []*model.File{file}}, nil
	case InputZIP:
		return input.ParseAPKWithOptions(data, inputOptions)
	default:
		return nil, errors.New("dex2jar: unsupported input magic (want DEX or APK/ZIP)")
	}
}

func applicationToJar(app *model.Application) ([]byte, Report, error) {
	if app == nil {
		return nil, Report{}, errors.New("dex2jar: nil application")
	}
	report := Report{DEXFiles: len(app.Files)}
	entries := make([]jarEntry, 0)
	seen := make(map[string]string)
	for _, file := range app.Files {
		if file == nil {
			return nil, report, errors.New("dex2jar: application contains a nil DEX file")
		}
		for classIndex := range file.Classes {
			class := &file.Classes[classIndex]
			name, err := classEntryName(class.Descriptor)
			if err != nil {
				return nil, report, fmt.Errorf("dex2jar: class %q: %w", class.Descriptor, err)
			}
			if previous, exists := seen[name]; exists {
				return nil, report, fmt.Errorf("dex2jar: duplicate output entry %q from %q and %q", name, previous, file.Entry)
			}
			seen[name] = file.Entry

			classBytes, err := lower.Class(file, class)
			if err != nil {
				return nil, report, err
			}
			entries = append(entries, jarEntry{name: name, data: classBytes})
			report.Classes++
			for _, method := range class.DirectMethods {
				report.Methods++
				if method.Code != nil {
					report.TranslatedMethods++
				}
			}
			for _, method := range class.VirtualMethods {
				report.Methods++
				if method.Code != nil {
					report.TranslatedMethods++
				}
			}
		}
	}

	jar, err := writeJar(entries)
	if err != nil {
		return nil, report, err
	}
	return jar, report, nil
}

func classEntryName(descriptor string) (string, error) {
	name, err := ir.InternalName(descriptor)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(name, "[") || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("invalid class name %q", name)
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." || component == ".." {
			return "", fmt.Errorf("invalid class name %q", name)
		}
	}
	return name + ".class", nil
}

func writeJar(classes []jarEntry) ([]byte, error) {
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	entries := make([]jarEntry, 0, len(classes)+1)
	entries = append(entries, jarEntry{name: "META-INF/MANIFEST.MF", data: []byte(manifest)})
	entries = append(entries, classes...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		header.SetModTime(archiveTimestamp)
		header.SetMode(0o644)
		file, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("dex2jar: create JAR entry %q: %w", entry.name, err)
		}
		if _, err := file.Write(entry.data); err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("dex2jar: write JAR entry %q: %w", entry.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("dex2jar: finish JAR: %w", err)
	}
	return out.Bytes(), nil
}

func hasDEXMagic(data []byte) bool {
	return len(data) >= 8 && string(data[:4]) == "dex\n" && data[7] == 0 &&
		data[4] >= '0' && data[4] <= '9' && data[5] >= '0' && data[5] <= '9' && data[6] >= '0' && data[6] <= '9'
}

func hasZIPMagic(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' &&
		((data[2] == 3 && data[3] == 4) || (data[2] == 5 && data[3] == 6) || (data[2] == 7 && data[3] == 8))
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".javajive-dex2jar-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() {
		if temp != nil {
			_ = temp.Close()
		}
		if err != nil {
			_ = os.Remove(tempName)
		}
	}()
	if err = temp.Chmod(mode); err != nil {
		return err
	}
	if _, err = temp.Write(data); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		temp = nil
		return err
	}
	temp = nil
	if err = os.Rename(tempName, path); err != nil {
		return err
	}
	return nil
}
