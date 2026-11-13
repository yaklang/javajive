package javaclassparser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
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

func t23MustResource(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "resource_limit") {
		t.Fatalf("expected resource_limit, got %v", err)
	}
}

func t23ArchiveClass(t *testing.T, name string) []byte {
	t.Helper()
	class := classfilebuilder.NewClass(name, "java/lang/Object", 0x0021)
	objectInit, err := class.Pool().Methodref("java/lang/Object", "<init>", "()V")
	if err != nil {
		t.Fatal(err)
	}
	class.AddMethod(0x0001, "<init>", "()V").Code().
		Load(classfilebuilder.RefLocal, 0).
		CP(classfilebuilder.INVOKESPECIAL, objectInit).
		Emit(classfilebuilder.RETURN)
	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestArchiveClassOutcomeContainsFailures(t *testing.T) {
	valid := t23ArchiveClass(t, "Valid")
	future := bytes.Clone(valid)
	future[6], future[7] = 0, byte(MaxSupportedClassMajor+1)
	raw := t23Zip(t, map[string][]byte{
		"Valid.class":     valid,
		"Malformed.class": {0xca, 0xfe, 0xba, 0xbe},
		"Future.class":    future,
	})
	zipFS, err := filesys.NewZipFSFromString(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer zipFS.Close()
	jar := NewJarFS(zipFS)

	complete, err := jar.ReadClassOutcome("Valid.class")
	if err != nil || complete.Status != ClassDecompileComplete || !bytes.Contains(complete.Source, []byte("class Valid")) {
		t.Fatalf("valid outcome = status %q err %v source %q", complete.Status, err, complete.Source)
	}
	for _, test := range []struct {
		name   string
		code   string
		prefix string
	}{
		{"Malformed.class", "class_parse_failed", "// decompile parse failed for Malformed.class:"},
		{"Future.class", "unsupported_classfile_version", "// decompile fallback for Future.class:"},
	} {
		outcome, err := jar.ReadClassOutcome(test.name)
		if err != nil || outcome.Status != ClassDecompileFallback || len(outcome.Diagnostics) != 1 || outcome.Diagnostics[0].Code != test.code {
			t.Fatalf("%s outcome = %+v err %v", test.name, outcome, err)
		}
		if !bytes.HasPrefix(outcome.Source, []byte(test.prefix)) {
			t.Fatalf("%s source marker = %q, want prefix %q", test.name, outcome.Source, test.prefix)
		}
	}
}

func TestClassFallbackTextIsSingleLineAndBounded(t *testing.T) {
	outcome := classFallbackOutcome("evil\nName.class", "class_decompile_panic", strings.Repeat("x", 600)+"\nclass Injected {}")
	if strings.Count(string(outcome.Source), "\n") != 1 || bytes.Contains(outcome.Source, []byte("class Injected")) {
		t.Fatalf("fallback source = %q", outcome.Source)
	}
	if len([]rune(outcome.Diagnostics[0].Message)) != 480 {
		t.Fatalf("diagnostic length = %d, want 480", len([]rune(outcome.Diagnostics[0].Message)))
	}
}

func TestArchiveClassOutcomePreservesMethodStubs(t *testing.T) {
	object, err := Parse(readValidationSeed(t))
	if err != nil {
		t.Fatal(err)
	}
	var code *CodeAttribute
	for _, method := range object.Methods {
		if int(method.NameIndex) == 0 || int(method.NameIndex) > len(object.ConstantPool) {
			continue
		}
		nameInfo, ok := object.ConstantPool[method.NameIndex-1].(*ConstantUtf8Info)
		if !ok || nameInfo.Value != "run" {
			continue
		}
		for _, attribute := range method.Attributes {
			if candidate, ok := attribute.(*CodeAttribute); ok {
				code = candidate
			}
		}
	}
	if code == nil {
		t.Fatal("seed run method has no Code attribute")
	}
	corrupted := false
	for i := 0; i+2 < len(code.Code); i++ {
		if code.Code[i] == 0xb2 {
			binary.BigEndian.PutUint16(code.Code[i+1:i+3], 0xffff)
			corrupted = true
			break
		}
	}
	if !corrupted {
		t.Fatal("seed run method has no getstatic operand to corrupt")
	}
	archive := t23Zip(t, map[string][]byte{"Partial.class": object.Bytes()})
	zipFS, err := filesys.NewZipFSFromString(string(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer zipFS.Close()
	outcome, err := NewJarFS(zipFS).ReadClassOutcome("Partial.class")
	if err != nil || outcome.Status != ClassDecompilePartial || len(outcome.StubMethods) == 0 || !bytes.Contains(outcome.Source, []byte(DecompileStubMarker)) {
		t.Fatalf("partial outcome = %+v err %v source %q", outcome, err, outcome.Source)
	}
}

func TestArchiveClassPanicContainmentPreservesResourceErrors(t *testing.T) {
	jar := &JarFS{}
	var work *workbudget.Budget
	outcome, err := jar.containClassDecompile("Panic.class", &work, func() (ClassDecompileOutcome, error) {
		panic("fixture panic")
	})
	if err != nil || outcome.Status != ClassDecompileFallback || len(outcome.Diagnostics) != 1 || outcome.Diagnostics[0].Code != "class_decompile_panic" {
		t.Fatalf("panic outcome = %+v err %v", outcome, err)
	}

	work = workbudget.New(context.Background(), Limits{MaxRequestWork: 1})
	if err := work.Charge(workbudget.CounterRequestWork, 2); err == nil {
		t.Fatal("fixture did not exhaust resource budget")
	}
	outcome, err = jar.containClassDecompile("Resource.class", &work, func() (ClassDecompileOutcome, error) {
		panic("after resource failure")
	})
	if err == nil || !workbudget.Is(err) || outcome.Status != "" {
		t.Fatalf("resource panic outcome = %+v err %v", outcome, err)
	}
}

func TestT23_C01_ExpandedPositiveControl(t *testing.T) {
	raw := t23Zip(t, map[string][]byte{"hello.txt": []byte("hello")})
	limits := filesys.ArchiveLimits{MaxArchiveEntries: 32, MaxArchiveEntryBytes: 1 << 20, MaxArchiveTotalBytes: 1 << 20, MaxArchiveDepth: 8}
	zfs, err := filesys.NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, filesys.NewArchiveBudget(context.Background(), limits))
	if err != nil {
		t.Fatal(err)
	}
	e := NewExpandedZipFS(zfs, zfs)
	got, err := e.ReadFile("hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestT23_C02_StrictNestedPaths(t *testing.T) {
	raw := t23Zip(t, map[string][]byte{"../x": []byte("nope")})
	limits := filesys.ArchiveLimits{MaxArchiveEntries: 16, MaxArchiveEntryBytes: 1024, MaxArchiveTotalBytes: 1024}
	_, err := filesys.NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, filesys.NewArchiveBudget(context.Background(), limits))
	if err == nil || (!filesys.IsPathRejected(err) && !strings.Contains(err.Error(), "invalid_input") && !strings.Contains(err.Error(), "archive_path_rejected")) {
		t.Fatalf("strict nested policy must reject zip-slip, got %v", err)
	}
}

func TestT23_C03_NestedDepth(t *testing.T) {
	hello := []byte("ok")
	leaf := t23Zip(t, map[string][]byte{"hello.txt": hello})
	d1 := t23Zip(t, map[string][]byte{"d2.zip": leaf})
	outer := t23Zip(t, map[string][]byte{"d1.zip": d1})

	const L = 1
	limits := filesys.ArchiveLimits{
		MaxArchiveDepth:      L,
		MaxArchiveEntries:    64,
		MaxArchiveEntryBytes: 1 << 20,
		MaxArchiveTotalBytes: 1 << 20,
	}
	budget := filesys.NewArchiveBudget(context.Background(), limits)
	zfs, err := filesys.NewZipFSRawWithLimits(bytes.NewReader(d1), int64(len(d1)), limits, budget)
	if err != nil {
		t.Fatal(err)
	}
	e := NewExpandedZipFS(zfs, zfs)
	got, err := e.ReadFile("d2.zip/hello.txt")
	if err != nil {
		t.Fatalf("depth L should be readable: %v", err)
	}
	if string(got) != "ok" {
		t.Fatalf("got %q", got)
	}

	budget2 := filesys.NewArchiveBudget(context.Background(), limits)
	zfs2, err := filesys.NewZipFSRawWithLimits(bytes.NewReader(outer), int64(len(outer)), limits, budget2)
	if err != nil {
		t.Fatal(err)
	}
	e2 := NewExpandedZipFS(zfs2, zfs2)
	_, err = e2.ReadFile("d1.zip/d2.zip/hello.txt")
	t23MustResource(t, err)

	t.Run("shared_total_bytes", func(t *testing.T) {
		payload := bytes.Repeat([]byte("n"), 80)
		child := t23Zip(t, map[string][]byte{"b.txt": payload})
		parent := t23Zip(t, map[string][]byte{"child.zip": child})
		lim := filesys.ArchiveLimits{
			MaxArchiveEntryBytes: 10 << 10,
			MaxArchiveTotalBytes: int64(len(child) + 40),
			MaxArchiveEntries:    32,
			MaxArchiveDepth:      8,
		}
		b := filesys.NewArchiveBudget(context.Background(), lim)
		pz, err := filesys.NewZipFSRawWithLimits(bytes.NewReader(parent), int64(len(parent)), lim, b)
		if err != nil {
			t.Fatal(err)
		}
		ex := NewExpandedZipFS(pz, pz)
		_, err = ex.ReadFile("child.zip/b.txt")
		t23MustResource(t, err)
	})
}

func TestT23_C04_ExpandedEntryBudget(t *testing.T) {
	files := map[string][]byte{}
	for i := 0; i < 50; i++ {
		files["n"+itoa(i)+".txt"] = []byte("x")
	}
	raw := t23Zip(t, files)
	limits := filesys.ArchiveLimits{MaxArchiveEntries: 10, MaxArchiveEntryBytes: 1 << 20, MaxArchiveTotalBytes: 1 << 20}
	_, err := filesys.NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, filesys.NewArchiveBudget(context.Background(), limits))
	t23MustResource(t, err)
}

func TestT23_C05_ExpandedCancel(t *testing.T) {
	raw := t23Zip(t, map[string][]byte{"hello.txt": []byte("hello")})
	limits := filesys.ArchiveLimits{MaxArchiveEntries: 16, MaxArchiveEntryBytes: 1 << 20, MaxArchiveTotalBytes: 1 << 20}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)),
		filesys.WithArchiveLimits(limits),
		filesys.WithArchiveBudget(filesys.NewArchiveBudget(ctx, limits)),
		filesys.WithArchiveContext(ctx),
		filesys.WithSafeArchive(true),
	)
	if err == nil {
		zfs, err2 := filesys.NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)),
			filesys.WithArchiveLimits(limits),
			filesys.WithArchiveBudget(filesys.NewArchiveBudget(context.Background(), limits)),
			filesys.WithArchiveContext(ctx),
			filesys.WithSafeArchive(true),
		)
		if err2 != nil {
			err = err2
		} else {
			_, err = zfs.ReadFile("hello.txt")
		}
	}
	if err == nil {
		t.Fatal("expected canceled")
	}
	if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("expected canceled, got %v", err)
	}
}

