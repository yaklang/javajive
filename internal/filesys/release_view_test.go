package filesys

import (
	"bytes"
	"context"
	"testing"
)

type releaseViewCloseSpy struct{ calls int }

func (s *releaseViewCloseSpy) Close() error { s.calls++; return nil }

func TestReleaseViewRetainsBudgetReaderAndPhysicalIdentity(t *testing.T) {
	raw := t23Zip(t, map[string][]byte{"A.class": []byte("base"), "META-INF/versions/9/A.class": []byte("nine"), "META-INF/versions/11/A.class": []byte("eleven")})
	limits := ArchiveLimits{MaxArchiveEntries: 8, MaxArchiveEntryBytes: 1024, MaxArchiveTotalBytes: 1024, MaxArchiveDepth: 2}
	budget := NewArchiveBudget(context.Background(), limits)
	z, e := NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)), WithArchiveLimits(limits), WithArchiveBudget(budget), WithSafeArchive(true))
	if e != nil {
		t.Fatal(e)
	}
	spy := &releaseViewCloseSpy{}
	z.closer = spy
	for _, n := range []int{11, 9} {
		v := z.ReleaseView(n)
		if v == nil || v.ArchiveBudget() != z.ArchiveBudget() || v.ArchiveBudget().Work() != budget.Work() || v.r != z.r || v.forest != z.forest || v.TargetRelease() != n || z.TargetRelease() != 0 {
			t.Fatal("view changed reader, budget, or parent policy")
		}
		if e := v.Close(); e != nil || spy.calls != 0 {
			t.Fatal("view closed borrowed parent")
		}
		if b, e := v.ReadFile("A.class"); e != nil || string(b) != map[int]string{9: "nine", 11: "eleven"}[n] {
			t.Fatalf("selected read %s %v", b, e)
		}
		if b, e := v.ReadFile("META-INF/versions/9/A.class"); e != nil || string(b) != "nine" {
			t.Fatalf("physical read %s %v", b, e)
		}
	}
	if b, e := z.ReadFile("A.class"); e != nil || string(b) != "base" {
		t.Fatalf("parent read %s %v", b, e)
	}
	if z.ReleaseView(8) != nil {
		t.Fatal("invalid view accepted")
	}
	if e := z.Close(); e != nil || spy.calls != 1 {
		t.Fatal("parent ownership changed")
	}
}
