// Package javajive is the unified, single-import entry point for the portable,
// pure-Go Java toolkit. It aggregates the three building blocks of the project
// behind one small, stable API:
//
//   - Decompilation: turn .class bytes, Java archives (.jar/.war/.ear/.aar/
//     .par/.jmod/.zip), or JDK runtime images (.jimage / lib/modules) into Java
//     source.
//   - DEX translation: turn .dex/.apk/.apks/.xapk/.apkm/.aab input into a
//     deterministic JAR or directly into Java source.
//   - Class parsing: inspect the structure of a .class file (constant pool,
//     fields, methods, version, access flags).
//   - Java serialization: parse and re-marshal the Java ObjectStream wire format,
//     and convert it to/from JSON.
//
// Most callers only need this package:
//
//	import "github.com/yaklang/javajive"
//
//	src, err := javajive.Decompile(classBytes)
//	err = javajive.DecompileArchive("app.jar", "app-src")
//	objs, err := javajive.ParseSerialized(raw)
//
// The sub-packages (classparser, classparser/jarwar, serialization) remain
// available for advanced use.
package javajive

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/classparser/jarwar"
	nativedex "github.com/yaklang/javajive/dex2jar"
	"github.com/yaklang/javajive/jimage"
	yserx "github.com/yaklang/javajive/serialization"
)

// Version is the javajive library/CLI version.
const Version = "0.7.0"

// ---------------------------------------------------------------------------
// Re-exported types
// ---------------------------------------------------------------------------

// ClassObject is the parsed representation of a single Java .class file.
type ClassObject = classparser.ClassObject

// JavaSerializable is one node of a parsed Java serialization stream.
type JavaSerializable = yserx.JavaSerializable

// DexOptions controls native DEX and Android container translation.
type DexOptions = nativedex.Options

// DexReport summarizes a successful native DEX translation.
type DexReport = nativedex.Report

// DexConvertOptions selects strict or explicit best-effort translation.
type DexConvertOptions = nativedex.ConvertOptions

// DexDetailedReport includes complete/partial status and stub diagnostics.
type DexDetailedReport = nativedex.DetailedReport

type DexConversionDiagnostic = nativedex.ConversionDiagnostic
type DexFailurePolicy = nativedex.FailurePolicy

const (
	DexFailureStrict                 = nativedex.FailureStrict
	DexFailureStubUnsupportedMethods = nativedex.FailureStubUnsupportedMethods
)

// InputKind identifies reverse-toolchain input by content magic.
type InputKind = nativedex.InputKind

const (
	InputUnknown = nativedex.InputUnknown
	InputClass   = nativedex.InputClass
	InputDEX     = nativedex.InputDEX
	InputZIP     = nativedex.InputZIP
	// InputJMOD identifies a JMOD archive ("JM\x01\x00" + ZIP).
	InputJMOD = nativedex.InputJMOD
	// InputJIMAGE identifies a JDK modular runtime image.
	InputJIMAGE = nativedex.InputJIMAGE
)

// ContainerKind is the structure-aware classification of an input. It refines
// InputKind for ZIP containers (APK vs APKS vs JAR, and so on).
type ContainerKind = nativedex.ContainerKind

const (
	ContainerUnknown = nativedex.ContainerUnknown
	ContainerClass   = nativedex.ContainerClass
	ContainerDEX     = nativedex.ContainerDEX
	ContainerJMOD    = nativedex.ContainerJMOD
	ContainerJImage  = nativedex.ContainerJImage
	ContainerJAR     = nativedex.ContainerJAR
	ContainerWAR     = nativedex.ContainerWAR
	ContainerEAR     = nativedex.ContainerEAR
	ContainerAAR     = nativedex.ContainerAAR
	ContainerPAR     = nativedex.ContainerPAR
	ContainerZIP     = nativedex.ContainerZIP
	ContainerAPK     = nativedex.ContainerAPK
	ContainerAPKS    = nativedex.ContainerAPKS
	ContainerXAPK    = nativedex.ContainerXAPK
	ContainerAPKM    = nativedex.ContainerAPKM
	ContainerAAB     = nativedex.ContainerAAB
)

// ClassifyInput classifies data by magic, archive structure, and the supplied
// name's extension.
func ClassifyInput(data []byte, name string) ContainerKind {
	return nativedex.ClassifyInput(data, name)
}

// ClassifyInputFile classifies a file without loading the whole archive.
func ClassifyInputFile(name string) (ContainerKind, error) {
	return nativedex.ClassifyInputFile(name)
}

// IsAndroidContainer reports whether kind is translated by the DEX pipeline.
func IsAndroidContainer(kind ContainerKind) bool {
	return nativedex.IsAndroidContainer(kind)
}

// IsJavaArchiveContainer reports whether kind is exported by the Java archive
// decompiler.
func IsJavaArchiveContainer(kind ContainerKind) bool {
	return nativedex.IsJavaArchiveContainer(kind)
}

