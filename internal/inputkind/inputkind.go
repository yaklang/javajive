// Package inputkind classifies supported Java and Android input containers by
// content magic first and archive structure second. Extensions are only a
// fallback, so renamed artifacts keep working.
package inputkind

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Kind identifies a supported input container.
type Kind string

const (
	Unknown Kind = "unknown"
	Class   Kind = "class"
	DEX     Kind = "dex"
	JMOD    Kind = "jmod"
	JIMAGE  Kind = "jimage"
	JAR     Kind = "jar"
	WAR     Kind = "war"
	EAR     Kind = "ear"
	AAR     Kind = "aar"
	PAR     Kind = "par"
	ZIP     Kind = "zip"
	APK     Kind = "apk"
	APKS    Kind = "apks"
	XAPK    Kind = "xapk"
	APKM    Kind = "apkm"
	AAB     Kind = "aab"
)

// JMODHeaderSize is the size of the JMOD prefix before the embedded ZIP.
const JMODHeaderSize = 4

var (
	rootDEXPattern = regexp.MustCompile(`^classes([0-9]*)\.dex$`)
	aabDEXPattern  = regexp.MustCompile(`^([^/]+)/dex/classes([0-9]*)\.dex$`)
)

// IsAndroid reports whether kind is an Android code container that the DEX
// pipeline can translate.
func IsAndroid(kind Kind) bool {
	switch kind {
	case APK, APKS, XAPK, APKM, AAB:
		return true
	default:
		return false
	}
}

// IsJavaArchive reports whether kind should be exported by the Java archive
// decompiler. ZIP is included because a generic ZIP may contain classes or
// nested JARs.
func IsJavaArchive(kind Kind) bool {
	switch kind {
	case JMOD, JIMAGE, JAR, WAR, EAR, AAR, PAR, ZIP:
		return true
	default:
		return false
	}
}

// IsJMODHeader reports whether data starts with the JMOD magic prefix
// "JM\x01\x00" that precedes the embedded ZIP.
func IsJMODHeader(data []byte) bool {
	return len(data) >= JMODHeaderSize &&
		data[0] == 'J' && data[1] == 'M' && data[2] == 1 && data[3] == 0
}

// JImageMagic is the JImage header marker. JDK images use native endianness,
// so the marker is checked in both byte orders.
const JImageMagic = 0xCAFEDADA

// IsJImageHeader reports whether data starts with a JImage magic in either
// byte order.
func IsJImageHeader(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	return binary.LittleEndian.Uint32(data[:4]) == JImageMagic ||
		binary.BigEndian.Uint32(data[:4]) == JImageMagic
}

// StripJMODHeader returns the embedded ZIP bytes when data is a JMOD, and data
// unchanged otherwise.
func StripJMODHeader(data []byte) []byte {
	if IsJMODHeader(data) {
		return data[JMODHeaderSize:]
	}
	return data
}

// DetectMagic classifies only the first bytes of an input. It never opens a
// ZIP central directory, so callers may pass an 8-byte prefix.
func DetectMagic(data []byte) Kind {
	switch {
	case hasDEXMagic(data):
		return DEX
	case IsJMODHeader(data):
		return JMOD
	case IsJImageHeader(data):
		return JIMAGE
	case hasZIPMagic(data):
		return ZIP
	case hasClassMagic(data):
		return Class
	default:
		return Unknown
	}
}

// DetectBytes classifies a complete input by magic, archive structure, and
// finally the file name extension.
func DetectBytes(data []byte, name string) Kind {
	return detect(bytes.NewReader(data), int64(len(data)), name)
}

// DetectReaderAt classifies an input without requiring it to be held in memory.
// The reader must remain valid for the duration of the call.
func DetectReaderAt(r io.ReaderAt, size int64, name string) (Kind, error) {
	return detectWithError(r, size, name)
}

// DetectFile classifies a file on disk without loading the whole archive.
func DetectFile(name string) (Kind, error) {
	file, err := os.Open(name)
	if err != nil {
		return Unknown, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Unknown, err
	}
	return detectWithError(file, info.Size(), name)
}

func detect(r io.ReaderAt, size int64, name string) Kind {
	kind, _ := detectWithError(r, size, name)
	return kind
}

func detectWithError(r io.ReaderAt, size int64, name string) (Kind, error) {
	var header [8]byte
	n, err := r.ReadAt(header[:], 0)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return Unknown, err
	}
	head := header[:n]
	switch DetectMagic(head) {
	case DEX:
		return DEX, nil
	case Class:
		return Class, nil
	case JMOD:
		return JMOD, nil
	case JIMAGE:
		return JIMAGE, nil
	case ZIP:
		return detectZIP(r, size, name)
	default:
		return kindFromExtension(name), nil
	}
}

