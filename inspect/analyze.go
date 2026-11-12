// Package inspect provides read-only, best-effort inventory and DEX analysis
// for Java and Android artifacts. It never translates DEX input or emits a JAR.
package inspect

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/javajive/dex2jar/input"
	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/lower"
	"github.com/yaklang/javajive/dex2jar/model"
	dexreport "github.com/yaklang/javajive/dex2jar/report"
	"github.com/yaklang/javajive/internal/inputkind"
	"github.com/yaklang/javajive/jimage"
)

const schemaVersion = 1
const maxArchiveEntrySize = int64(256 << 20)

// Status describes how much of an artifact was understood.
type Status string

const (
	StatusComplete    Status = "complete"
	StatusPartial     Status = "partial"
	StatusUnsupported Status = "unsupported"
	StatusFailed      Status = "failed"
)

// Options controls report detail. The zero value disables disassembly.
type Options struct {
	// Disassembly includes per-instruction DEX disassembly and raw code units.
	// Disable it for large applications where the report would otherwise reach
	// hundreds of megabytes; inventory, verdicts and diagnostics are unchanged.
	Disassembly bool
}

// DefaultOptions is the full-detail inspection policy.
var DefaultOptions = Options{Disassembly: true}

// Report is the stable, JSON-oriented result of one inspection.
type Report struct {
	SchemaVersion int          `json:"schema_version"`
	Status        Status       `json:"status"`
	Artifact      Artifact     `json:"artifact"`
	Archive       *Archive     `json:"archive,omitempty"`
	DEXFiles      []DEXFile    `json:"dex_files"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
}

// Artifact identifies the input bytes and their detected format.
type Artifact struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Magic  string `json:"magic"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Archive contains a sorted ZIP inventory. Resources exclude directories,
// class files, and canonical APK multidex entries.
type Archive struct {
	Entries   []ArchiveEntry `json:"entries"`
	Resources []ArchiveEntry `json:"resources"`
}

// ArchiveEntry describes the uncompressed content of one ZIP entry.
type ArchiveEntry struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Size            uint64 `json:"size"`
	CompressedSize  uint64 `json:"compressed_size"`
	Compression     uint16 `json:"compression"`
	CRC32           uint32 `json:"crc32"`
	SHA256          string `json:"sha256"`
	Directory       bool   `json:"directory"`
	ArchivePosition int    `json:"archive_position"`
}

// DEXFile is one standalone or APK-embedded DEX and its parsed classes.
type DEXFile struct {
	Status      Status        `json:"status"`
	Provenance  DEXProvenance `json:"provenance"`
	Version     string        `json:"version"`
	Size        uint64        `json:"size"`
	SHA256      string        `json:"sha256"`
	Classes     []Class       `json:"classes"`
	Diagnostics []Diagnostic  `json:"diagnostics"`
}

// DEXProvenance records the canonical multidex identity of DEX bytes.
type DEXProvenance struct {
	Container     string `json:"container"`
	Entry         string `json:"entry"`
	MultidexIndex int    `json:"multidex_index"`
	Canonical     bool   `json:"canonical"`
}

// Class is a deterministic class and method inventory.
type Class struct {
	Descriptor  string      `json:"descriptor"`
	AccessFlags uint32      `json:"access_flags"`
	Superclass  string      `json:"superclass"`
	SourceFile  string      `json:"source_file"`
	Methods     []Method    `json:"methods"`
	Translation Translation `json:"translation"`
}

