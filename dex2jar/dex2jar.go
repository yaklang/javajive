// Package dex2jar translates raw DEX files, APK archives, Android split
// bundles (APKS/XAPK/APKM), and Android App Bundles (AAB) into deterministic
// Java archives using the native, pure-Go translation pipeline.
package dex2jar

import (
	"archive/zip"
	"bytes"
	"encoding/json"
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

// FailurePolicy selects strict conversion, recognized method-local stubbing,
// or full class-level recovery. The zero value is strict.
type FailurePolicy string

const (
	FailureStrict                 FailurePolicy = "strict"
	FailureStubUnsupportedMethods FailurePolicy = "stub_unsupported_methods"
	FailureRecover                FailurePolicy = "recover"
)

// ConvertOptions combines existing input bounds with an explicit conversion
// policy without changing the source-compatible Options type.
type ConvertOptions struct {
	Input  Options
	Policy FailurePolicy
}

// ConversionDiagnostic is stable, JSON-friendly evidence of one degradation or
// terminal conversion failure.
type ConversionDiagnostic struct {
	Entry      string `json:"entry,omitempty"`
	Class      string `json:"class,omitempty"`
	Method     string `json:"method,omitempty"`
	Descriptor string `json:"descriptor,omitempty"`
	Category   string `json:"category"`
	Action     string `json:"action"`
	Feature    string `json:"feature,omitempty"`
	Code       string `json:"code,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Stage      string `json:"stage,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Offset     int64  `json:"offset,omitempty"`
	Op         string `json:"op,omitempty"`
	Artifact   string `json:"artifact,omitempty"`
	Message    string `json:"message"`
}

// DetailedReport describes complete, partial, or failed conversion without
// weakening the existing scalar Report contract.
type DetailedReport struct {
	Status            string                 `json:"status"`
	Summary           Report                 `json:"summary"`
	AttemptedClasses  int                    `json:"attempted_classes"`
	StubbedClasses    int                    `json:"stubbed_classes"`
	OmittedClasses    int                    `json:"omitted_classes"`
	StubbedMethods    int                    `json:"stubbed_methods"`
	DroppedAttributes int                    `json:"dropped_attributes"`
	Diagnostics       []ConversionDiagnostic `json:"diagnostics,omitempty"`
}

type jarEntry struct {
	name string
	data []byte
}

type convertedClass struct {
	file   *model.File
	class  *model.Class
	name   string
	result lower.ClassResult
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
	jar, detailed, err := DexToJarWithOptions(data, ConvertOptions{Input: options, Policy: FailureStrict})
	return jar, detailed.Summary, err
}

// DexToJarWithOptions translates an input with an explicit failure policy. An
// error always means no JAR bytes; partial success has a nil error and Status
// "partial" with method, class, initializer, and metadata recovery diagnostics.
func DexToJarWithOptions(data []byte, options ConvertOptions) (jar []byte, detailed DetailedReport, err error) {
	detailed.Status = "failed"
	defer func() {
		if recovered := recover(); recovered != nil {
			jar = nil
			detailed.Status = "failed"
			detailed.Diagnostics = append(detailed.Diagnostics, ConversionDiagnostic{
				Category: "internal",
				Action:   "failed",
				Message:  fmt.Sprintf("dex2jar internal panic: %v", recovered),
			})
			err = fmt.Errorf("dex2jar: internal panic: %v", recovered)
		}
	}()
	policy := options.Policy
	if policy == "" {
		policy = FailureStrict
	}
	if policy != FailureStrict && policy != FailureStubUnsupportedMethods && policy != FailureRecover {
		return nil, detailed, fmt.Errorf("dex2jar: unknown failure policy %q", policy)
	}
	app, _, err := parseApplication(data, options.Input)
	if err != nil {
		detailed.Diagnostics = append(detailed.Diagnostics, diagnosticFromError(err, "failed", ""))
		return nil, detailed, err
	}
	return applicationToJarWithPolicy(app, policy)
}

// APKToJar translates an APK containing classes*.dex entries.
func APKToJar(data []byte, options Options) (jar []byte, report Report, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			jar = nil
			report = Report{}
			err = fmt.Errorf("dex2jar: internal panic: %v", recovered)
		}
	}()
	if !hasZIPMagic(data) {
		return nil, Report{}, errors.New("dex2jar: input is not an APK/ZIP archive")
	}
	app, err := input.ParseAPKWithOptions(data, inputOptions(options))
	if err != nil {
		return nil, Report{}, err
	}
	jar, report, err = applicationToJar(app)
	return
}

