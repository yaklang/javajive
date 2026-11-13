package jimage_test

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/jimagefixture"
	"github.com/yaklang/javajive/jimage"
)

func buildImage(t *testing.T, entries []jimagefixture.Entry) []byte {
	t.Helper()
	data, err := jimagefixture.Build(entries)
	if err != nil {
		t.Fatalf("build JImage: %v", err)
	}
	return data
}

func requireJImageIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("JAVAJIVE_JIMAGE_INTEGRATION") != "1" {
		t.Skip("JDK/JImage integration disabled; set JAVAJIVE_JIMAGE_INTEGRATION=1 to run")
	}
}

func TestReadEntriesAndResources(t *testing.T) {
	classBytes := []byte{0xca, 0xfe, 0xba, 0xbe, 1, 2, 3, 4}
	resource := []byte("security.provider.1=SunSecurityProvider\n")
	image := buildImage(t, []jimagefixture.Entry{
		{Path: "/java.base/java/lang/Object.class", Data: classBytes},
		{Path: "/java.base/conf/security/java.security", Data: resource},
	})
	reader, err := jimage.OpenBytes(image)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer reader.Close()

	header := reader.Header()
	if header.ResourceCount != 2 || header.TableLength != 2 || header.Major() != 1 || header.Minor() != 1 {
		t.Fatalf("header = %#v", header)
	}
	entries := reader.Entries()
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	classEntry, ok := reader.Entry("/java.base/java/lang/Object.class")
	if !ok {
		t.Fatal("class entry not found")
	}
	if classEntry.Module != "java.base" || classEntry.Parent != "java/lang" ||
		classEntry.Base != "Object" || classEntry.Extension != "class" || classEntry.Directory {
		t.Fatalf("class entry = %#v", classEntry)
	}
	resourceEntry, ok := reader.Entry("/java.base/conf/security/java.security")
	if !ok {
		t.Fatal("resource entry not found")
	}
	if resourceEntry.Module != "java.base" || resourceEntry.Parent != "conf/security" ||
		resourceEntry.Base != "java" || resourceEntry.Extension != "security" {
		t.Fatalf("resource entry = %#v", resourceEntry)
	}

	for _, name := range []string{
		"/java.base/java/lang/Object.class",
		"java.base/java/lang/Object.class",
		"/modules/java.base/java/lang/Object.class",
	} {
		got, err := reader.Read(name)
		if err != nil {
			t.Fatalf("Read(%q): %v", name, err)
		}
		if !bytes.Equal(got, classBytes) {
			t.Fatalf("Read(%q) = %x, want %x", name, got, classBytes)
		}
	}
	got, err := reader.Read("java.base/conf/security/java.security")
	if err != nil {
		t.Fatalf("Read resource: %v", err)
	}
	if !bytes.Equal(got, resource) {
		t.Fatalf("resource = %q, want %q", got, resource)
	}
}

func TestReadCompressedResource(t *testing.T) {
	data := bytes.Repeat([]byte("compressed jimage resource "), 64)
	image := buildImage(t, []jimagefixture.Entry{
		{Path: "/test/demo/Foo.class", Data: data, Compressed: true},
	})
	reader, err := jimage.OpenBytes(image)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer reader.Close()
	entry, ok := reader.Entry("/test/demo/Foo.class")
	if !ok {
		t.Fatal("entry not found")
	}
	if entry.CompressedSize == 0 {
		t.Fatal("compressed size is zero")
	}
	got, err := reader.Read("/test/demo/Foo.class")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("decompressed bytes differ")
	}
}

func TestOpenRejectsResourceCountOverTableLength(t *testing.T) {
	image := buildImage(t, []jimagefixture.Entry{
		{Path: "/test/Foo.class", Data: []byte("class")},
	})
	binary.LittleEndian.PutUint32(image[12:16], ^uint32(0))
	if _, err := jimage.OpenBytes(image); err == nil || !strings.Contains(err.Error(), "resource count") {
		t.Fatalf("OpenBytes error = %v, want resource count rejection", err)
	}
}

