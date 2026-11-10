// Package dex2jar translates raw DEX files, APK archives, Android split
// bundles (APKS/XAPK/APKM), and Android App Bundles (AAB) into deterministic
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
	dexreport "github.com/yaklang/javajive/dex2jar/report"
	"github.com/yaklang/javajive/internal/inputkind"
)

const manifest = "Manifest-Version: 1.0\r\n\r\n"

var archiveTimestamp = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

// Options controls DEX and Android container input bounds. Non-positive values
// use defaults.
type Options struct {
	MaxDEXSize      int64
	MaxAPKSize      int64
	MaxTotalDEXSize int64
	// MaxTotalAPKSize bounds the sum of nested APK sizes in APKS/XAPK/APKM
	// containers. Non-positive values use the input package default.
	MaxTotalAPKSize int64
	MaxDEXFiles     int
	// MaxNestedAPKs bounds the number of nested APK entries in a split bundle.
	// Non-positive values use the input package default.
	MaxNestedAPKs int
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
	// InputJMOD identifies a JMOD archive: "JM\x01\x00" followed by a ZIP.
	InputJMOD
	// InputJIMAGE identifies a JDK modular runtime image.
	InputJIMAGE
)

// DetectInputKind classifies supported reverse-toolchain input by content magic.
func DetectInputKind(data []byte) InputKind {
	switch inputkind.DetectMagic(data) {
	case inputkind.DEX:
		return InputDEX
	case inputkind.Class:
		return InputClass
	case inputkind.JMOD:
		return InputJMOD
	case inputkind.JIMAGE:
		return InputJIMAGE
	case inputkind.ZIP:
		return InputZIP
	default:
		return InputUnknown
	}
}

// ContainerKind is the structure-aware classification of an input. It refines
// InputKind for ZIP containers (APK vs APKS vs JAR, and so on).
type ContainerKind string

const (
	ContainerUnknown ContainerKind = "unknown"
	ContainerClass   ContainerKind = "class"
	ContainerDEX     ContainerKind = "dex"
	ContainerJMOD    ContainerKind = "jmod"
	ContainerJImage  ContainerKind = "jimage"
	ContainerJAR     ContainerKind = "jar"
	ContainerWAR     ContainerKind = "war"
	ContainerEAR     ContainerKind = "ear"
	ContainerAAR     ContainerKind = "aar"
	ContainerPAR     ContainerKind = "par"
	ContainerZIP     ContainerKind = "zip"
	ContainerAPK     ContainerKind = "apk"
	ContainerAPKS    ContainerKind = "apks"
	ContainerXAPK    ContainerKind = "xapk"
	ContainerAPKM    ContainerKind = "apkm"
	ContainerAAB     ContainerKind = "aab"
)

// ClassifyInput classifies data by magic, archive structure, and finally the
// supplied name's extension.
func ClassifyInput(data []byte, name string) ContainerKind {
	return ContainerKind(inputkind.DetectBytes(data, name))
}

// ClassifyInputFile classifies a file without loading the whole archive.
func ClassifyInputFile(name string) (ContainerKind, error) {
	kind, err := inputkind.DetectFile(name)
	return ContainerKind(kind), err
}

// IsAndroidContainer reports whether kind is translated by the DEX pipeline.
func IsAndroidContainer(kind ContainerKind) bool {
	return inputkind.IsAndroid(inputkind.Kind(kind))
}

// IsJavaArchiveContainer reports whether kind is exported by the Java archive
// decompiler.
func IsJavaArchiveContainer(kind ContainerKind) bool {
	return inputkind.IsJavaArchive(inputkind.Kind(kind))
}

// DexToJar translates a raw DEX, APK, APKS/XAPK/APKM bundle, or AAB, selected
// by content magic and archive structure. On any parse or translation failure
// it returns no JAR bytes.
func DexToJar(data []byte, options Options) ([]byte, Report, error) {
	app, _, err := parseApplication(data, options)
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
	if inputkind.IsJImageHeader(magic[:read]) {
		return Report{}, fmt.Errorf("dex2jar: unsupported DEX input: detected jimage")
	}
	archiveInput := hasZIPMagic(magic[:read]) || inputkind.IsJMODHeader(magic[:read])
	maxSize := options.MaxDEXSize
	if archiveInput {
		maxSize = options.MaxAPKSize
	}
	if maxSize <= 0 {
		if archiveInput {
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
		MaxTotalAPKSize: options.MaxTotalAPKSize,
		MaxDEXFiles:     options.MaxDEXFiles,
		MaxNestedAPKs:   options.MaxNestedAPKs,
	}
}

func parseApplication(data []byte, options Options) (*model.Application, ContainerKind, error) {
	app, kind, err := input.ParseApplicationWithOptions(data, "", inputOptions(options))
	if err != nil {
		return nil, ContainerKind(kind), err
	}
	return app, ContainerKind(kind), nil
}

func applicationToJar(app *model.Application) ([]byte, Report, error) {
	if app == nil {
		return nil, Report{}, errors.New("dex2jar: nil application")
	}
	report := Report{DEXFiles: len(app.Files)}
	entries := make([]jarEntry, 0)
	seen := make(map[string]string)
	hierarchy := applicationHierarchy(app)
	for _, file := range app.Files {
		if file == nil {
			return nil, report, errors.New("dex2jar: application contains a nil DEX file")
		}
		// Validate the file once instead of once per method.
		if err := ir.ValidateFile(file); err != nil {
			return nil, report, dexreport.New(file.Entry, "", "", 0, "metadata", err)
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

			classBytes, err := lower.ClassWithOptions(file, class, lower.ClassOptions{FileValidated: true, Hierarchy: hierarchy})
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

// applicationHierarchy indexes every class of the application so assignability
// resolves across multidex files, where a superclass often lives in a
// different DEX than its subclass.
func applicationHierarchy(app *model.Application) map[string]*model.Class {
	index := make(map[string]*model.Class)
	for _, file := range app.Files {
		if file == nil {
			continue
		}
		for position := range file.Classes {
			class := &file.Classes[position]
			if _, exists := index[class.Descriptor]; !exists {
				index[class.Descriptor] = class
			}
		}
	}
	return index
}