// DexFileToJarWithOptions converts a file with explicit policy and atomically
// replaces dst only after a complete or explicitly partial JAR is available.
func DexFileToJarWithOptions(src, dst string, options ConvertOptions) (DetailedReport, error) {
	data, err := readInputFile(src, options.Input)
	if err != nil {
		return DetailedReport{Status: "failed"}, err
	}
	jar, detailed, err := DexToJarWithOptions(data, options)
	if err != nil {
		return detailed, err
	}
	if err := atomicWriteFile(dst, jar, 0o644); err != nil {
		detailed.Status = "failed"
		return detailed, fmt.Errorf("dex2jar: write %q: %w", dst, err)
	}
	return detailed, nil
}

// DexFileToJar reads a raw DEX or APK and writes the complete JAR to a
// same-directory temporary file before replacing dst. An existing destination
// is left untouched when conversion or temporary-file writing fails.
func DexFileToJar(src, dst string, options Options) (Report, error) {
	detailed, err := DexFileToJarWithOptions(src, dst, ConvertOptions{Input: options, Policy: FailureStrict})
	return detailed.Summary, err
}

func readInputFile(src string, options Options) ([]byte, error) {
	info, err := os.Stat(src)
	if err != nil {
		return nil, fmt.Errorf("dex2jar: stat %q: %w", src, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("dex2jar: input %q is not a regular file", src)
	}
	file, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("dex2jar: open %q: %w", src, err)
	}
	var magic [8]byte
	read, readErr := io.ReadFull(file, magic[:])
	_ = file.Close()
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("dex2jar: read magic from %q: %w", src, readErr)
	}
	if inputkind.IsJImageHeader(magic[:read]) {
		return nil, fmt.Errorf("dex2jar: unsupported DEX input: detected jimage")
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
		return nil, fmt.Errorf("dex2jar: input size %d exceeds limit %d", info.Size(), maxSize)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("dex2jar: read %q: %w", src, err)
	}
	return data, nil
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
	jar, detailed, err := applicationToJarWithPolicy(app, FailureStrict)
	return jar, detailed.Summary, err
}

