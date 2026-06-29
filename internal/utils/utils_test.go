package utils

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStringSliceContain(t *testing.T) {
	if !StringSliceContain([]string{"a", "b"}, "b") {
		t.Fatal("should contain b")
	}
	if StringSliceContain([]string{"a", "b"}, "c") {
		t.Fatal("should not contain c")
	}
}

func TestGetLastElement(t *testing.T) {
	if got := GetLastElement([]int{1, 2, 3}); got != 3 {
		t.Fatalf("GetLastElement = %d", got)
	}
	if got := GetLastElement([]int{}); got != 0 {
		t.Fatalf("GetLastElement empty = %d want 0", got)
	}
}

func TestCopyMapShallow(t *testing.T) {
	orig := map[string]int{"a": 1, "b": 2}
	cp := CopyMapShallow(orig)
	cp["a"] = 99
	if orig["a"] != 1 {
		t.Fatal("CopyMapShallow must not mutate original")
	}
	if cp["b"] != 2 {
		t.Fatal("CopyMapShallow lost a value")
	}
}

func TestStringArrayFilterEmpty(t *testing.T) {
	got := StringArrayFilterEmpty([]string{"a", "", "  ", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("StringArrayFilterEmpty = %v", got)
	}
}

func TestParseStringToLines(t *testing.T) {
	got := ParseStringToLines("a\n\n  b  \nc\n")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("ParseStringToLines = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q want %q", i, got[i], want[i])
		}
	}
}

func TestEscapeInvalidUTF8Byte(t *testing.T) {
	if got := EscapeInvalidUTF8Byte([]byte("ok")); got != "ok" {
		t.Fatalf("plain ascii escaped = %q", got)
	}
	if got := EscapeInvalidUTF8Byte([]byte{0xff}); got != "\\xff" {
		t.Fatalf("invalid byte escaped = %q", got)
	}
}

func TestSimplifyUtf8RoundTrip(t *testing.T) {
	in := []byte("héllo世界")
	out, err := SimplifyUtf8(in)
	if err != nil {
		t.Fatalf("SimplifyUtf8: %v", err)
	}
	if string(out) != string(in) {
		t.Fatalf("SimplifyUtf8 round-trip = %q want %q", out, in)
	}
}

func TestRandStringBytes(t *testing.T) {
	s := RandStringBytes(16)
	if len(s) != 16 {
		t.Fatalf("RandStringBytes len = %d", len(s))
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			t.Fatalf("unexpected char %q", c)
		}
	}
}

func TestPathHelpers(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := PathExists(file); !ok {
		t.Fatal("PathExists should be true")
	}
	if ok, _ := PathExists(filepath.Join(dir, "nope")); ok {
		t.Fatal("PathExists should be false")
	}
	if !IsFile(file) || IsDir(file) {
		t.Fatal("IsFile/IsDir wrong for file")
	}
	if !IsDir(dir) || IsFile(dir) {
		t.Fatal("IsFile/IsDir wrong for dir")
	}
	if got := GetFirstExistedFile(filepath.Join(dir, "nope"), file); got != file {
		t.Fatalf("GetFirstExistedFile = %q want %q", got, file)
	}
	if got := GetFirstExistedPath(filepath.Join(dir, "nope"), dir); got != dir {
		t.Fatalf("GetFirstExistedPath = %q want %q", got, dir)
	}
}

func TestErrors(t *testing.T) {
	err := Error("boom")
	if err == nil || err.Error() != "boom" {
		t.Fatalf("Error = %v", err)
	}
	werr := Wrap(Errorf("inner %d", 1), "outer")
	if werr == nil || werr.Error() == "" {
		t.Fatal("Wrap returned empty")
	}
	if Wrap(nil, "x") != nil {
		t.Fatal("Wrap(nil) should be nil")
	}
}

func TestTTLCache(t *testing.T) {
	c := NewTTLCacheWithKey[string, int](50 * time.Millisecond)
	c.Set("a", 1)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get = %d,%v", v, ok)
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Fatal("entry should have expired")
	}

	c2 := NewTTLCacheWithKey[string, int]() // no TTL
	c2.Set("k", 7)
	if v, ok := c2.Get("k"); !ok || v != 7 {
		t.Fatalf("no-ttl Get = %d,%v", v, ok)
	}
	c2.Delete("k")
	if _, ok := c2.Get("k"); ok {
		t.Fatal("Delete failed")
	}
}

func TestSetAndStack(t *testing.T) {
	s := NewSet[string]()
	s.Add("x")
	if !s.Has("x") {
		t.Fatal("set should contain x")
	}

	st := NewStack[int]()
	st.Push(1)
	st.Push(2)
	if st.Pop() != 2 || st.Pop() != 1 {
		t.Fatal("stack LIFO order wrong")
	}
}
