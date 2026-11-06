package jarwar

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/internal/filesys"
)

func t23Zip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := fw.Write(body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestT23_C02_DumpConfined(t *testing.T) {
	t.Run("safe_join", func(t *testing.T) {
		root := t.TempDir()
		if _, err := SafeJoin(root, "../x"); err == nil {
			t.Fatal("expected reject ../x")
		}
		if _, err := SafeJoin(root, "/abs"); err == nil {
			t.Fatal("expected reject /abs")
		}
		got, err := SafeJoin(root, `foo\bar`)
		if err != nil {
			t.Fatalf("backslash: %v", err)
		}
		want := filepath.Join(root, "foo", "bar")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("dump_never_writes_outside_root", func(t *testing.T) {
		parent := t.TempDir()
		dst := filepath.Join(parent, "out")
		if err := os.MkdirAll(dst, 0755); err != nil {
			t.Fatal(err)
		}
		raw := t23Zip(t, map[string][]byte{
			"hello.txt":   []byte("hi"),
			"../evil.txt": []byte("escaped"),
		})
		zfs, err := filesys.NewZipFSFromString(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		jfs := javaclassparser.NewJarFS(zfs)
		ins, err := NewFromJarFS(jfs)
		if err != nil {
			t.Fatal(err)
		}
		_ = ins.DumpToLocalFileSystem(dst)
		if _, err := os.Stat(filepath.Join(parent, "evil.txt")); err == nil {
			t.Fatal("zip-slip dump wrote outside destination")
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(parent), "evil.txt")); err == nil {
			t.Fatal("zip-slip dump wrote outside parent")
		}
	})
}

func t23ZipOrdered(t *testing.T, entries []struct {
	name string
	body []byte
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, entry := range entries {
		fw, err := w.Create(entry.name)
		if err != nil {
			t.Fatalf("create %s: %v", entry.name, err)
		}
		if _, err := fw.Write(entry.body); err != nil {
			t.Fatalf("write %s: %v", entry.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func t23ClassFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/plexus-cipher-2.0.jar")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	const name = "org/sonatype/plexus/components/cipher/PlexusCipherException.class"
	data, err := fs.ReadFile(zr, name)
	if err != nil {
		t.Fatalf("read class fixture %s: %v", name, err)
	}
	return data
}

func t23WriteArchive(t *testing.T, raw []byte, suffix string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "fixture"+suffix)
	if err := os.WriteFile(name, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return name
}

func t23Files(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func TestStructuredWARExport(t *testing.T) {
	classBytes := t23ClassFixture(t)
	innerResource := []byte{0, 1, 2, 3, 0xff}
	inner := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"lib/data.bin", innerResource},
		{"lib/Nested.class", classBytes},
	})
	outerResource := []byte("<web-app>exact resource</web-app>\n")
	war := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"assets/app.js", []byte("const exact = true;\n")},
		{"WEB-INF/lib/dependency.jar", inner},
		{"WEB-INF/web.xml", outerResource},
		{"WEB-INF/classes/app/Outer.class", classBytes},
	})
	src := t23WriteArchive(t, war, ".war")

	run := func(t *testing.T) (ArchiveReport, string) {
		t.Helper()
		dst := t.TempDir()
		report, err := AutoDecompileWithOptions(src, dst, ArchiveOptions{Structured: true})
		if err != nil {
			t.Fatalf("structured export: %v", err)
		}
		return report, dst
	}

	report, dst := run(t)
	second, secondDst := run(t)
	if !reflect.DeepEqual(report, second) {
		a, _ := json.Marshal(report)
		b, _ := json.Marshal(second)
		t.Fatalf("nondeterministic report:\n%s\n%s", a, b)
	}

	wantFiles := []string{
		"archives/WEB-INF/lib/dependency.jar",
		"resources/WEB-INF/lib/dependency.jar!/lib/data.bin",
		"resources/WEB-INF/web.xml",
		"resources/assets/app.js",
		"sources/WEB-INF/classes/app/Outer.java",
		"sources/WEB-INF/lib/dependency.jar!/lib/Nested.java",
	}
	if got := t23Files(t, dst); !reflect.DeepEqual(got, wantFiles) {
		t.Fatalf("files = %#v, want %#v", got, wantFiles)
	}
	if got := t23Files(t, secondDst); !reflect.DeepEqual(got, wantFiles) {
		t.Fatalf("second files = %#v, want %#v", got, wantFiles)
	}

	assertBytes := func(name string, want []byte) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s bytes differ", name)
		}
		info, err := os.Stat(filepath.Join(dst, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Fatalf("%s mode = %o, want 0644", name, info.Mode().Perm())
		}
	}
	assertBytes("archives/WEB-INF/lib/dependency.jar", inner)
	assertBytes("resources/WEB-INF/web.xml", outerResource)
	assertBytes("resources/WEB-INF/lib/dependency.jar!/lib/data.bin", innerResource)

	if len(report.Archives) != 1 {
		t.Fatalf("archives = %#v", report.Archives)
	}
	wantArchive := NestedArchiveReport{
		Path:       "WEB-INF/lib/dependency.jar",
		OutputPath: "archives/WEB-INF/lib/dependency.jar",
		Depth:      1,
		Size:       int64(len(inner)),
	}
	if report.Archives[0] != wantArchive {
		t.Fatalf("archive report = %#v, want %#v", report.Archives[0], wantArchive)
	}
	foundNested := false
	for _, entry := range report.Entries {
		if entry.SourcePath == "WEB-INF/lib/dependency.jar!/lib/Nested.class" {
			foundNested = entry.Archive == "WEB-INF/lib/dependency.jar" && entry.Kind == "source" && entry.OutputPath == "sources/WEB-INF/lib/dependency.jar!/lib/Nested.java"
		}
	}
	if !foundNested {
		t.Fatalf("missing nested source provenance in %#v", report.Entries)
	}
	for _, source := range []string{
		"sources/WEB-INF/classes/app/Outer.java",
		"sources/WEB-INF/lib/dependency.jar!/lib/Nested.java",
	} {
		raw, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(source)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(raw, []byte("PlexusCipherException")) {
			t.Fatalf("%s does not contain decompiled class source", source)
		}
	}
}

func TestStructuredWARExportNestedProvenance(t *testing.T) {
	deepResource := []byte("deep-exact")
	deep := t23Zip(t, map[string][]byte{"deep.txt": deepResource})
	middle := t23Zip(t, map[string][]byte{"nested/deep.jar": deep})
	war := t23Zip(t, map[string][]byte{"WEB-INF/lib/middle.jar": middle})
	src := t23WriteArchive(t, war, ".war")
	dst := t.TempDir()
	report, err := AutoDecompileWithOptions(src, dst, ArchiveOptions{Structured: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []NestedArchiveReport{
		{Path: "WEB-INF/lib/middle.jar", OutputPath: "archives/WEB-INF/lib/middle.jar", Depth: 1, Size: int64(len(middle))},
		{Path: "WEB-INF/lib/middle.jar!/nested/deep.jar", Parent: "WEB-INF/lib/middle.jar", OutputPath: "archives/WEB-INF/lib/middle.jar!/nested/deep.jar", Depth: 2, Size: int64(len(deep))},
	}
	if !reflect.DeepEqual(report.Archives, want) {
		t.Fatalf("archives = %#v, want %#v", report.Archives, want)
	}
	raw, err := os.ReadFile(filepath.Join(dst, "resources", "WEB-INF", "lib", "middle.jar!", "nested", "deep.jar!", "deep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, deepResource) {
		t.Fatalf("deep resource = %q", raw)
	}
}

func TestArchiveOptionsFlatCompatibilityAndSharedBudget(t *testing.T) {
	t.Run("zero options preserve flat layout", func(t *testing.T) {
		raw := t23Zip(t, map[string][]byte{"plain.txt": []byte("flat")})
		src := t23WriteArchive(t, raw, ".jar")
		dst := t.TempDir()
		report, err := AutoDecompileWithOptions(src, dst, ArchiveOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(report, ArchiveReport{}) {
			t.Fatalf("legacy report = %#v", report)
		}
		got, err := os.ReadFile(filepath.Join(dst, "plain.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "flat" {
			t.Fatalf("flat resource = %q", got)
		}
		if _, err := os.Stat(filepath.Join(dst, structuredResourcesRoot)); !os.IsNotExist(err) {
			t.Fatalf("zero options unexpectedly made structured tree: %v", err)
		}
	})

	t.Run("nested layers share aggregate budget", func(t *testing.T) {
		payload := bytes.Repeat([]byte("x"), 128)
		inner := t23Zip(t, map[string][]byte{"payload.bin": payload})
		war := t23Zip(t, map[string][]byte{"WEB-INF/lib/dependency.jar": inner})
		src := t23WriteArchive(t, war, ".war")
		_, err := AutoDecompileWithOptions(src, t.TempDir(), ArchiveOptions{
			Structured: true,
			Limits: javaclassparser.Limits{
				MaxArchiveEntries:    16,
				MaxArchiveEntryBytes: 1 << 20,
				MaxArchiveTotalBytes: int64(len(inner) + len(payload) - 1),
				MaxArchiveDepth:      4,
			},
		})
		if err == nil || !filesys.IsResourceLimit(err) {
			t.Fatalf("expected shared aggregate limit, got %v", err)
		}
	})
}

func TestStructuredWARExportPathSafetyAndErrors(t *testing.T) {
	t.Run("unsafe archive entry", func(t *testing.T) {
		war := t23Zip(t, map[string][]byte{"../escape.txt": []byte("no")})
		src := t23WriteArchive(t, war, ".war")
		parent := t.TempDir()
		dst := filepath.Join(parent, "out")
		_, err := AutoDecompileWithOptions(src, dst, ArchiveOptions{Structured: true})
		if err == nil || !filesys.IsPathRejected(err) {
			t.Fatalf("expected path rejection, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(parent, "escape.txt")); !os.IsNotExist(statErr) {
			t.Fatalf("escape file exists: %v", statErr)
		}
	})

	t.Run("provenance output collision", func(t *testing.T) {
		inner := t23Zip(t, map[string][]byte{"x.txt": []byte("nested")})
		war := t23Zip(t, map[string][]byte{
			"WEB-INF/lib/dependency.jar":        inner,
			"WEB-INF/lib/dependency.jar!/x.txt": []byte("outer"),
		})
		src := t23WriteArchive(t, war, ".war")
		_, err := AutoDecompileWithOptions(src, t.TempDir(), ArchiveOptions{Structured: true})
		if err == nil || !filesys.IsPathRejected(err) {
			t.Fatalf("expected provenance collision rejection, got %v", err)
		}
	})

	t.Run("corrupt nested archive", func(t *testing.T) {
		war := t23Zip(t, map[string][]byte{"WEB-INF/lib/broken.jar": []byte("not-a-zip")})
		src := t23WriteArchive(t, war, ".war")
		_, err := AutoDecompileWithOptions(src, t.TempDir(), ArchiveOptions{Structured: true})
		if err == nil || !strings.Contains(err.Error(), "open nested archive") {
			t.Fatalf("expected nested archive error, got %v", err)
		}
	})

	t.Run("destination symlink", func(t *testing.T) {
		war := t23Zip(t, map[string][]byte{"safe.txt": []byte("safe")})
		src := t23WriteArchive(t, war, ".war")
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "resources")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		_, err := AutoDecompileWithOptions(src, root, ArchiveOptions{Structured: true})
		if err == nil || !filesys.IsPathRejected(err) {
			t.Fatalf("expected symlink rejection, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(outside, "safe.txt")); !os.IsNotExist(statErr) {
			t.Fatalf("outside write exists: %v", statErr)
		}
	})
}