// Translation is the strict per-class translator verdict. A class marked
// unsupported is still fully inventoried and disassembled.
type Translation struct {
	Status     string `json:"status"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

// Method includes raw code units and deterministic decoder output when code is
// present. A method may be partially disassembled when strict decoding fails.
type Method struct {
	Name              string        `json:"name"`
	Descriptor        string        `json:"descriptor"`
	AccessFlags       uint32        `json:"access_flags"`
	Registers         uint16        `json:"registers"`
	Inputs            uint16        `json:"inputs"`
	Outputs           uint16        `json:"outputs"`
	CodeUnits         []string      `json:"code_units"`
	DisassemblyStatus string        `json:"disassembly_status"`
	Instructions      []Instruction `json:"instructions"`
	Diagnostics       []Diagnostic  `json:"diagnostics"`
}

// Instruction preserves DEX code-unit offsets, decoder metadata, raw words,
// and decoded operands.
type Instruction struct {
	Offset           uint32   `json:"offset"`
	Opcode           string   `json:"opcode"`
	Name             string   `json:"name"`
	Format           string   `json:"format"`
	Width            uint16   `json:"width"`
	DecoderSupported bool     `json:"decoder_supported"`
	Raw              []string `json:"raw"`
	Registers        []uint16 `json:"registers"`
	Index            *uint32  `json:"index,omitempty"`
	Literal          *int64   `json:"literal,omitempty"`
	Target           *int64   `json:"target,omitempty"`
}

// Diagnostic is a machine-readable failure or limitation with DEX context.
type Diagnostic struct {
	Source  string `json:"source"`
	Entry   string `json:"entry"`
	Class   string `json:"class"`
	Method  string `json:"method"`
	Offset  int64  `json:"offset"`
	Op      string `json:"op"`
	Message string `json:"message"`
}

// AnalyzeFile reads and inspects path. Read failures return a failed report and
// the underlying error; recognized but unsupported content is represented only
// in the report.
func AnalyzeFile(path string) (Report, error) {
	return AnalyzeFileWithOptions(path, DefaultOptions)
}

// AnalyzeFileWithOptions is AnalyzeFile with an explicit detail policy.
func AnalyzeFileWithOptions(path string, options Options) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		report := newReport(path, nil)
		report.Status = StatusFailed
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Source: "input", Offset: -1, Op: "read", Message: err.Error()})
		return report, err
	}
	return AnalyzeWithOptions(data, path, options), nil
}

// Analyze inspects data using name only as source identity and an archive-kind
// hint. Detection always begins with content magic.
func Analyze(data []byte, name string) Report {
	return AnalyzeWithOptions(data, name, DefaultOptions)
}

// AnalyzeWithOptions inspects data with an explicit detail policy.
func AnalyzeWithOptions(data []byte, name string, options Options) Report {
	report := newReport(name, data)
	switch report.Artifact.Magic {
	case "dex":
		report.Artifact.Format = "dex"
		report.DEXFiles = append(report.DEXFiles, analyzeDEX(data, nil, DEXProvenance{
			Container: "standalone", Entry: "classes.dex", MultidexIndex: 1, Canonical: false,
		}, options))
		report.Status = report.DEXFiles[0].Status
		report.Diagnostics = append(report.Diagnostics, report.DEXFiles[0].Diagnostics...)
	case "jmod":
		report.Artifact.Format = "jmod"
		if len(data) <= inputkind.JMODHeaderSize {
			report.Status = StatusFailed
			report.Diagnostics = append(report.Diagnostics, Diagnostic{Source: "jmod", Offset: 0, Op: "header", Message: "truncated JMOD header"})
			break
		}
		analyzeArchive(&report, data[inputkind.JMODHeaderSize:], name, inputkind.JMOD, options)
	case "jimage":
		report.Artifact.Format = "jimage"
		analyzeJImage(&report, data)
	case "zip":
		analyzeArchive(&report, data, name, inputkind.Unknown, options)
	case "class":
		report.Artifact.Format = "class"
		report.Status = StatusUnsupported
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Source: "inspect", Offset: 0, Op: "format", Message: "class-file semantic inspection is outside this analysis slice"})
	default:
		report.Artifact.Format = "unknown"
		report.Status = StatusUnsupported
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Source: "inspect", Offset: 0, Op: "magic", Message: "unsupported input magic"})
	}
	return report
}

func newReport(name string, data []byte) Report {
	sum := sha256.Sum256(data)
	return Report{
		SchemaVersion: schemaVersion,
		Status:        StatusComplete,
		Artifact: Artifact{
			Name: name, Magic: detectMagic(data), Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]),
		},
		DEXFiles:    []DEXFile{},
		Diagnostics: []Diagnostic{},
	}
}

func detectMagic(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:4], []byte("dex\n")) && data[7] == 0:
		return "dex"
	case inputkind.IsJMODHeader(data):
		return "jmod"
	case inputkind.IsJImageHeader(data):
		return "jimage"
	case len(data) >= 4 && (bytes.Equal(data[:4], []byte{'P', 'K', 3, 4}) || bytes.Equal(data[:4], []byte{'P', 'K', 5, 6}) || bytes.Equal(data[:4], []byte{'P', 'K', 7, 8})):
		return "zip"
	case len(data) >= 4 && bytes.Equal(data[:4], []byte{0xca, 0xfe, 0xba, 0xbe}):
		return "class"
	default:
		return "unknown"
	}
}

func analyzeArchive(report *Report, data []byte, name string, forced inputkind.Kind, options Options) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		report.Artifact.Format = archiveHint(name)
		if forced != inputkind.Unknown {
			report.Artifact.Format = string(forced)
		}
		report.Status = StatusFailed
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Source: "archive", Offset: -1, Op: "zip", Message: err.Error()})
		return
	}

	kind := forced
	if kind == inputkind.Unknown {
		kind = inputkind.DetectBytes(data, name)
	}
	report.Artifact.Format = string(kind)

	archive := &Archive{Entries: []ArchiveEntry{}, Resources: []ArchiveEntry{}}
	report.Archive = archive
	for position, file := range reader.File {
		entry, _, readErr := inspectArchiveEntry(file, position)
		archive.Entries = append(archive.Entries, entry)
		if !entry.Directory && !strings.HasSuffix(strings.ToLower(file.Name), ".class") && !isDEXEntry(file.Name) {
			archive.Resources = append(archive.Resources, entry)
		}
		if readErr != nil {
			report.Status = StatusPartial
			report.Diagnostics = append(report.Diagnostics, Diagnostic{Source: "archive", Entry: file.Name, Offset: -1, Op: "read entry", Message: readErr.Error()})
		}
	}
	sortArchiveEntries(archive.Entries)
	sortArchiveEntries(archive.Resources)

	if !inputkind.IsAndroid(kind) {
		if kind == inputkind.ZIP {
			report.Status = StatusUnsupported
			report.Diagnostics = append(report.Diagnostics, Diagnostic{Source: "inspect", Offset: 0, Op: "archive type", Message: "ZIP is not identifiable as a Java archive or Android container"})
		}
		return
	}

	sources, _, err := input.CollectDEXSources(data, name, input.Options{})
	if err != nil {
		report.Status = StatusPartial
		report.Diagnostics = append(report.Diagnostics, diagnosticFromError("container", err))
		return
	}
	// Parse each DEX once. The previous hierarchy pass parsed every file and the
	// analysis pass parsed them again; hierarchy and per-class verdicts can share
	// the same immutable model.File.
	hierarchy := make(map[string]*model.Class)
	parsedSources := make([]parsedDEXSource, len(sources))
	for index, source := range sources {
		parsed, parseErr := input.ParseDEX(source.Data, source.Entry)
		parsedSources[index] = parsedDEXSource{source: source, file: parsed, err: parseErr}
		if parseErr != nil {
			continue
		}
		for position := range parsed.Classes {
			class := &parsed.Classes[position]
			if _, exists := hierarchy[class.Descriptor]; !exists {
				hierarchy[class.Descriptor] = class
			}
		}
	}
	for _, item := range parsedSources {
		source := item.source
		dex := analyzeParsedDEX(source.Data, item.file, item.err, hierarchy, DEXProvenance{
			Container: string(kind), Entry: source.Entry,
			MultidexIndex: source.MultidexIndex, Canonical: source.Canonical,
		}, options)
		report.DEXFiles = append(report.DEXFiles, dex)
		report.Diagnostics = append(report.Diagnostics, dex.Diagnostics...)
		if dex.Status != StatusComplete && report.Status == StatusComplete {
			report.Status = StatusPartial
		}
	}
}

func analyzeJImage(report *Report, data []byte) {
	reader, err := jimage.OpenBytes(data)
	if err != nil {
		report.Status = StatusFailed
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Source: "jimage", Offset: 0, Op: "header", Message: err.Error()})
		return
	}
	defer reader.Close()

	archive := &Archive{Entries: []ArchiveEntry{}, Resources: []ArchiveEntry{}}
	report.Archive = archive
	const diagnosticLimit = 64
	failures := 0
	for position, entry := range reader.Entries() {
		kind := "resource"
		if entry.Directory {
			kind = "directory"
		} else if entry.Extension == "class" {
			kind = "class"
		}
		compression := uint16(0)
		if entry.CompressedSize != 0 {
			compression = 8
		}
		item := ArchiveEntry{
			Name: entry.Name(), Kind: kind, Size: entry.UncompressedSize,
			CompressedSize: entry.CompressedSize, Compression: compression,
			Directory: entry.Directory, ArchivePosition: position,
		}
		if !entry.Directory {
			content, readErr := reader.Read(entry.Path)
			if readErr != nil {
				report.Status = StatusPartial
				failures++
				if failures <= diagnosticLimit {
					report.Diagnostics = append(report.Diagnostics, Diagnostic{Source: "jimage", Entry: entry.Name(), Offset: -1, Op: "read", Message: readErr.Error()})
				}
			} else {
				sum := sha256.Sum256(content)
				item.SHA256 = hex.EncodeToString(sum[:])
			}
		}
		archive.Entries = append(archive.Entries, item)
		if kind != "class" && kind != "directory" {
			archive.Resources = append(archive.Resources, item)
		}
	}
	if failures > diagnosticLimit {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Source: "jimage", Offset: -1, Op: "read", Message: fmt.Sprintf("%d additional JImage resources could not be read", failures-diagnosticLimit)})
	}
	sortArchiveEntries(archive.Entries)
	sortArchiveEntries(archive.Resources)
}

func isDEXEntry(name string) bool {
	if _, ok := inputkind.RootDEXIndex(name); ok {
		return true
	}
	_, _, ok := inputkind.AABDEX(name)
	return ok
}

func archiveHint(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".apk":
		return "apk"
	case ".apks":
		return "apks"
	case ".xapk", ".apkx":
		return "xapk"
	case ".apkm":
		return "apkm"
	case ".aab":
		return "aab"
	case ".war":
		return "war"
	case ".ear":
		return "ear"
	case ".aar":
		return "aar"
	case ".par":
		return "par"
	case ".jmod":
		return "jmod"
	case ".jimage":
		return "jimage"
	case ".jar":
		return "jar"
	default:
		return "zip"
	}
}

func inspectArchiveEntry(file *zip.File, position int) (ArchiveEntry, []byte, error) {
	entry := ArchiveEntry{
		Name: file.Name, Kind: archiveEntryKind(file.Name, file.FileInfo().IsDir()), Size: file.UncompressedSize64,
		CompressedSize: file.CompressedSize64, Compression: file.Method, CRC32: file.CRC32,
		Directory: file.FileInfo().IsDir(), ArchivePosition: position,
	}
	if file.UncompressedSize64 > uint64(maxArchiveEntrySize) {
		return entry, nil, fmt.Errorf("uncompressed size %d exceeds inspection limit %d", file.UncompressedSize64, maxArchiveEntrySize)
	}
	stream, err := file.Open()
	if err != nil {
		return entry, nil, err
	}
	defer stream.Close()
	contents, err := io.ReadAll(io.LimitReader(stream, maxArchiveEntrySize+1))
	if err != nil {
		return entry, nil, err
	}
	if int64(len(contents)) > maxArchiveEntrySize {
		return entry, nil, fmt.Errorf("decompressed data exceeds inspection limit %d", maxArchiveEntrySize)
	}
	sum := sha256.Sum256(contents)
	entry.SHA256 = hex.EncodeToString(sum[:])
	return entry, contents, nil
}

func archiveEntryKind(name string, directory bool) string {
	if directory {
		return "directory"
	}
	lower := strings.ToLower(name)
	if isDEXEntry(name) {
		return "dex"
	}
	if strings.HasSuffix(lower, ".class") {
		return "class"
	}
	if strings.HasSuffix(lower, ".apk") {
		return "apk"
	}
	if strings.HasSuffix(lower, ".jar") || strings.HasSuffix(lower, ".aar") ||
		strings.HasSuffix(lower, ".war") || strings.HasSuffix(lower, ".ear") ||
		strings.HasSuffix(lower, ".par") || strings.HasSuffix(lower, ".jmod") {
		return "archive"
	}
	return "resource"
}

func sortArchiveEntries(entries []ArchiveEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].ArchivePosition < entries[j].ArchivePosition
	})
}

type parsedDEXSource struct {
	source input.DEXSource
	file   *model.File
	err    error
}

func analyzeDEX(data []byte, hierarchy map[string]*model.Class, provenance DEXProvenance, options Options) DEXFile {
	parsed, err := input.ParseDEX(data, provenance.Entry)
	return analyzeParsedDEX(data, parsed, err, hierarchy, provenance, options)
}

func analyzeParsedDEX(data []byte, parsed *model.File, parseErr error, hierarchy map[string]*model.Class, provenance DEXProvenance, options Options) DEXFile {
	sum := sha256.Sum256(data)
	dex := DEXFile{
		Status: StatusComplete, Provenance: provenance, Version: dexVersion(data), Size: uint64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), Classes: []Class{}, Diagnostics: []Diagnostic{},
	}
	if parseErr != nil {
		dex.Status = StatusPartial
		dex.Diagnostics = append(dex.Diagnostics, diagnosticFromError("strict-parser", parseErr))
		return dex
	}
	classes := append([]model.Class(nil), parsed.Classes...)
	sort.SliceStable(classes, func(i, j int) bool { return classes[i].Descriptor < classes[j].Descriptor })
	// Validate the file once; the per-class verdicts below then skip the
	// per-method whole-file validation.
	fileErr := ir.ValidateFile(parsed)
	for classIndex := range classes {
		classModel := &classes[classIndex]
		class := Class{
			Descriptor: classModel.Descriptor, AccessFlags: classModel.AccessFlags, Superclass: classModel.Superclass,
			SourceFile: classModel.SourceFile, Methods: []Method{},
			Translation: analyzeTranslation(parsed, classModel, fileErr, hierarchy),
		}
		methods := append([]model.EncodedMethod(nil), classModel.DirectMethods...)
		methods = append(methods, classModel.VirtualMethods...)
		sort.SliceStable(methods, func(i, j int) bool {
			left, right := methodDescriptor(parsed, &methods[i]), methodDescriptor(parsed, &methods[j])
			if methods[i].Method.Name != methods[j].Method.Name {
				return methods[i].Method.Name < methods[j].Method.Name
			}
			if left != right {
				return left < right
			}
			return methods[i].MethodIndex < methods[j].MethodIndex
		})
		for methodIndex := range methods {
			method := analyzeMethod(parsed, classModel, &methods[methodIndex], options)
			if len(method.Diagnostics) > 0 {
				dex.Status = StatusPartial
				dex.Diagnostics = append(dex.Diagnostics, method.Diagnostics...)
			}
			class.Methods = append(class.Methods, method)
		}
		dex.Classes = append(dex.Classes, class)
	}
	return dex
}

// analyzeTranslation reports whether the strict translator can recover this
// class on its own. It never emits a classfile: the verdict tells a reverse
// engineer which classes are recoverable and why the others are not.
func analyzeTranslation(file *model.File, class *model.Class, fileErr error, hierarchy map[string]*model.Class) Translation {
	err := fileErr
	if err == nil {
		_, err = lower.ClassWithOptions(file, class, lower.ClassOptions{FileValidated: true, Hierarchy: hierarchy})
	}
	if err != nil {
		message := err.Error()
		var diagnostic *dexreport.Diagnostic
		if errors.As(err, &diagnostic) && diagnostic.Err != nil {
			message = diagnostic.Err.Error()
		}
		return Translation{Status: "unsupported", Diagnostic: message}
	}
	return Translation{Status: "complete"}
}

func dexVersion(data []byte) string {
	if len(data) >= 8 && bytes.Equal(data[:4], []byte("dex\n")) && data[7] == 0 {
		return string(data[4:7])
	}
	return ""
}

func methodDescriptor(file *model.File, method *model.EncodedMethod) string {
	if method == nil || int(method.Method.ProtoIndex) >= len(file.Protos) {
		return ""
	}
	proto := file.Protos[method.Method.ProtoIndex]
	return "(" + strings.Join(proto.Parameters, "") + ")" + proto.ReturnType
}

func analyzeMethod(file *model.File, class *model.Class, encoded *model.EncodedMethod, options Options) Method {
	method := Method{
		Name: encoded.Method.Name, Descriptor: methodDescriptor(file, encoded), AccessFlags: encoded.AccessFlags,
		CodeUnits: []string{}, Instructions: []Instruction{}, Diagnostics: []Diagnostic{}, DisassemblyStatus: "not_applicable",
	}
	if encoded.Code == nil {
		return method
	}
	method.Registers = encoded.Code.RegistersSize
	method.Inputs = encoded.Code.InsSize
	method.Outputs = encoded.Code.OutsSize
	if !options.Disassembly {
		method.DisassemblyStatus = "skipped"
		return method
	}
	method.CodeUnits = formatWords(encoded.Code.Instructions)
	ctx := ir.Context{Entry: file.Entry, Class: class.Descriptor, Method: encoded.Method.Name}
	decoded, err := ir.Decode(encoded.Code, ctx)
	if err != nil {
		method.DisassemblyStatus = "partial"
		method.Instructions = metadataDisassembly(encoded.Code.Instructions)
		method.Diagnostics = append(method.Diagnostics, diagnosticFromError("decoder", err))
		return method
	}
	method.DisassemblyStatus = "complete"
	for _, instruction := range decoded {
		method.Instructions = append(method.Instructions, convertInstruction(instruction, encoded.Code.Instructions))
	}
	return method
}

func convertInstruction(instruction ir.Instruction, words []uint16) Instruction {
	_, _, _, supported, _ := ir.OpcodeInfo(instruction.Opcode)
	out := Instruction{
		Offset: instruction.Offset, Opcode: fmt.Sprintf("0x%02x", instruction.Opcode), Name: instruction.Name,
		Format: instruction.Format, Width: instruction.Width, DecoderSupported: supported,
		Raw: wordRange(words, instruction.Offset, instruction.Width), Registers: append([]uint16(nil), instruction.Regs...),
	}
	if formatHasIndex(instruction.Format) {
		value := instruction.Index
		out.Index = &value
	}
	if formatHasLiteral(instruction.Format) {
		value := instruction.Literal
		out.Literal = &value
	}
	if formatHasTarget(instruction.Format) {
		value := instruction.Target
		out.Target = &value
	}
	return out
}

func metadataDisassembly(words []uint16) []Instruction {
	out := make([]Instruction, 0, len(words))
	for offset := uint32(0); offset < uint32(len(words)); {
		word := words[offset]
		opcode := byte(word)
		name, format, width, supported, known := ir.OpcodeInfo(opcode)
		if opcode == 0 && word>>8 >= 1 && word>>8 <= 3 {
			name, format, width, supported, known = "payload", "payload", 1, false, true
		}
		if !known {
			name, format, width = "unknown", "unknown", 1
		}
		available := width
		if remaining := uint32(len(words)) - offset; uint32(available) > remaining {
			available = uint16(remaining)
		}
		out = append(out, Instruction{
			Offset: offset, Opcode: fmt.Sprintf("0x%02x", opcode), Name: name, Format: format, Width: width,
			DecoderSupported: supported, Raw: wordRange(words, offset, available), Registers: []uint16{},
		})
		if available == 0 {
			break
		}
		offset += uint32(available)
	}
	return out
}

func formatWords(words []uint16) []string {
	out := make([]string, len(words))
	for index, word := range words {
		out[index] = fmt.Sprintf("0x%04x", word)
	}
	return out
}

func wordRange(words []uint16, offset uint32, width uint16) []string {
	start := int(offset)
	end := start + int(width)
	if start < 0 || start > len(words) {
		return []string{}
	}
	if end > len(words) {
		end = len(words)
	}
	return formatWords(words[start:end])
}

func formatHasIndex(format string) bool {
	switch format {
	case "21c", "31c", "22c", "35c", "3rc", "45cc", "4rcc":
		return true
	default:
		return false
	}
}

func formatHasLiteral(format string) bool {
	switch format {
	case "11n", "21s", "31i", "51l", "21h", "22s", "22b":
		return true
	default:
		return false
	}
}

func formatHasTarget(format string) bool {
	switch format {
	case "10t", "20t", "30t", "21t", "22t", "31t":
		return true
	default:
		return false
	}
}

func diagnosticFromError(source string, err error) Diagnostic {
	diagnostic := Diagnostic{Source: source, Offset: -1, Message: err.Error()}
	if structured, ok := dexreport.AsDiagnostic(err); ok {
		diagnostic.Entry = structured.Entry
		diagnostic.Class = structured.Class
		diagnostic.Method = structured.Method
		diagnostic.Offset = structured.Offset
		diagnostic.Op = structured.Op
		if structured.Err != nil {
			diagnostic.Message = structured.Err.Error()
		}
	}
	return diagnostic
}