// JImageReader reads a JDK modular runtime image (lib/modules or .jimage).
type JImageReader = jimage.Reader

// JImageEntry is one resource inside a JDK runtime image.
type JImageEntry = jimage.Entry

// JImageHeader is the fixed JImage header.
type JImageHeader = jimage.Header

// OpenJImageFile opens a JDK runtime image and keeps the file handle until the
// reader is closed.
func OpenJImageFile(path string) (*JImageReader, error) {
	return jimage.OpenFile(path)
}

// JImageToJar converts a JDK runtime image into a JAR. Class resources are
// placed at package paths; module-info.class and non-class resources keep their
// module prefix. The destination is replaced atomically after the complete JAR
// has been written.
func JImageToJar(src, dst string) error {
	reader, err := jimage.OpenFile(src)
	if err != nil {
		return err
	}
	defer reader.Close()

	dir := filepath.Dir(dst)
	temp, err := os.CreateTemp(dir, ".javajive-jimage-*.jar")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		if temp != nil {
			_ = temp.Close()
		}
		if cleanup {
			_ = os.Remove(tempName)
		}
	}()
	if err := reader.WriteZIP(temp); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		temp = nil
		return err
	}
	temp = nil
	if err := os.Rename(tempName, dst); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// ---------------------------------------------------------------------------
// Decompilation
// ---------------------------------------------------------------------------

// Decompile decompiles the bytes of a single Java .class file into Java source.
func Decompile(classBytes []byte) (string, error) {
	return classparser.Decompile(classBytes)
}

// DecompileOptions selects the source-recovery policy and optional class resolver.
type DecompileOptions = classparser.DecompileOptions
type DecompileResult = classparser.DecompileResult
type DecompileMode = classparser.DecompileMode
type Limits = classparser.Limits
type WorkBudget = classparser.WorkBudget

// ArchiveOptions controls archive export. Structured mode separates decompiled
// sources, resources, and preserved nested archives.
type ArchiveOptions = jarwar.ArchiveOptions

// DecompileArchiveOptions is the explicit name for ArchiveOptions.
type DecompileArchiveOptions = jarwar.DecompileArchiveOptions

type ArchiveReport = jarwar.ArchiveReport
type DecompileArchiveReport = jarwar.DecompileArchiveReport
type ArchiveEntryReport = jarwar.ArchiveEntryReport
type NestedArchiveReport = jarwar.NestedArchiveReport

const (
	Precision     = classparser.Precision
	Compatibility = classparser.Compatibility
)

func DecompileWithOptions(classBytes []byte, options DecompileOptions) (DecompileResult, error) {
	return classparser.DecompileWithOptions(classBytes, options)
}

// DecompileFile reads a single .class file from disk and decompiles it.
func DecompileFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return classparser.Decompile(data)
}

// DecompileWithResolver decompiles a single .class, using resolve to look up the
// bytes of referenced classes by their internal name (e.g. "java/lang/Object")
// when the decompiler needs richer type information.
func DecompileWithResolver(classBytes []byte, resolve func(internalName string) ([]byte, bool)) (string, error) {
	return classparser.DecompileWithResolver(classBytes, resolve)
}

// DecompileArchive decompiles every class inside a Java archive
// (.jar/.war/.ear/.aar/.par/.jmod/.zip) or JDK runtime image (.jimage /
// lib/modules) at src and writes the resulting .java files into the dst
// directory tree. Nested archive layers are decompiled in place; .jmod inputs
// have their four-byte header stripped automatically, and .jimage inputs are
// converted to a package-layout JAR view before decompilation.
func DecompileArchive(src, dst string) error {
	return jarwar.AutoDecompile(src, dst)
}

// DecompileArchiveWithAnalysisBudget is DecompileArchive with an explicit
// per-method analysis budget. Values above 1,000,000 opt into the scaled graph
// caps needed by very large generated methods; zero keeps the default policy.
func DecompileArchiveWithAnalysisBudget(src, dst string, maxAnalysisUpdates int) error {
	return jarwar.AutoDecompileWithAnalysisBudget(src, dst, maxAnalysisUpdates)
}

// DecompileArchiveWithOptions decompiles an archive with explicit export policy.
// Set Structured to preserve nested archive bytes separately from their expanded
// source and resource views.
func DecompileArchiveWithOptions(src, dst string, options ArchiveOptions) (ArchiveReport, error) {
	return jarwar.AutoDecompileWithOptions(src, dst, options)
}

// DetectInputKind classifies supported reverse-toolchain input by content magic.
func DetectInputKind(input []byte) InputKind {
	return nativedex.DetectInputKind(input)
}

