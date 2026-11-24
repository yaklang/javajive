package jarwar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
)

func buildMinimalClassBytes(internalName string) []byte {
	b := classfilebuilder.NewClass(internalName, "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
	initM := b.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "()V")
	code := initM.Code()
	ref, _ := b.Pool().Methodref("java/lang/Object", "<init>", "()V")
	code.Load(classfilebuilder.RefLocal, 0).CP(classfilebuilder.INVOKESPECIAL, ref).Emit(classfilebuilder.RETURN)
	raw, err := b.Bytes()
	if err != nil {
		panic(err)
	}
	return raw
}

func TestCaseFoldAndIllegalFilenameRecovery(t *testing.T) {
	archive := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"D0/A.class", buildMinimalClassBytes("D0/A")},
		{"D0/a.class", buildMinimalClassBytes("D0/a")},
		{"?/??.class", buildMinimalClassBytes("?/??")},
	})
	src := t23WriteArchive(t, archive, ".jar")

	strictDst := t.TempDir()
	if err := AutoDecompile(src, strictDst); err == nil {
		t.Fatal("strict AutoDecompile unexpectedly accepted case-fold collision")
	}

	recoverDst := t.TempDir()
	if _, err := AutoDecompileWithOptions(src, recoverDst, ArchiveOptions{
		RetainUnprovedSource: true,
	}); err != nil {
		t.Fatalf("best-effort AutoDecompileWithOptions failed: %v", err)
	}

	files := t23Files(t, recoverDst)
	if len(files) != 3 {
		t.Fatalf("expected 3 non-overwriting exported files, got %#v", files)
	}
	var sawSanitized, sawDisambiguated bool
	for _, rel := range files {
		if strings.Contains(rel, "_u003f") {
			sawSanitized = true
		}
		if strings.Contains(rel, "__case2/") && strings.EqualFold(filepath.Base(rel), "a.java") {
			sawDisambiguated = true
		}
		body, err := os.ReadFile(filepath.Join(recoverDst, filepath.FromSlash(rel)))
		if err != nil || !strings.Contains(string(body), "class") {
			t.Fatalf("exported file %s unreadable or missing class source: err=%v body=%q", rel, err, body)
		}
	}
	if !sawSanitized || !sawDisambiguated {
		t.Fatalf("expected both sanitized illegal filename and __case2/ file, got %#v", files)
	}

	// Structured export under RetainUnprovedSource must also disambiguate instead of failing.
	structDst := t.TempDir()
	rep, err := AutoDecompileWithOptions(src, structDst, ArchiveOptions{
		Structured:           true,
		RetainUnprovedSource: true,
	})
	if err != nil {
		t.Fatalf("structured RetainUnprovedSource failed: %v", err)
	}
	if len(rep.Entries) != 3 {
		t.Fatalf("expected 3 structured entries, got %d (%+v)", len(rep.Entries), rep.Entries)
	}
}

func TestBestEffortEscapesIllegalJavaPackageDirectories(t *testing.T) {
	archive := t23ZipOrdered(t, []struct {
		name string
		body []byte
	}{
		{"\u266b/", nil},
		{"\u266b/A.class", buildMinimalClassBytes("\u266b/A")},
		{"WEB-INF/classes/\u266b/B.class", buildMinimalClassBytes("\u266b/B")},
		{"res/\u266b/note.txt", []byte("ok")},
	})
	src := t23WriteArchive(t, archive, ".jar")

	recoverDst := t.TempDir()
	if _, err := AutoDecompileWithOptions(src, recoverDst, ArchiveOptions{
		RetainUnprovedSource: true,
	}); err != nil {
		t.Fatalf("best-effort AutoDecompileWithOptions failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(recoverDst, "\u266b")); !os.IsNotExist(err) {
		t.Fatalf("expected no stale unsanitized package directory, stat err=%v", err)
	}
	aBytes, err := os.ReadFile(filepath.Join(recoverDst, "_u266b", "A.java"))
	if err != nil {
		t.Fatalf("expected sanitized _u266b/A.java: %v (files=%#v)", err, t23Files(t, recoverDst))
	}
	if !strings.Contains(string(aBytes), "package _u266b;") {
		t.Fatalf("expected escaped package declaration in _u266b/A.java, got:\n%s", aBytes)
	}
	if _, err := os.Stat(filepath.Join(recoverDst, "WEB-INF", "classes", "_u266b", "B.java")); err != nil {
		t.Fatalf("expected WEB-INF/classes/_u266b/B.java without escaping container segment: %v (files=%#v)", err, t23Files(t, recoverDst))
	}
	if _, err := os.Stat(filepath.Join(recoverDst, "res", "\u266b", "note.txt")); err != nil {
		t.Fatalf("expected non-class resource path preserved: %v", err)
	}

	structDst := t.TempDir()
	rep, err := AutoDecompileWithOptions(src, structDst, ArchiveOptions{
		Structured:           true,
		RetainUnprovedSource: true,
	})
	if err != nil {
		t.Fatalf("structured RetainUnprovedSource failed: %v", err)
	}
	wantOutputs := map[string]bool{
		"sources/_u266b/A.java":                  false,
		"sources/WEB-INF/classes/_u266b/B.java": false,
		"resources/res/\u266b/note.txt":         false,
	}
	for _, entry := range rep.Entries {
		if _, ok := wantOutputs[entry.OutputPath]; ok {
			wantOutputs[entry.OutputPath] = true
		}
	}
	for out, seen := range wantOutputs {
		if !seen {
			t.Fatalf("missing structured output %q in %+v", out, rep.Entries)
		}
	}
}