func detectZIP(r io.ReaderAt, size int64, name string) (Kind, error) {
	reader, err := zip.NewReader(r, size)
	if err != nil {
		return kindFromExtension(name), nil
	}

	var hasRootDEX, hasAABDEX, hasNestedAPK, hasClass, hasManifest bool
	var hasWAR, hasEAR, hasAndroidManifest, hasClassesJAR bool
	var hasTOC, hasManifestJSON, hasInfoJSON, hasBundleConfig bool

	for _, file := range reader.File {
		entry := cleanEntryName(file.Name)
		lower := strings.ToLower(entry)
		if lower == "" {
			continue
		}
		if _, ok := RootDEXIndex(entry); ok {
			hasRootDEX = true
		}
		if _, _, ok := AABDEX(entry); ok {
			hasAABDEX = true
		}
		if !file.FileInfo().IsDir() && strings.HasSuffix(lower, ".apk") {
			hasNestedAPK = true
		}
		if strings.HasSuffix(lower, ".class") {
			hasClass = true
		}
		switch lower {
		case "meta-inf/manifest.mf":
			hasManifest = true
		case "web-inf/web.xml":
			hasWAR = true
		case "meta-inf/application.xml":
			hasEAR = true
		case "androidmanifest.xml":
			hasAndroidManifest = true
		case "classes.jar":
			hasClassesJAR = true
		case "toc.pb":
			hasTOC = true
		case "manifest.json":
			hasManifestJSON = true
		case "info.json":
			hasInfoJSON = true
		case "bundleconfig.pb":
			hasBundleConfig = true
		}
		if strings.HasPrefix(lower, "web-inf/classes/") ||
			strings.HasPrefix(lower, "web-inf/lib/") ||
			strings.HasPrefix(lower, "web-inf/") {
			hasWAR = true
		}
		if strings.HasPrefix(lower, "ear/lib/") {
			hasEAR = true
		}
	}

	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case hasRootDEX:
		return APK, nil
	case hasAABDEX, hasBundleConfig && ext == ".aab":
		return AAB, nil
	case hasNestedAPK && !hasClass:
		switch {
		case hasManifestJSON:
			return XAPK, nil
		case hasInfoJSON:
			return APKM, nil
		case hasTOC:
			return APKS, nil
		}
		switch ext {
		case ".xapk", ".apkx":
			return XAPK, nil
		case ".apkm":
			return APKM, nil
		case ".apks":
			return APKS, nil
		}
		return APKS, nil
	case hasWAR:
		return WAR, nil
	case hasEAR:
		return EAR, nil
	case hasAndroidManifest && hasClassesJAR:
		return AAR, nil
	case hasManifest || hasClass:
		return JAR, nil
	default:
		if kind := kindFromExtension(name); kind != Unknown {
			return kind, nil
		}
		return ZIP, nil
	}
}

// RootDEXIndex reports the canonical multidex index of a root-level
// classes*.dex entry. Non-canonical names are rejected.
func RootDEXIndex(name string) (int, bool) {
	entry := cleanEntryName(name)
	match := rootDEXPattern.FindStringSubmatch(entry)
	if match == nil {
		return 0, false
	}
	return multidexIndex(match[1])
}

// AABDEX reports the module name and canonical multidex index of a
// <module>/dex/classes*.dex entry.
func AABDEX(name string) (module string, index int, ok bool) {
	entry := cleanEntryName(name)
	match := aabDEXPattern.FindStringSubmatch(entry)
	if match == nil {
		return "", 0, false
	}
	index, ok = multidexIndex(match[2])
	if !ok {
		return "", 0, false
	}
	return match[1], index, true
}

func multidexIndex(digits string) (int, bool) {
	if digits == "" {
		return 1, true
	}
	if digits[0] == '0' {
		return 0, false
	}
	index, err := strconv.Atoi(digits)
	if err != nil || index < 2 {
		return 0, false
	}
	return index, true
}

func kindFromExtension(name string) Kind {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".class":
		return Class
	case ".dex":
		return DEX
	case ".jmod":
		return JMOD
	case ".jimage":
		return JIMAGE
	case ".jar":
		return JAR
	case ".war":
		return WAR
	case ".ear":
		return EAR
	case ".aar":
		return AAR
	case ".par":
		return PAR
	case ".apk":
		return APK
	case ".apks":
		return APKS
	case ".xapk", ".apkx":
		return XAPK
	case ".apkm":
		return APKM
	case ".aab":
		return AAB
	case ".zip":
		return ZIP
	default:
		return Unknown
	}
}

func cleanEntryName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	name = path.Clean(name)
	if name == "." {
		return ""
	}
	return name
}

func hasDEXMagic(data []byte) bool {
	return len(data) >= 8 && string(data[:4]) == "dex\n" && data[7] == 0 &&
		data[4] >= '0' && data[4] <= '9' &&
		data[5] >= '0' && data[5] <= '9' &&
		data[6] >= '0' && data[6] <= '9'
}

func hasZIPMagic(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' &&
		((data[2] == 3 && data[3] == 4) ||
			(data[2] == 5 && data[3] == 6) ||
			(data[2] == 7 && data[3] == 8))
}

func hasClassMagic(data []byte) bool {
	return len(data) >= 4 &&
		data[0] == 0xca && data[1] == 0xfe && data[2] == 0xba && data[3] == 0xbe
}