// DexToJar converts raw DEX, APK, APKS/XAPK/APKM bundle, or AAB bytes into a
// deterministic JAR.
func DexToJar(input []byte, options DexOptions) ([]byte, DexReport, error) {
	return nativedex.DexToJar(input, options)
}

// DexToJarWithOptions converts input with an explicit failure policy. Partial
// success has a nil error and a detailed report identifying every stub.
func DexToJarWithOptions(input []byte, options DexConvertOptions) ([]byte, DexDetailedReport, error) {
	return nativedex.DexToJarWithOptions(input, options)
}

// DexFileToJar converts a raw DEX, APK, APKS/XAPK/APKM, or AAB file and replaces
// dst only after the complete JAR has been written to a temporary file in the
// same directory.
func DexFileToJar(src, dst string, options DexOptions) (DexReport, error) {
	return nativedex.DexFileToJar(src, dst, options)
}

// DexFileToJarWithOptions converts a file with explicit failure policy and
// atomically replaces dst on complete or explicitly partial success.
func DexFileToJarWithOptions(src, dst string, options DexConvertOptions) (DexDetailedReport, error) {
	return nativedex.DexFileToJarWithOptions(src, dst, options)
}

// DecompileDEXFile converts a raw DEX, APK, APKS/XAPK/APKM, or AAB through a
// private temporary JAR, decompiles that archive into dst, and always removes
// the temporary JAR. Archive decompilation may write part of dst before
// returning an error.
func DecompileDEXFile(src, dst string, options DexOptions) error {
	return DecompileDEXFileWithAnalysisBudget(src, dst, options, 0)
}

// DecompileDEXFileWithAnalysisBudget is DecompileDEXFile with an explicit
// per-method analysis budget for the translated JAR's decompilation.
func DecompileDEXFileWithAnalysisBudget(src, dst string, options DexOptions, maxAnalysisUpdates int) error {
	_, err := DecompileDEXFileWithPolicy(src, dst, DexConvertOptions{
		Input:  options,
		Policy: DexFailureStrict,
	}, maxAnalysisUpdates)
	return err
}

// DecompileDEXFileWithPolicy translates DEX/Android input with an explicit
// failure policy, decompiles the resulting private JAR, and returns the
// detailed translation report. The temporary JAR is always removed.
func DecompileDEXFileWithPolicy(src, dst string, options DexConvertOptions, maxAnalysisUpdates int) (DexDetailedReport, error) {
	tempDir, err := os.MkdirTemp("", "javajive-dex-decompile-*")
	if err != nil {
		return DexDetailedReport{Status: "failed"}, err
	}
	defer os.RemoveAll(tempDir)

	jarPath := filepath.Join(tempDir, "classes.jar")
	detailed, err := DexFileToJarWithOptions(src, jarPath, options)
	if err != nil {
		return detailed, err
	}
	if err := DecompileArchiveWithAnalysisBudget(jarPath, dst, maxAnalysisUpdates); err != nil {
		return detailed, err
	}
	return detailed, nil
}

// ---------------------------------------------------------------------------
// Class parsing
// ---------------------------------------------------------------------------

// ParseClass parses the bytes of a single Java .class file into a ClassObject.
func ParseClass(classBytes []byte) (*ClassObject, error) {
	return classparser.Parse(classBytes)
}

// ParseClassFile reads a single .class file from disk and parses it.
func ParseClassFile(path string) (*ClassObject, error) {
	return classparser.ParseFromFile(path)
}

// ---------------------------------------------------------------------------
// Java serialization
// ---------------------------------------------------------------------------

// ParseSerialized parses a raw Java serialization (ObjectStream) byte stream.
func ParseSerialized(raw []byte) ([]JavaSerializable, error) {
	return yserx.ParseJavaSerialized(raw)
}

// ParseSerializedHex parses a hex-encoded Java serialization byte stream.
// Surrounding whitespace in the hex string is ignored.
func ParseSerializedHex(hexStr string) ([]JavaSerializable, error) {
	return yserx.ParseHexJavaSerialized(strings.TrimSpace(hexStr))
}

// MarshalSerialized re-encodes parsed objects back into the Java serialization
// wire format.
func MarshalSerialized(objs ...JavaSerializable) []byte {
	return yserx.MarshalJavaObjects(objs...)
}

// MarshalSerializedHex is MarshalSerialized with a hex-encoded result.
func MarshalSerializedHex(objs ...JavaSerializable) string {
	return hex.EncodeToString(yserx.MarshalJavaObjects(objs...))
}

// SerializedToJSON converts parsed objects into an indented JSON representation.
func SerializedToJSON(objs ...JavaSerializable) ([]byte, error) {
	return yserx.ToJson(objs)
}

// SerializedFromJSON rebuilds objects from the JSON produced by SerializedToJSON.
func SerializedFromJSON(raw []byte) ([]JavaSerializable, error) {
	return yserx.FromJson(raw)
}
