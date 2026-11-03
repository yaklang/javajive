package dex2jar_test

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	javajive "github.com/yaklang/javajive"
	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar"
)

func TestDexToJarDeterministicAndParseable(t *testing.T) {
	raw := minimalDEX("Lexample/Minimal;")
	first, report, err := dex2jar.DexToJar(raw, dex2jar.Options{})
	if err != nil {
		t.Fatalf("DexToJar: %v", err)
	}
	second, secondReport, err := dex2jar.DexToJar(raw, dex2jar.Options{})
	if err != nil {
		t.Fatalf("second DexToJar: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("repeated conversion produced different JAR bytes")
	}
	if !reflect.DeepEqual(report, secondReport) {
		t.Fatalf("reports differ: %#v and %#v", report, secondReport)
	}
	if report.DEXFiles != 1 || report.Classes != 1 || report.Methods != 0 || report.TranslatedMethods != 0 || report.Warnings != 0 {
		t.Fatalf("unexpected report: %#v", report)
	}

	reader, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil {
		t.Fatalf("open JAR: %v", err)
	}
	wantNames := []string{"META-INF/MANIFEST.MF", "example/Minimal.class"}
	if len(reader.File) != len(wantNames) {
		t.Fatalf("JAR entries = %d, want %d", len(reader.File), len(wantNames))
	}
	for index, file := range reader.File {
		if file.Name != wantNames[index] {
			t.Fatalf("entry[%d] = %q, want %q", index, file.Name, wantNames[index])
		}
		if !file.Modified.Equal(time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("entry %q timestamp = %v", file.Name, file.Modified)
		}
		data := readZipFile(t, file)
		if strings.HasSuffix(file.Name, ".class") {
			parsed, err := classparser.Parse(data)
			if err != nil {
				t.Fatalf("parse %q: %v", file.Name, err)
			}
			if parsed.GetClassName() != "example/Minimal" {
				t.Fatalf("class name = %q", parsed.GetClassName())
			}
		} else if string(data) != "Manifest-Version: 1.0\r\n\r\n" {
			t.Fatalf("manifest = %q", data)
		}
	}
}

func TestDexToJarDetectsAPKByMagic(t *testing.T) {
	apk := makeAPK(t, minimalDEX("Lexample/FromAPK;"))
	jar, report, err := dex2jar.DexToJar(apk, dex2jar.Options{})
	if err != nil {
		t.Fatalf("DexToJar(APK): %v", err)
	}
	if len(jar) == 0 || report.DEXFiles != 1 || report.Classes != 1 {
		t.Fatalf("jar bytes=%d report=%#v", len(jar), report)
	}
}

func TestClassFailureReturnsNoJarAfterEarlierClassSucceeded(t *testing.T) {
	valid := minimalDEX("Lexample/First;")
	invalid := minimalDEX("Lexample/Second;")
	classDefsOff := binary.LittleEndian.Uint32(invalid[100:104])
	putU32(invalid, int(classDefsOff)+4, 0x80000001)
	apk := makeAPKEntries(t, map[string][]byte{
		"classes.dex":  valid,
		"classes2.dex": invalid,
	})

	jar, report, err := dex2jar.DexToJar(apk, dex2jar.Options{})
	if err == nil {
		t.Fatal("DexToJar accepted a class with unsupported access flags")
	}
	if jar != nil {
		t.Fatalf("DexToJar returned %d bytes after transactional failure", len(jar))
	}
	if report.DEXFiles != 2 || report.Classes != 1 {
		t.Fatalf("failure report = %#v", report)
	}
}

func TestDexFileToJarRejectsInputBeforeFullRead(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "large.dex")
	dst := filepath.Join(dir, "output.jar")
	data := append([]byte("dex\n035\x00"), make([]byte, 64)...)
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := dex2jar.DexFileToJar(src, dst, dex2jar.Options{MaxDEXSize: 8}); err == nil || !strings.Contains(err.Error(), "input size") {
		t.Fatalf("DexFileToJar size error = %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("output exists after input size rejection: %v", err)
	}
}

func TestUnsupportedInputReturnsNoProduct(t *testing.T) {
	jar, report, err := dex2jar.DexToJar([]byte("not a dex or apk"), dex2jar.Options{})
	if err == nil {
		t.Fatal("DexToJar accepted unsupported input")
	}
	if jar != nil {
		t.Fatalf("DexToJar returned %d bytes on failure", len(jar))
	}
	if report != (dex2jar.Report{}) {
		t.Fatalf("report = %#v, want zero", report)
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "bad.dex")
	dst := filepath.Join(dir, "output.jar")
	if err := os.WriteFile(src, []byte("not dex"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := []byte("existing product")
	if err := os.WriteFile(dst, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := dex2jar.DexFileToJar(src, dst, dex2jar.Options{}); err == nil {
		t.Fatal("DexFileToJar accepted unsupported input")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("destination changed on failure: %q", got)
	}
}

func TestRootAPIAndDecompileDEXFile(t *testing.T) {
	raw := minimalDEX("Lexample/RootAPI;")
	jar, report, err := javajive.DexToJar(raw, javajive.DexOptions{})
	if err != nil {
		t.Fatalf("javajive.DexToJar: %v", err)
	}
	if len(jar) == 0 || report.Classes != 1 {
		t.Fatalf("jar bytes=%d report=%#v", len(jar), report)
	}

	dir := t.TempDir()
	dexPath := filepath.Join(dir, "renamed.apk")
	outputDir := filepath.Join(dir, "source")
	if err := os.WriteFile(dexPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	fileJar := filepath.Join(dir, "from-file.jar")
	fileReport, err := javajive.DexFileToJar(dexPath, fileJar, javajive.DexOptions{})
	if err != nil {
		t.Fatalf("javajive.DexFileToJar: %v", err)
	}
	if fileReport.Classes != 1 {
		t.Fatalf("file report = %#v", fileReport)
	}
	if err := javajive.DecompileDEXFile(dexPath, outputDir, javajive.DexOptions{}); err != nil {
		t.Fatalf("DecompileDEXFile: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(outputDir, "example", "RootAPI.java"))
	if err != nil {
		t.Fatalf("read decompiled source: %v", err)
	}
	if !strings.Contains(string(source), "class RootAPI") {
		t.Fatalf("unexpected decompiled source:\n%s", source)
	}
}

func readZipFile(t *testing.T, file *zip.File) []byte {
	t.Helper()
	reader, err := file.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func makeAPK(t *testing.T, dex []byte) []byte {
	t.Helper()
	return makeAPKEntries(t, map[string][]byte{"classes.dex": dex})
}

func makeAPKEntries(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func minimalDEX(descriptor string) []byte {
	const headerSize = 0x70
	stringsTable := []string{descriptor, "Ljava/lang/Object;"}
	stringIDsOff := headerSize
	typeIDsOff := stringIDsOff + len(stringsTable)*4
	classDefsOff := typeIDsOff + len(stringsTable)*4
	dataOff := classDefsOff + 32
	data := make([]byte, dataOff)
	stringOffsets := make([]uint32, len(stringsTable))
	for index, value := range stringsTable {
		stringOffsets[index] = uint32(len(data))
		data = append(data, byte(len(value)))
		data = append(data, value...)
		data = append(data, 0)
	}

	copy(data[:8], []byte("dex\n035\x00"))
	putU32(data, 32, uint32(len(data)))
	putU32(data, 36, headerSize)
	putU32(data, 40, 0x12345678)
	putU32(data, 56, uint32(len(stringsTable)))
	putU32(data, 60, uint32(stringIDsOff))
	putU32(data, 64, uint32(len(stringsTable)))
	putU32(data, 68, uint32(typeIDsOff))
	putU32(data, 96, 1)
	putU32(data, 100, uint32(classDefsOff))
	putU32(data, 104, uint32(len(data)-dataOff))
	putU32(data, 108, uint32(dataOff))
	for index, offset := range stringOffsets {
		putU32(data, stringIDsOff+index*4, offset)
		putU32(data, typeIDsOff+index*4, uint32(index))
	}
	putU32(data, classDefsOff, 0)
	putU32(data, classDefsOff+4, 1)
	putU32(data, classDefsOff+8, 1)
	putU32(data, classDefsOff+16, ^uint32(0))
	return data
}

func putU32(data []byte, offset int, value uint32) {
	binary.LittleEndian.PutUint32(data[offset:offset+4], value)
}
