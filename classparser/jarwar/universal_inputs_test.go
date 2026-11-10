package jarwar

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/internal/jimagefixture"
)

func TestFlatExportNestedJar(t *testing.T) {
	classBytes := t23ClassFixture(t)
	inner := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"lib/Nested.class", classBytes},
	})
	outer := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"dep.jar", inner},
		{"root/Kept.class", classBytes},
	})
	src := t23WriteArchive(t, outer, ".jar")
	dst := t.TempDir()
	if err := AutoDecompile(src, dst); err != nil {
		t.Fatalf("AutoDecompile: %v", err)
	}
	want := []string{"dep.jar/lib/Nested.java", "root/Kept.java"}
	if got := t23Files(t, dst); !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
	for _, name := range want {
		body, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(body), "class") {
			t.Fatalf("%s does not contain decompiled source: %q", name, body)
		}
	}
}

func TestFlatExportWARNestedJar(t *testing.T) {
	classBytes := t23ClassFixture(t)
	inner := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"lib/Nested.class", classBytes},
	})
	war := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"WEB-INF/classes/app/Outer.class", classBytes},
		{"WEB-INF/lib/dependency.jar", inner},
		{"WEB-INF/web.xml", []byte("<web-app/>\n")},
	})
	src := t23WriteArchive(t, war, ".war")
	dst := t.TempDir()
	if err := AutoDecompile(src, dst); err != nil {
		t.Fatalf("AutoDecompile: %v", err)
	}
	want := []string{
		"WEB-INF/classes/app/Outer.java",
		"WEB-INF/lib/dependency.jar/lib/Nested.java",
		"WEB-INF/web.xml",
	}
	if got := t23Files(t, dst); !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
}

func TestFlatExportAAR(t *testing.T) {
	classBytes := t23ClassFixture(t)
	classesJAR := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"lib/Nested.class", classBytes},
	})
	extraJAR := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"lib/Extra.class", classBytes},
	})
	aar := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"AndroidManifest.xml", []byte("<manifest/>\n")},
		{"classes.jar", classesJAR},
		{"libs/extra.jar", extraJAR},
	})
	src := t23WriteArchive(t, aar, ".aar")
	dst := t.TempDir()
	if err := AutoDecompile(src, dst); err != nil {
		t.Fatalf("AutoDecompile: %v", err)
	}
	want := []string{
		"AndroidManifest.xml",
		"classes.jar/lib/Nested.java",
		"libs/extra.jar/lib/Extra.java",
	}
	if got := t23Files(t, dst); !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
}

func TestStructuredExportAAR(t *testing.T) {
	classBytes := t23ClassFixture(t)
	classesJAR := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"lib/Nested.class", classBytes},
	})
	aar := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"AndroidManifest.xml", []byte("<manifest/>\n")},
		{"classes.jar", classesJAR},
	})
	src := t23WriteArchive(t, aar, ".aar")
	dst := t.TempDir()
	report, err := AutoDecompileWithOptions(src, dst, ArchiveOptions{Structured: true})
	if err != nil {
		t.Fatalf("structured export: %v", err)
	}
	if len(report.Archives) != 1 || report.Archives[0].Path != "classes.jar" {
		t.Fatalf("archives = %#v", report.Archives)
	}
	want := []string{
		"archives/classes.jar",
		"resources/AndroidManifest.xml",
		"sources/classes.jar!/lib/Nested.java",
	}
	if got := t23Files(t, dst); !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
}

func TestFlatAndStructuredExportJMOD(t *testing.T) {
	classBytes := t23ClassFixture(t)
	payload := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"classes/lib/Nested.class", classBytes},
		{"conf/example.properties", []byte("key=value\n")},
	})
	jmod := append([]byte{'J', 'M', 1, 0}, payload...)
	src := t23WriteArchive(t, jmod, ".jmod")

	flat := t.TempDir()
	if err := AutoDecompile(src, flat); err != nil {
		t.Fatalf("flat AutoDecompile: %v", err)
	}
	wantFlat := []string{"classes/lib/Nested.java", "conf/example.properties"}
	if got := t23Files(t, flat); !reflect.DeepEqual(got, wantFlat) {
		t.Fatalf("flat files = %#v, want %#v", got, wantFlat)
	}

	structured := t.TempDir()
	if _, err := AutoDecompileWithOptions(src, structured, ArchiveOptions{Structured: true}); err != nil {
		t.Fatalf("structured AutoDecompile: %v", err)
	}
	wantStructured := []string{"resources/conf/example.properties", "sources/classes/lib/Nested.java"}
	if got := t23Files(t, structured); !reflect.DeepEqual(got, wantStructured) {
		t.Fatalf("structured files = %#v, want %#v", got, wantStructured)
	}
}

