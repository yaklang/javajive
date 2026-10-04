package jarwar

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
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
