// Package javajive is the unified, single-import entry point for the portable,
// pure-Go Java toolkit. It aggregates the three building blocks of the project
// behind one small, stable API:
//
//   - Decompilation: turn .class bytes or .jar/.war/.zip archives into Java source.
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
	"strings"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/classparser/jarwar"
	yserx "github.com/yaklang/javajive/serialization"
)

// Version is the javajive library/CLI version.
const Version = "0.1.0"

// ---------------------------------------------------------------------------
// Re-exported types
// ---------------------------------------------------------------------------

// ClassObject is the parsed representation of a single Java .class file.
type ClassObject = classparser.ClassObject

// JavaSerializable is one node of a parsed Java serialization stream.
type JavaSerializable = yserx.JavaSerializable

// ---------------------------------------------------------------------------
// Decompilation
// ---------------------------------------------------------------------------

// Decompile decompiles the bytes of a single Java .class file into Java source.
func Decompile(classBytes []byte) (string, error) {
	return classparser.Decompile(classBytes)
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

// DecompileArchive decompiles every class inside a .jar/.war/.zip archive at src
// and writes the resulting .java files into the dst directory tree.
func DecompileArchive(src, dst string) error {
	return jarwar.AutoDecompile(src, dst)
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
