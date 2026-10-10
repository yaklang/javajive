package filesys

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	zipx "github.com/yaklang/javajive/internal/zipx"
)

func t23Zip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	return t23ZipMethod(t, files, zip.Deflate)
}

func t23ZipMethod(t *testing.T, files map[string][]byte, method uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		hdr := &zip.FileHeader{Name: name, Method: method}
		fw, err := w.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := fw.Write(body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func t23OpenLimited(t *testing.T, raw []byte, limits ArchiveLimits) *ZipFS {
	t.Helper()
	budget := NewArchiveBudget(context.Background(), limits)
	zfs, err := NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, budget)
	if err != nil {
		t.Fatalf("NewZipFSRawWithLimits: %v", err)
	}
	return zfs
}

func t23MustResourceLimit(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected resource_limit error")
	}
	if !IsResourceLimit(err) && !strings.Contains(err.Error(), KindResource) {
		t.Fatalf("expected resource_limit, got %v", err)
	}
}

func t23MustPathRejected(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected path rejection")
	}
	if !IsPathRejected(err) {
		t.Fatalf("expected invalid_input/archive_path_rejected, got %v", err)
	}
}

func TestT23_C01_BoundedDecompression(t *testing.T) {
	const payload = 2 << 20 // 2MiB zeros
	const capBytes int64 = 64 << 10

	raw := t23Zip(t, map[string][]byte{"zeros.bin": make([]byte, payload)})
	if len(raw) >= payload {
		t.Fatalf("expected highly compressible zip, got %d bytes", len(raw))
	}

	t.Run("actual_bytes_over_limit", func(t *testing.T) {
		limits := ArchiveLimits{MaxArchiveEntryBytes: capBytes, MaxArchiveTotalBytes: 8 << 20, MaxArchiveEntries: 16}
		zfs := t23OpenLimited(t, raw, limits)
		_, err := zfs.ReadFile("zeros.bin")
		t23MustResourceLimit(t, err)
	})

	t.Run("lying_small_header_zip", func(t *testing.T) {
		patched := patchFirstCDHUncompressed(t, raw, 16)
		limits := ArchiveLimits{MaxArchiveEntryBytes: capBytes, MaxArchiveTotalBytes: 8 << 20, MaxArchiveEntries: 16}
		zfs := t23OpenLimited(t, patched, limits)
		_, err := zfs.ReadFile("zeros.bin")
		t23MustResourceLimit(t, err)
	})

	t.Run("lying_small_header_custom_reader", func(t *testing.T) {
		limits := ArchiveLimits{MaxArchiveEntryBytes: capBytes}
		budget := NewArchiveBudget(context.Background(), limits)
		r := bytes.NewReader(make([]byte, payload))
		_, err := ReadArchiveStream(context.Background(), r, 8, budget, limits)
		t23MustResourceLimit(t, err)
	})

	t.Run("maxint64_limit_nonempty_success", func(t *testing.T) {
		// Regression: LimitReader(MaxInt64+1) wrapped negative and returned empty success.
		in := []byte("abc")
		got, err := ReadArchiveStream(context.Background(), bytes.NewReader(in), 3, nil, ArchiveLimits{MaxArchiveEntryBytes: math.MaxInt64})
		if err != nil {
			t.Fatalf("MaxInt64 cap rejected nonempty stream: %v", err)
		}
		if string(got) != "abc" {
			t.Fatalf("got %q want abc (empty success on overflow)", got)
		}
	})

	t.Run("uint64_header_does_not_go_negative", func(t *testing.T) {
		_, err := ReadArchiveStream(context.Background(), bytes.NewReader([]byte("x")), ^uint64(0), nil, ArchiveLimits{MaxArchiveEntryBytes: 8})
		t23MustResourceLimit(t, err)
	})

	t.Run("early_reject_lying_large_header", func(t *testing.T) {
		limits := ArchiveLimits{MaxArchiveEntryBytes: capBytes}
		_, err := ReadArchiveStream(context.Background(), bytes.NewReader(nil), uint64(1<<30), nil, limits)
		t23MustResourceLimit(t, err)
	})

	t.Run("per_entry_not_cumulative", func(t *testing.T) {
		raw := t23Zip(t, map[string][]byte{
			"a.txt": bytes.Repeat([]byte("A"), 40),
			"b.txt": bytes.Repeat([]byte("B"), 40),
		})
		limits := ArchiveLimits{MaxArchiveEntryBytes: 50, MaxArchiveTotalBytes: 200, MaxArchiveEntries: 16}
		zfs := t23OpenLimited(t, raw, limits)
		a, err := zfs.ReadFile("a.txt")
		if err != nil {
			t.Fatalf("a.txt: %v", err)
		}
		b, err := zfs.ReadFile("b.txt")
		if err != nil {
			t.Fatalf("b.txt should pass per-entry cap: %v", err)
		}
		if len(a) != 40 || len(b) != 40 {
			t.Fatalf("unexpected sizes %d %d", len(a), len(b))
		}
	})

	t.Run("positive_control_text_file", func(t *testing.T) {
		raw := t23Zip(t, map[string][]byte{"hello.txt": []byte("hello")})
		limits := ArchiveLimits{MaxArchiveEntryBytes: 1 << 20, MaxArchiveTotalBytes: 1 << 20, MaxArchiveEntries: 64, MaxArchiveDepth: 8}
		zfs := t23OpenLimited(t, raw, limits)
		got, err := zfs.ReadFile("hello.txt")
		if err != nil {
			t.Fatalf("positive control: %v", err)
		}
		if string(got) != "hello" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("overflow_guard", func(t *testing.T) {
		b := NewArchiveBudget(context.Background(), ArchiveLimits{})
		if err := b.Charge(CounterArchiveTotalBytes, math.MaxInt64); err != nil {
			t.Fatalf("first charge: %v", err)
		}
		if err := b.Charge(CounterArchiveTotalBytes, 1); err == nil {
			t.Fatal("expected overflow resource_limit")
		} else {
			t23MustResourceLimit(t, err)
		}
	})
}

func TestT23_C02_PathsAndDuplicates(t *testing.T) {
	t.Run("normalize_helper", func(t *testing.T) {
		if _, err := NormalizeArchiveEntryName("../x"); err == nil {
			t.Fatal("expected reject ../x")
		} else {
			t23MustPathRejected(t, err)
		}
		if _, err := NormalizeArchiveEntryName("/abs"); err == nil {
			t.Fatal("expected reject /abs")
		} else {
			t23MustPathRejected(t, err)
		}
		if _, err := NormalizeArchiveEntryName("C:/windows/x"); err == nil {
			t.Fatal("expected reject drive path")
		} else {
			t23MustPathRejected(t, err)
		}
		if _, err := NormalizeArchiveEntryName("foo\x00bar"); err == nil {
			t.Fatal("expected reject NUL")
		} else {
			t23MustPathRejected(t, err)
		}
		got, err := NormalizeArchiveEntryName(`foo\bar`)
		if err != nil {
			t.Fatalf("backslash should normalize, not reject: %v", err)
		}
		if got != "foo/bar" {
			t.Fatalf("backslash policy: got %q want foo/bar", got)
		}
		got, err = NormalizeArchiveEntryName("foo/../bar")
		if err != nil {
			t.Fatalf("internal .. that stays inside should clean: %v", err)
		}
		if got != "bar" {
			t.Fatalf("got %q", got)
		}
	})

	limits := ArchiveLimits{MaxArchiveEntries: 32, MaxArchiveEntryBytes: 1 << 20, MaxArchiveTotalBytes: 1 << 20}

	t.Run("zip_slip_rejected", func(t *testing.T) {
		raw := t23Zip(t, map[string][]byte{"../x": []byte("nope")})
		_, err := NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, NewArchiveBudget(context.Background(), limits))
		t23MustPathRejected(t, err)
	})

	t.Run("absolute_rejected", func(t *testing.T) {
		raw := t23Zip(t, map[string][]byte{"/abs": []byte("nope")})
		_, err := NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, NewArchiveBudget(context.Background(), limits))
		t23MustPathRejected(t, err)
	})

	t.Run("backslash_normalized", func(t *testing.T) {
		raw := t23Zip(t, map[string][]byte{`foo\bar`: []byte("ok")})
		zfs, err := NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, NewArchiveBudget(context.Background(), limits))
		if err != nil {
			t.Fatalf("backslash entry should be accepted after normalize: %v", err)
		}
		got, err := zfs.ReadFile("foo/bar")
		if err != nil {
			t.Fatalf("read foo/bar: %v", err)
		}
		if string(got) != "ok" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("duplicate_names", func(t *testing.T) {
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		for i := 0; i < 2; i++ {
			fw, err := w.Create("same.txt")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fw.Write([]byte("x"))
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		raw := buf.Bytes()
		_, err := NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, NewArchiveBudget(context.Background(), limits))
		t23MustPathRejected(t, err)
	})

	t.Run("case_fold_collision", func(t *testing.T) {
		raw := t23Zip(t, map[string][]byte{
			"Foo.class": []byte("A"),
			"foo.class": []byte("B"),
		})
		_, err := NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, NewArchiveBudget(context.Background(), limits))
		t23MustPathRejected(t, err)
	})

	t.Run("file_vs_directory", func(t *testing.T) {
		raw := t23Zip(t, map[string][]byte{
			"foo":     []byte("file"),
			"foo/bar": []byte("nested"),
		})
		_, err := NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, NewArchiveBudget(context.Background(), limits))
		t23MustPathRejected(t, err)
	})

	t.Run("default_unlimited_keeps_historical", func(t *testing.T) {
		raw := t23Zip(t, map[string][]byte{"hello.txt": []byte("hello")})
		zfs, err := NewZipFSFromString(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		got, err := zfs.ReadFile("hello.txt")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "hello" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestT23_C03_NestedDepth(t *testing.T) {
	hello := []byte("ok")
	inner := t23Zip(t, map[string][]byte{"hello.txt": hello})
	inner2 := t23Zip(t, map[string][]byte{"d2.zip": inner})
	outer := t23Zip(t, map[string][]byte{"d1.zip": inner2})

	t.Run("depth_L_ok_Lplus1_rejected", func(t *testing.T) {
		const L = 1
		limits := ArchiveLimits{MaxArchiveDepth: L, MaxArchiveEntries: 32, MaxArchiveEntryBytes: 1 << 20, MaxArchiveTotalBytes: 1 << 20}
		budget := NewArchiveBudget(context.Background(), limits)
		zfs, err := NewZipFSRawWithLimits(bytes.NewReader(inner), int64(len(inner)), limits, budget)
		if err != nil {
			t.Fatal(err)
		}
		got, err := zfs.ReadFile("hello.txt")
		if err != nil {
			t.Fatalf("depth 0 read: %v", err)
		}
		if string(got) != "ok" {
			t.Fatalf("got %q", got)
		}

		nested, err := NewZipFSRawWithOptions(bytes.NewReader(inner), int64(len(inner)),
			WithArchiveLimits(limits), WithArchiveBudget(budget), WithArchiveDepth(L), WithSafeArchive(true))
		if err != nil {
			t.Fatalf("depth L should be allowed: %v", err)
		}
		got, err = nested.ReadFile("hello.txt")
		if err != nil {
			t.Fatalf("depth L read: %v", err)
		}
		if string(got) != "ok" {
			t.Fatalf("got %q", got)
		}

		_, err = NewZipFSRawWithOptions(bytes.NewReader(inner), int64(len(inner)),
			WithArchiveLimits(limits), WithArchiveBudget(budget), WithArchiveDepth(L+1), WithSafeArchive(true))
		t23MustResourceLimit(t, err)
	})

	t.Run("shared_total_across_layers", func(t *testing.T) {
		payload := bytes.Repeat([]byte("x"), 80)
		child := t23Zip(t, map[string][]byte{"b.txt": payload})
		parent := t23Zip(t, map[string][]byte{"child.zip": child})
		limits := ArchiveLimits{
			MaxArchiveEntryBytes: 10 << 10,
			MaxArchiveTotalBytes: int64(len(child) + 40),
			MaxArchiveEntries:    32,
			MaxArchiveDepth:      4,
		}
		budget := NewArchiveBudget(context.Background(), limits)
		zfs, err := NewZipFSRawWithLimits(bytes.NewReader(parent), int64(len(parent)), limits, budget)
		if err != nil {
			t.Fatal(err)
		}
		childBytes, err := zfs.ReadFile("child.zip")
		if err != nil {
			t.Fatalf("read child.zip: %v", err)
		}
		inner, err := NewZipFSRawWithOptions(bytes.NewReader(childBytes), int64(len(childBytes)), zfs.NestedArchiveOptions(1)...)
		if err != nil {
			t.Fatalf("open nested: %v", err)
		}
		_, err = inner.ReadFile("b.txt")
		t23MustResourceLimit(t, err)
	})

	_ = outer
}

func TestT23_C04_ManySmallEntries(t *testing.T) {
	files := make(map[string][]byte, 50)
	for i := 0; i < 50; i++ {
		files["f"+strings.Repeat("0", 2)+itoa(i)] = []byte("x")
	}
	raw := t23Zip(t, files)
	limits := ArchiveLimits{MaxArchiveEntries: 10, MaxArchiveEntryBytes: 1 << 20, MaxArchiveTotalBytes: 1 << 20}
	_, err := NewZipFSRawWithLimits(bytes.NewReader(raw), int64(len(raw)), limits, NewArchiveBudget(context.Background(), limits))
	t23MustResourceLimit(t, err)
}

func TestT23_C05_CorruptAndCancel(t *testing.T) {
	raw := t23Zip(t, map[string][]byte{"hello.txt": []byte("hello world")})
	limits := ArchiveLimits{MaxArchiveEntries: 16, MaxArchiveEntryBytes: 1 << 20, MaxArchiveTotalBytes: 1 << 20}

	t.Run("crc_mismatch", func(t *testing.T) {
		bad := flipFirstCDHCRC(t, raw)
		zfs, err := NewZipFSRawWithLimits(bytes.NewReader(bad), int64(len(bad)), limits, NewArchiveBudget(context.Background(), limits))
		if err != nil {
			t.Fatalf("catalog of crc-flipped zip: %v", err)
		}
		data, err := zfs.ReadFile("hello.txt")
		if err == nil {
			t.Fatalf("CRC mismatch must not succeed, got %q", data)
		}
		if !errors.Is(err, zipx.ErrChecksum) && !strings.Contains(strings.ToLower(err.Error()), "checksum") {
			t.Fatalf("expected checksum error, got %v", err)
		}
	})

	t.Run("truncated_zip", func(t *testing.T) {
		if len(raw) < 20 {
			t.Fatal("zip too small")
		}
		trunc := raw[:len(raw)/2]
		_, err := NewZipFSRawWithLimits(bytes.NewReader(trunc), int64(len(trunc)), limits, NewArchiveBudget(context.Background(), limits))
		if err == nil {
			zfs, err2 := NewZipFSFromString(string(trunc))
			if err2 == nil {
				_, err2 = zfs.ReadFile("hello.txt")
			}
			if err2 == nil {
				t.Fatal("truncated zip must not succeed")
			}
			return
		}
	})

	t.Run("truncated_stream", func(t *testing.T) {
		r := &errAfter{n: 3, err: io.ErrUnexpectedEOF, r: bytes.NewReader([]byte("hello world"))}
		_, err := ReadArchiveStream(context.Background(), r, 11, nil, ArchiveLimits{MaxArchiveEntryBytes: 1 << 20})
		if err == nil {
			t.Fatal("truncated stream must not return success")
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("expected unexpected EOF, got %v", err)
		}
	})

	t.Run("context_cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		zfs, err := NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)),
			WithArchiveLimits(limits), WithArchiveContext(ctx), WithSafeArchive(true),
			WithArchiveBudget(NewArchiveBudget(ctx, limits)))
		if err != nil {
			if !IsCanceled(err) && !strings.Contains(err.Error(), KindCanceled) {
				t.Fatalf("constructor cancel: %v", err)
			}
			return
		}
		_, err = zfs.ReadFile("hello.txt")
		if err == nil {
			t.Fatal("canceled context must not succeed")
		}
		if !IsCanceled(err) && !strings.Contains(err.Error(), KindCanceled) && !errors.Is(err, context.Canceled) {
			t.Fatalf("expected canceled, got %v", err)
		}
	})

	t.Run("cancel_during_read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		r := &cancelAfter{cancel: cancel, leftover: bytes.NewReader(bytes.Repeat([]byte("z"), 64*1024))}
		_, err := ReadArchiveStream(ctx, r, 64*1024, NewArchiveBudget(ctx, limits), limits)
		if err == nil {
			t.Fatal("expected cancel during read")
		}
		if !IsCanceled(err) && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), KindCanceled) {
			t.Fatalf("expected canceled, got %v", err)
		}
	})
}