func applicationToJarWithPolicy(app *model.Application, policy FailurePolicy) ([]byte, DetailedReport, error) {
	detailed := DetailedReport{Status: "failed"}
	if app == nil {
		return nil, detailed, errors.New("dex2jar: nil application")
	}
	detailed.Summary.DEXFiles = len(app.Files)
	entries := make([]jarEntry, 0)
	seen := make(map[string]string)
	hierarchy := applicationHierarchy(app)
	converted := make([]convertedClass, 0, len(hierarchy))
	lowerPolicy := lower.Strict
	switch policy {
	case FailureStubUnsupportedMethods:
		lowerPolicy = lower.StubUnsupportedMethods
	case FailureRecover:
		lowerPolicy = lower.Recover
	}
	for _, file := range app.Files {
		if file == nil {
			return nil, detailed, errors.New("dex2jar: application contains a nil DEX file")
		}
		if err := ir.ValidateFile(file); err != nil {
			err = dexreport.New(file.Entry, "", "", 0, "metadata", err)
			detailed.Diagnostics = append(detailed.Diagnostics, diagnosticFromError(err, "failed", ""))
			return nil, detailed, err
		}
		for classIndex := range file.Classes {
			class := &file.Classes[classIndex]
			detailed.AttemptedClasses++
			name, err := classEntryName(class.Descriptor)
			if err != nil {
				err = fmt.Errorf("dex2jar: class %q: %w", class.Descriptor, err)
				detailed.Diagnostics = append(detailed.Diagnostics, diagnosticFromError(err, "failed", ""))
				return nil, detailed, err
			}
			if previous, exists := seen[name]; exists {
				err = fmt.Errorf("dex2jar: duplicate output entry %q from %q and %q", name, previous, file.Entry)
				detailed.Diagnostics = append(detailed.Diagnostics, diagnosticFromError(err, "failed", ""))
				return nil, detailed, err
			}
			seen[name] = file.Entry

			result, err := lower.ClassWithPolicy(file, class, lower.ClassOptions{FileValidated: true, Hierarchy: hierarchy}, lowerPolicy)
			if err != nil {
				detailed.Summary.Classes = len(converted)
				detailed.Diagnostics = append(detailed.Diagnostics, diagnosticFromError(err, "failed", ""))
				return nil, detailed, err
			}
			converted = append(converted, convertedClass{file: file, class: class, name: name, result: result})
		}
	}
	if policy == FailureRecover {
		if err := recoverMinimalDependencies(converted, hierarchy); err != nil {
			detailed.Diagnostics = append(detailed.Diagnostics, diagnosticFromError(err, "failed", "dependency-closure"))
			return nil, detailed, err
		}
	}
	for _, item := range converted {
		file, class, name, result := item.file, item.class, item.name, item.result
		entries = append(entries, jarEntry{name: name, data: result.Bytes})
		detailed.Summary.Classes++
		if result.ClassStubbed {
			detailed.StubbedClasses++
		}
		for _, recovery := range result.Recoveries {

			detailed.Summary.Warnings++
			if recovery.Action == "drop_attribute" {
				detailed.DroppedAttributes++
			}
			scope := recovery.Scope
			if scope == "" {
				scope = "class"
			}
			detailed.Diagnostics = append(detailed.Diagnostics, ConversionDiagnostic{
				Entry: file.Entry, Class: class.Descriptor, Method: recovery.Member,
				Descriptor: recovery.Descriptor, Category: "recovery", Action: recovery.Action,
				Feature: recovery.Feature, Code: recovery.Code, Kind: "structural",
				Scope: scope, Stage: "class_assemble", Severity: "warning", Message: recovery.Message,
			})
		}
		recoveredMethods := make(map[string]bool, len(result.Recoveries))
		for _, recovery := range result.Recoveries {
			if recovery.Action == "stub_method" || recovery.Action == "stub_initializer" && recovery.Scope == "method" {
				recoveredMethods[recovery.Member+"\x00"+recovery.Descriptor] = true
			}
		}

		stubs := make(map[string]bool, len(result.StubbedMethods))
		for _, stub := range result.StubbedMethods {
			stubs[stub.Method+"\x00"+stub.Descriptor] = true
			detailed.StubbedMethods++
			if !result.ClassStubbed {
				detailed.Summary.Warnings++
			}
			if !recoveredMethods[stub.Method+"\x00"+stub.Descriptor] {
				detailed.Diagnostics = append(detailed.Diagnostics, ConversionDiagnostic{
					Entry: file.Entry, Class: class.Descriptor, Method: stub.Method + stub.Descriptor,
					Descriptor: stub.Descriptor, Category: "unsupported", Action: "stubbed",
					Feature: stub.Feature, Code: "method.unsupported_stub", Kind: "capability",
					Scope: "method", Stage: "lower_method", Severity: "warning", Message: stub.Message,
				})
			}
		}
		for _, method := range append(class.DirectMethods, class.VirtualMethods...) {
			detailed.Summary.Methods++
			if method.Code == nil {
				continue
			}
			descriptor, descriptorErr := methodDescriptorForReport(file, &method)
			if descriptorErr == nil && !stubs[method.Method.Name+"\x00"+descriptor] {
				detailed.Summary.TranslatedMethods++
			}
		}
	}

	if len(detailed.Diagnostics) > 0 {
		payload, marshalErr := json.MarshalIndent(struct {
			Version     int                    `json:"version"`
			Status      string                 `json:"status"`
			Summary     Report                 `json:"summary"`
			Diagnostics []ConversionDiagnostic `json:"diagnostics"`
		}{Version: 1, Status: "partial", Summary: detailed.Summary, Diagnostics: detailed.Diagnostics}, "", "  ")
		if marshalErr != nil {
			return nil, detailed, fmt.Errorf("dex2jar: encode diagnostics: %w", marshalErr)
		}
		payload = append(payload, '\n')
		entries = append(entries, jarEntry{name: "META-INF/javajive/diagnostics.json", data: payload})
	}
	jar, err := writeJar(entries)
	if err != nil {
		detailed.Diagnostics = append(detailed.Diagnostics, diagnosticFromError(err, "failed", ""))
		return nil, detailed, err
	}
	detailed.Status = "complete"
	if detailed.Summary.Warnings > 0 {
		detailed.Status = "partial"
	}
	return jar, detailed, nil
}

