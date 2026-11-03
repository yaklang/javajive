package dex2jar_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"hash/adler32"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	javajive "github.com/yaklang/javajive"
	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar"
)

func TestAcceptedRawDEXCorpus(t *testing.T) {
	fixture := filepath.Join("testdata", "accept", "arithmetic.dex")
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 8 || string(raw[:8]) != "dex\n035\x00" {
		t.Fatalf("fixture magic = %q", raw[:min(len(raw), 8)])
	}

	first, report, err := dex2jar.DexToJar(raw, dex2jar.Options{})
	if err != nil {
		t.Fatalf("DexToJar: %v", err)
	}
	second, secondReport, err := dex2jar.DexToJar(raw, dex2jar.Options{})
	if err != nil {
		t.Fatalf("second DexToJar: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("conversion is not byte-for-byte deterministic")
	}
	if report != secondReport {
		t.Fatalf("reports differ: %#v and %#v", report, secondReport)
	}
	wantReport := dex2jar.Report{DEXFiles: 1, Classes: 1, Methods: 5, TranslatedMethods: 5}
	if report != wantReport {
		t.Fatalf("report = %#v, want %#v", report, wantReport)
	}

	classes := readAcceptedJar(t, first, []string{"META-INF/MANIFEST.MF", "accept/Arithmetic.class"})
	parsed, err := classparser.Parse(classes["accept/Arithmetic.class"])
	if err != nil {
		t.Fatalf("parse class: %v", err)
	}
	if parsed.GetClassName() != "accept/Arithmetic" || len(parsed.Fields) != 1 || len(parsed.Methods) != 5 {
		t.Fatalf("parsed class = %q with %d fields and %d methods", parsed.GetClassName(), len(parsed.Fields), len(parsed.Methods))
	}

	verifyAndExecute(t, classes, "accept.Arithmetic")

	inputPath := filepath.Join(t.TempDir(), "arithmetic.input")
	if err := os.WriteFile(inputPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	sourceDir := filepath.Join(t.TempDir(), "source")
	if err := javajive.DecompileDEXFile(inputPath, sourceDir, javajive.DexOptions{}); err != nil {
		t.Fatalf("DecompileDEXFile: %v", err)
	}
	assertOnlyJavaSources(t, sourceDir, []string{"accept/Arithmetic.java"})
}

func TestAcceptedMultidexAPKCorpus(t *testing.T) {
	fixture := filepath.Join("testdata", "accept", "multidex.apk")
	apk, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	assertAPKEntries(t, apk, []string{
		"AndroidManifest.xml",
		"assets/ignored.dex",
		"classes.dex",
		"classes2.dex",
		"res/raw/placeholder.txt",
	})

	first, report, err := dex2jar.DexToJar(apk, dex2jar.Options{})
	if err != nil {
		t.Fatalf("DexToJar(APK): %v", err)
	}
	second, secondReport, err := dex2jar.APKToJar(apk, dex2jar.Options{})
	if err != nil {
		t.Fatalf("APKToJar: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("DexToJar and APKToJar produced different JAR bytes")
	}
	if report != secondReport {
		t.Fatalf("reports differ: %#v and %#v", report, secondReport)
	}
	wantReport := dex2jar.Report{DEXFiles: 2, Classes: 2, Methods: 4, TranslatedMethods: 4}
	if report != wantReport {
		t.Fatalf("report = %#v, want %#v", report, wantReport)
	}

	classes := readAcceptedJar(t, first, []string{
		"META-INF/MANIFEST.MF",
		"accept/apk/Primary.class",
		"accept/apk/Secondary.class",
	})
	for entry, className := range map[string]string{
		"accept/apk/Primary.class":   "accept/apk/Primary",
		"accept/apk/Secondary.class": "accept/apk/Secondary",
	} {
		parsed, err := classparser.Parse(classes[entry])
		if err != nil {
			t.Fatalf("parse %s: %v", entry, err)
		}
		if parsed.GetClassName() != className || len(parsed.Methods) != 2 {
			t.Fatalf("parsed %s = %q with %d methods", entry, parsed.GetClassName(), len(parsed.Methods))
		}
	}
	verifyAndExecute(t, classes, "accept.apk.Primary")
	verifyAndExecute(t, classes, "accept.apk.Secondary")

	sourceDir := filepath.Join(t.TempDir(), "source")
	if err := javajive.DecompileDEXFile(fixture, sourceDir, javajive.DexOptions{}); err != nil {
		t.Fatalf("DecompileDEXFile(APK): %v", err)
	}
	assertOnlyJavaSources(t, sourceDir, []string{
		"accept/apk/Primary.java",
		"accept/apk/Secondary.java",
	})
	for _, ignored := range []string{"AndroidManifest.xml", "assets/ignored.dex", "res/raw/placeholder.txt"} {
		if _, err := os.Stat(filepath.Join(sourceDir, filepath.FromSlash(ignored))); !os.IsNotExist(err) {
			t.Fatalf("APK resource %q leaked into DEX-derived output: %v", ignored, err)
		}
	}
}

func assertAPKEntries(t *testing.T, apk []byte, want []string) {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(apk), int64(len(apk)))
	if err != nil {
		t.Fatalf("open APK: %v", err)
	}
	names := make([]string, len(reader.File))
	for index, file := range reader.File {
		names[index] = file.Name
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("APK entries = %v, want %v", names, want)
	}
}

func readAcceptedJar(t *testing.T, jar []byte, wantNames []string) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(jar), int64(len(jar)))
	if err != nil {
		t.Fatalf("open JAR: %v", err)
	}
	names := make([]string, 0, len(reader.File))
	classes := make(map[string][]byte)
	for _, file := range reader.File {
		names = append(names, file.Name)
		stream, err := file.Open()
		if err != nil {
			t.Fatalf("open %q: %v", file.Name, err)
		}
		data, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read %q: %v / %v", file.Name, readErr, closeErr)
		}
		if strings.HasSuffix(file.Name, ".class") {
			if _, err := classparser.Parse(data); err != nil {
				t.Fatalf("parse %q: %v", file.Name, err)
			}
			classes[file.Name] = data
		}
	}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("JAR entries = %v, want %v", names, wantNames)
	}
	return classes
}

func verifyAndExecute(t *testing.T, classes map[string][]byte, mainClass string) {
	t.Helper()
	java, err := exec.LookPath("java")
	if err != nil {
		t.Log("java is unavailable; skipping JVM verification/execution")
		return
	}
	root := t.TempDir()
	for name, data := range classes {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", root, mainClass).CombinedOutput(); err != nil {
		t.Fatalf("java verification/execution: %v\n%s", err, output)
	}
}

func assertOnlyJavaSources(t *testing.T, root string, want []string) {
	t.Helper()
	var javaFiles []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasSuffix(relative, ".java") {
			javaFiles = append(javaFiles, relative)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "yak-decompiler:") {
			t.Errorf("%s contains a yak-decompiler marker", relative)
		}
		if strings.HasSuffix(relative, ".class") {
			t.Errorf("%s is an undecompiled .class fallback", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(root, "META-INF", "MANIFEST.MF"))
	if err != nil {
		t.Fatalf("read preserved manifest: %v", err)
	}
	if string(manifest) != "Manifest-Version: 1.0\r\n\r\n" {
		t.Fatalf("preserved manifest = %q", manifest)
	}
	sort.Strings(javaFiles)
	sort.Strings(want)
	if !reflect.DeepEqual(javaFiles, want) {
		t.Fatalf("decompiled Java paths = %v, want %v", javaFiles, want)
	}
}