func TestT23_C06_MultiReleaseJAR(t *testing.T) {
	base := []byte("BASE-A")
	v11 := []byte("VER11-A")
	raw := t23Zip(t, map[string][]byte{
		"com/acme/A.class":                      base,
		"META-INF/versions/11/com/acme/A.class": v11,
		"note.txt":                              []byte("plain"),
	})
	entries := []string{"com/acme/A.class", "META-INF/versions/11/com/acme/A.class", "note.txt"}

	t.Run("select_helper", func(t *testing.T) {
		if got, ok := SelectMultiReleasePath(entries, "com/acme/A.class", 0); !ok || got != "com/acme/A.class" {
			t.Fatalf("TR0: got %q ok=%v", got, ok)
		}
		if got, ok := SelectMultiReleasePath(entries, "com/acme/A.class", 9); !ok || got != "com/acme/A.class" {
			t.Fatalf("TR9: got %q ok=%v", got, ok)
		}
		if got, ok := SelectMultiReleasePath(entries, "com/acme/A.class", 11); !ok || got != "META-INF/versions/11/com/acme/A.class" {
			t.Fatalf("TR11: got %q ok=%v", got, ok)
		}
		if got, ok := SelectMultiReleasePath(entries, "com/acme/A.class", 17); !ok || got != "META-INF/versions/11/com/acme/A.class" {
			t.Fatalf("TR17: got %q ok=%v", got, ok)
		}
	})

	limits := ArchiveLimits{MaxArchiveEntries: 32, MaxArchiveEntryBytes: 1 << 20, MaxArchiveTotalBytes: 1 << 20, MaxArchiveDepth: 4}

	read := func(tr int) []byte {
		t.Helper()
		zfs, err := NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)),
			WithArchiveLimits(limits),
			WithArchiveBudget(NewArchiveBudget(context.Background(), limits)),
			WithTargetRelease(tr),
			WithSafeArchive(true),
		)
		if err != nil {
			t.Fatalf("open tr=%d: %v", tr, err)
		}
		got, err := zfs.ReadFile("com/acme/A.class")
		if err != nil {
			t.Fatalf("read tr=%d: %v", tr, err)
		}
		return got
	}

	if !bytes.Equal(read(0), base) {
		t.Fatalf("TR0 want base")
	}
	if !bytes.Equal(read(9), base) {
		t.Fatalf("TR9 want base")
	}
	if !bytes.Equal(read(11), v11) {
		t.Fatalf("TR11 want versioned")
	}
	if !bytes.Equal(read(17), v11) {
		t.Fatalf("TR17 want versioned")
	}

	k0 := ArchiveCacheKey("lib.jar", 0)
	k11 := ArchiveCacheKey("lib.jar", 11)
	if k0 == k11 {
		t.Fatal("cache key must include TargetRelease")
	}

	t.Run("positive_control", func(t *testing.T) {
		zfs, err := NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)),
			WithArchiveLimits(limits), WithTargetRelease(11), WithSafeArchive(true),
			WithArchiveBudget(NewArchiveBudget(context.Background(), limits)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := zfs.ReadFile("note.txt")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "plain" {
			t.Fatalf("got %q", got)
		}
	})
}