func methodDescriptorForReport(file *model.File, method *model.EncodedMethod) (string, error) {
	if int(method.Method.ProtoIndex) >= len(file.Protos) {
		return "", fmt.Errorf("proto index out of range")
	}
	proto := file.Protos[method.Method.ProtoIndex]
	var descriptor strings.Builder
	descriptor.WriteByte('(')
	for _, parameter := range proto.Parameters {
		descriptor.WriteString(parameter)
	}
	descriptor.WriteByte(')')
	descriptor.WriteString(proto.ReturnType)
	return descriptor.String(), nil
}

func diagnosticFromError(err error, action, feature string) ConversionDiagnostic {
	out := ConversionDiagnostic{Category: "invalid", Action: action, Feature: feature, Severity: "error", Offset: -1, Message: err.Error()}
	if failure, ok := dexreport.AsFailure(err); ok {
		out.Code = failure.Code
		out.Kind = string(failure.Kind)
		out.Scope = string(failure.Scope)
		out.Stage = string(failure.Stage)
		out.Severity = string(failure.Severity)
		out.Entry = failure.Entry
		out.Class = failure.Class
		out.Method = failure.Member
		out.Descriptor = failure.Descriptor
		out.Offset = failure.Offset
		out.Op = failure.Op
		if failure.Feature != "" {
			out.Feature = failure.Feature
		}
		if action == "" || action == "failed" && failure.Action != "" {
			out.Action = string(failure.Action)
		}
		if failure.Kind == dexreport.KindUnsupported {
			out.Category = "unsupported"
		}
	}
	if dexreport.IsUnsupported(err) {
		out.Category = "unsupported"
		var capability *dexreport.CapabilityError
		if errors.As(err, &capability) {
			out.Feature = capability.Feature
		}
	}
	if diagnostic, ok := dexreport.AsDiagnostic(err); ok {
		if out.Entry == "" {
			out.Entry = diagnostic.Entry
		}
		if out.Class == "" {
			out.Class = diagnostic.Class
		}
		if out.Method == "" {
			out.Method = diagnostic.Method
		}
		if out.Offset < 0 {
			out.Offset = diagnostic.Offset
		}
		if out.Op == "" {
			out.Op = diagnostic.Op
		}
	}
	return out
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
func recoverMinimalDependencies(classes []convertedClass, hierarchy map[string]*model.Class) error {
	unsafeDependency := make(map[string]bool)
	for index := range classes {
		if classes[index].result.ClassMinimal {
			unsafeDependency[classes[index].class.Descriptor] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for index := range classes {
			item := &classes[index]
			if unsafeDependency[item.class.Descriptor] {
				continue
			}
			dependency := ""
			if unsafeDependency[item.class.Superclass] {
				dependency = item.class.Superclass
			} else {
				for _, descriptor := range item.class.Interfaces {
					if unsafeDependency[descriptor] {
						dependency = descriptor
						break
					}
				}
			}
			if dependency == "" {
				continue
			}
			if !item.result.ClassStubbed {
				result, err := lower.ClassWithPolicy(item.file, item.class, lower.ClassOptions{
					FileValidated: true, Hierarchy: hierarchy, ForceClassShell: true,
					RecoveryReason: fmt.Sprintf("dependency %s was reduced to a minimal class shell", dependency),
				}, lower.Recover)
				if err != nil {
					return err
				}
				item.result = result
			}
			unsafeDependency[item.class.Descriptor] = true
			changed = true
		}
	}
	return nil
}

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