func TestT23_C06_MultiReleaseJAR(t *testing.T) {
	base := []byte("BASE-A")
	v11 := []byte("VER11-A")
	lib := t23Zip(t, map[string][]byte{
		"com/acme/A.class":                      base,
		"META-INF/versions/11/com/acme/A.class": v11,
	})
	parent := t23Zip(t, map[string][]byte{"lib.jar": lib})
	limits := filesys.ArchiveLimits{MaxArchiveEntries: 64, MaxArchiveEntryBytes: 1 << 20, MaxArchiveTotalBytes: 1 << 20, MaxArchiveDepth: 8}

	open := func(tr int) *ExpandedZipFS {
		t.Helper()
		b := filesys.NewArchiveBudget(context.Background(), limits)
		zfs, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(parent), int64(len(parent)),
			filesys.WithArchiveLimits(limits),
			filesys.WithArchiveBudget(b),
			filesys.WithSafeArchive(true),
		)
		if err != nil {
			t.Fatal(err)
		}
		return NewExpandedZipFSWithArchiveOptions(zfs, zfs, true, filesys.ArchiveOptions{
			Limits:        limits,
			Budget:        b,
			TargetRelease: tr,
			SafeArchive:   true,
		})
	}

	readRaw := func(e *ExpandedZipFS) []byte {
		t.Helper()
		ufs, err := e.GetJarFS("lib.jar")
		if err != nil {
			t.Fatalf("GetJarFS: %v", err)
		}
		jfs, ok := ufs.GetFileSystem().(*JarFS)
		if !ok {
			t.Fatalf("inner fs %T", ufs.GetFileSystem())
		}
		got, err := jfs.ZipFS.ReadFile("com/acme/A.class")
		if err != nil {
			t.Fatalf("raw read: %v", err)
		}
		return got
	}

	e0 := open(0)
	e11 := open(11)
	e17 := open(17)
	e9 := open(9)
	if !bytes.Equal(readRaw(e0), base) {
		t.Fatal("TR0 want base")
	}
	if !bytes.Equal(readRaw(e9), base) {
		t.Fatal("TR9 want base")
	}
	if !bytes.Equal(readRaw(e11), v11) {
		t.Fatal("TR11 want versioned")
	}
	if !bytes.Equal(readRaw(e17), v11) {
		t.Fatal("TR17 want versioned")
	}
	if e0.archiveCacheKey("lib.jar") == e11.archiveCacheKey("lib.jar") {
		t.Fatal("cache key must include TargetRelease")
	}
	if filesys.ArchiveCacheKey("lib.jar", 0) == filesys.ArchiveCacheKey("lib.jar", 11) {
		t.Fatal("ArchiveCacheKey must include TargetRelease")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [16]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}