func patchFirstCDHUncompressed(t *testing.T, raw []byte, size uint32) []byte {
	t.Helper()
	out := append([]byte(nil), raw...)
	idx := bytes.Index(out, []byte{'P', 'K', 0x01, 0x02})
	if idx < 0 || idx+28 > len(out) {
		t.Fatal("central directory header not found")
	}
	binary.LittleEndian.PutUint32(out[idx+24:], size)
	return out
}

func flipFirstCDHCRC(t *testing.T, raw []byte) []byte {
	t.Helper()
	out := append([]byte(nil), raw...)
	idx := bytes.Index(out, []byte{'P', 'K', 0x01, 0x02})
	if idx < 0 || idx+20 > len(out) {
		t.Fatal("central directory header not found")
	}
	out[idx+16] ^= 0xff
	return out
}

type errAfter struct {
	n   int
	err error
	r   io.Reader
}

func (e *errAfter) Read(p []byte) (int, error) {
	if e.n <= 0 {
		return 0, e.err
	}
	if len(p) > e.n {
		p = p[:e.n]
	}
	n, err := e.r.Read(p)
	e.n -= n
	if e.n <= 0 {
		return n, e.err
	}
	return n, err
}

type cancelAfter struct {
	once     bool
	cancel   context.CancelFunc
	leftover io.Reader
}

func (c *cancelAfter) Read(p []byte) (int, error) {
	if c.once {
		c.cancel()
	}
	c.once = true
	return c.leftover.Read(p)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [8]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}