func TestReadRejectsOversizedDecompressorLayer(t *testing.T) {
	image := buildImage(t, []jimagefixture.Entry{
		{Path: "/test/Foo.class", Data: []byte("class"), Compressed: true},
	})
	reader, err := jimage.Open(bytes.NewReader(image), int64(len(image)), jimage.Options{MaxResourceSize: 64})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	entry, ok := reader.Entry("/test/Foo.class")
	if !ok {
		t.Fatal("entry not found")
	}
	resourceStart := len(image) - int(entry.CompressedSize)
	binary.LittleEndian.PutUint64(image[resourceStart+12:resourceStart+20], 65)
	if _, err := reader.Read("/test/Foo.class"); err == nil || !strings.Contains(err.Error(), "uncompressed layer size") {
		t.Fatalf("Read error = %v, want decompressor layer limit", err)
	}
}

func TestUnsupportedCompressor(t *testing.T) {
	image := buildImage(t, []jimagefixture.Entry{
		{Path: "/test/Compact.class", Data: []byte{0xca, 0xfe, 0xba, 0xbe}, Compressed: true, Compressor: "lzma"},
	})
	reader, err := jimage.OpenBytes(image)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer reader.Close()
	_, err = reader.Read("/test/Compact.class")
	if !errors.Is(err, jimage.ErrUnsupportedCompressor) {
		t.Fatalf("error = %v, want ErrUnsupportedCompressor", err)
	}
	var unsupported *jimage.UnsupportedCompressorError
	if !errors.As(err, &unsupported) || unsupported.Compressor != "lzma" {
		t.Fatalf("error details = %#v", err)
	}
}

func TestWriteZIP(t *testing.T) {
	classBytes := []byte{0xca, 0xfe, 0xba, 0xbe, 9, 8, 7}
	moduleInfo := []byte{0xca, 0xfe, 0xba, 0xbe, 6, 5}
	resource := []byte("key=value\n")
	image := buildImage(t, []jimagefixture.Entry{
		{Path: "/test/demo/Foo.class", Data: classBytes},
		{Path: "/test/conf/app.properties", Data: resource},
		{Path: "/java.base/module-info.class", Data: moduleInfo},
	})
	reader, err := jimage.OpenBytes(image)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer reader.Close()
	jarBytes, err := reader.ZIPBytes()
	if err != nil {
		t.Fatalf("ZIPBytes: %v", err)
	}
	zipReader, err := zip.NewReader(bytes.NewReader(jarBytes), int64(len(jarBytes)))
	if err != nil {
		t.Fatalf("open JAR: %v", err)
	}
	want := map[string][]byte{
		"demo/Foo.class":              classBytes,
		"test/conf/app.properties":    resource,
		"java.base/module-info.class": moduleInfo,
	}
	if len(zipReader.File) != len(want) {
		t.Fatalf("JAR entries = %d, want %d", len(zipReader.File), len(want))
	}
	for _, file := range zipReader.File {
		expected, ok := want[file.Name]
		if !ok {
			t.Fatalf("unexpected JAR entry %q", file.Name)
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatalf("open %q: %v", file.Name, err)
		}
		body, err := io.ReadAll(stream)
		stream.Close()
		if err != nil {
			t.Fatalf("read %q: %v", file.Name, err)
		}
		if !bytes.Equal(body, expected) {
			t.Fatalf("JAR entry %q = %x, want %x", file.Name, body, expected)
		}
	}
}

func TestWriteZIPRejectsClassCollision(t *testing.T) {
	image := buildImage(t, []jimagefixture.Entry{
		{Path: "/module.a/demo/Foo.class", Data: []byte{1}},
		{Path: "/module.b/demo/Foo.class", Data: []byte{2}},
	})
	reader, err := jimage.OpenBytes(image)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer reader.Close()
	if _, err := reader.ZIPBytes(); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("error = %v, want collision", err)
	}
}

