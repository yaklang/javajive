package javaclassparser

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

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

func t23MustResource(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "resource_limit") {
		t.Fatalf("expected resource_limit, got %v", err)
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