func TestFlatAndStructuredExportJImage(t *testing.T) {
	classBytes := t23ClassFixture(t)
	image, err := jimagefixture.Build([]jimagefixture.Entry{
		{
			Path:       "/test/org/sonatype/plexus/components/cipher/PlexusCipherException.class",
			Data:       classBytes,
			Compressed: true,
		},
		{Path: "/test/conf/app.properties", Data: []byte("key=value\n")},
	})
	if err != nil {
		t.Fatalf("build JImage: %v", err)
	}
	src := t23WriteArchive(t, image, ".jimage")

	flat := t.TempDir()
	if err := AutoDecompile(src, flat); err != nil {
		t.Fatalf("flat AutoDecompile: %v", err)
	}
	wantFlat := []string{
		"org/sonatype/plexus/components/cipher/PlexusCipherException.java",
		"test/conf/app.properties",
	}
	if got := t23Files(t, flat); !reflect.DeepEqual(got, wantFlat) {
		t.Fatalf("flat files = %#v, want %#v", got, wantFlat)
	}

	structured := t.TempDir()
	if _, err := AutoDecompileWithOptions(src, structured, ArchiveOptions{Structured: true}); err != nil {
		t.Fatalf("structured AutoDecompile: %v", err)
	}
	wantStructured := []string{
		"resources/test/conf/app.properties",
		"sources/org/sonatype/plexus/components/cipher/PlexusCipherException.java",
	}
	if got := t23Files(t, structured); !reflect.DeepEqual(got, wantStructured) {
		t.Fatalf("structured files = %#v, want %#v", got, wantStructured)
	}
}

func TestJImageUnsupportedCompressor(t *testing.T) {
	classBytes := t23ClassFixture(t)
	image, err := jimagefixture.Build([]jimagefixture.Entry{
		{
			Path:       "/test/org/sonatype/plexus/components/cipher/PlexusCipherException.class",
			Data:       classBytes,
			Compressed: true,
			Compressor: "compact-cp",
		},
	})
	if err != nil {
		t.Fatalf("build JImage: %v", err)
	}
	src := t23WriteArchive(t, image, ".jimage")
	err = AutoDecompile(src, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "compact-cp") {
		t.Fatalf("error = %v, want compact-cp diagnostic", err)
	}
}

func TestJarFSReadsJMOD(t *testing.T) {
	classBytes := t23ClassFixture(t)
	payload := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"classes/lib/Nested.class", classBytes},
	})
	jmod := append([]byte{'J', 'M', 1, 0}, payload...)
	path := t23WriteArchive(t, jmod, ".jmod")
	jarFS, err := classparser.NewJarFSFromLocal(path)
	if err != nil {
		t.Fatalf("NewJarFSFromLocal: %v", err)
	}
	defer jarFS.Close()
	source, err := jarFS.ReadFile("classes/lib/Nested.class")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(source), "class") {
		t.Fatalf("source = %q", source)
	}
}

func TestFlatExportNestedJMOD(t *testing.T) {
	classBytes := t23ClassFixture(t)
	payload := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"lib/Nested.class", classBytes},
	})
	nested := append([]byte{'J', 'M', 1, 0}, payload...)
	outer := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"lib/dependency.jmod", nested},
	})
	src := t23WriteArchive(t, outer, ".jar")
	dst := t.TempDir()
	if err := AutoDecompile(src, dst); err != nil {
		t.Fatalf("AutoDecompile: %v", err)
	}
	want := []string{"lib/dependency.jmod/lib/Nested.java"}
	if got := t23Files(t, dst); !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
}

func TestFlatExportPAR(t *testing.T) {
	classBytes := t23ClassFixture(t)
	archive := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"lib/Nested.class", classBytes},
	})
	src := t23WriteArchive(t, archive, ".par")
	dst := t.TempDir()
	if err := AutoDecompile(src, dst); err != nil {
		t.Fatalf("AutoDecompile: %v", err)
	}
	want := []string{"lib/Nested.java"}
	if got := t23Files(t, dst); !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
}