func TestOpenJDKImage(t *testing.T) {
	requireJImageIntegration(t)

	_, home := findJDK(t)
	path := filepath.Join(home, "lib", "modules")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("JDK image not found at %s", path)
	}
	reader, err := jimage.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer reader.Close()
	if reader.Header().ResourceCount < 1000 {
		t.Fatalf("resource count = %d, want a full JDK image", reader.Header().ResourceCount)
	}
	object, err := reader.Read("java.base/java/lang/Object.class")
	if err != nil {
		t.Fatalf("Read Object.class: %v", err)
	}
	if !bytes.HasPrefix(object, []byte{0xca, 0xfe, 0xba, 0xbe}) {
		t.Fatalf("Object.class magic = %x", object[:4])
	}
}

func findJDK(t *testing.T) (jlink, home string) {
	t.Helper()
	if home := os.Getenv("JAVA_HOME"); home != "" {
		candidate := filepath.Join(home, "bin", "jlink")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, home
		}
	}
	if output, err := exec.Command("/usr/libexec/java_home").Output(); err == nil {
		home := strings.TrimSpace(string(output))
		candidate := filepath.Join(home, "bin", "jlink")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, home
		}
	}
	path, err := exec.LookPath("jlink")
	if err != nil {
		t.Skip("jlink not available")
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	home = filepath.Dir(filepath.Dir(path))
	if _, err := os.Stat(filepath.Join(home, "lib", "modules")); err != nil {
		t.Skipf("JDK runtime image not found next to %s", path)
	}
	return path, home
}

func runJLink(t *testing.T, jlink string, args ...string) {
	t.Helper()
	output, err := exec.Command(jlink, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("jlink %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func TestJLinkCompactCPImage(t *testing.T) {
	requireJImageIntegration(t)

	jlink, home := findJDK(t)
	plainPath := filepath.Join(home, "lib", "modules")
	if _, err := os.Stat(plainPath); err != nil {
		t.Skipf("JDK runtime image not found at %s", plainPath)
	}
	output := filepath.Join(t.TempDir(), "image")
	runJLink(t, jlink, "--compress=1", "--add-modules", "java.base", "--output", output, "--no-header-files", "--no-man-pages")

	compact, err := jimage.OpenFile(filepath.Join(output, "lib", "modules"))
	if err != nil {
		t.Fatalf("open compact-cp image: %v", err)
	}
	defer compact.Close()
	plain, err := jimage.OpenFile(plainPath)
	if err != nil {
		t.Fatalf("open plain image: %v", err)
	}
	defer plain.Close()

	name := "java.base/java/lang/Object.class"
	got, err := compact.Read(name)
	if err != nil {
		t.Fatalf("read compact-cp Object.class: %v", err)
	}
	want, err := plain.Read(name)
	if err != nil {
		t.Fatalf("read plain Object.class: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("compact-cp %s differs from the plain image", name)
	}
}

func TestJLinkBigEndianImage(t *testing.T) {
	requireJImageIntegration(t)

	jlink, home := findJDK(t)
	plainPath := filepath.Join(home, "lib", "modules")
	if _, err := os.Stat(plainPath); err != nil {
		t.Skipf("JDK runtime image not found at %s", plainPath)
	}
	output := filepath.Join(t.TempDir(), "image")
	runJLink(t, jlink, "--compress=0", "--endian", "big", "--add-modules", "java.base", "--output", output, "--no-header-files", "--no-man-pages")

	bigEndian, err := jimage.OpenFile(filepath.Join(output, "lib", "modules"))
	if err != nil {
		t.Fatalf("open big-endian image: %v", err)
	}
	defer bigEndian.Close()
	plain, err := jimage.OpenFile(plainPath)
	if err != nil {
		t.Fatalf("open plain image: %v", err)
	}
	defer plain.Close()

	name := "java.base/java/lang/Object.class"
	got, err := bigEndian.Read(name)
	if err != nil {
		t.Fatalf("read big-endian Object.class: %v", err)
	}
	want, err := plain.Read(name)
	if err != nil {
		t.Fatalf("read plain Object.class: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("big-endian %s differs from the plain image", name)
	}
}
