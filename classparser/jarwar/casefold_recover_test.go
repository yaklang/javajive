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
