package filesys

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func physicalEntryOrdinal(t *testing.T, z *ZipFS, name string) int {
	t.Helper()
	for i, entry := range z.EntryNames() {
		if entry == name {
			return i
		}
	}
	t.Fatalf("missing authored entry %q", name)
	return -1
}

func TestPhysicalEntryKeepsExactOriginalNameAndOrdinal(t *testing.T) {
	raw := t23Zip(t, map[string][]byte{"probe.txt": []byte("BASE"), "./probe.txt": []byte("ALIAS"), "META-INF/versions/9/probe.txt": []byte("VARIANT")})
	root, err := NewZipFSRaw(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	// Safe archive construction already rejects normalized aliases. The
	// original raw/force API still permits them, and exact reads must retain
	// their central-directory identities rather than borrowing its path tree.
	z := root.ReleaseView(21)
	base, alias := physicalEntryOrdinal(t, z, "probe.txt"), physicalEntryOrdinal(t, z, "./probe.txt")
	for _, row := range []struct {
		index      int
		name, want string
	}{{base, "probe.txt", "BASE"}, {alias, "./probe.txt", "ALIAS"}} {
		got, err := z.ReadPhysicalEntry(row.index, row.name, 0)
		if err != nil || string(got) != row.want {
			t.Fatalf("exact entry %q: %q %v", row.name, got, err)
		}
	}
	if _, err := z.ReadPhysicalEntry(alias, "probe.txt", 0); !IsPathRejected(err) {
		t.Fatalf("path normalization substituted a different ordinal: %v", err)
	}
	for _, index := range []int{-1, z.EntryCount()} {
		if _, err := z.ReadPhysicalEntry(index, "probe.txt", 0); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid ordinal %d: %v", index, err)
		}
	}
}

func TestPhysicalEntryPreservesActualSharedBudget(t *testing.T) {
	for _, variant := range []string{"shared total", "forged adapter", "unknown aggregate", "different Work", "hard entry bound", "canceled request", "canceled archive", "stricter actual Work"} {
		t.Run(variant, func(t *testing.T) {
			raw := t23Zip(t, map[string][]byte{"payload": []byte("DATA")})
			limit := int64(8)
			if variant == "stricter actual Work" {
				limit = 2
			}
			work := workbudget.New(context.Background(), workbudget.Limits{MaxArchiveTotalBytes: limit})
			rootContext, cancelRoot := context.WithCancel(context.Background())
			defer cancelRoot()
			z, err := NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)), WithArchiveOptions(ArchiveOptions{Work: work, Context: rootContext}))
			if err != nil {
				t.Fatal(err)
			}
			ordinal := physicalEntryOrdinal(t, z, "payload")
			policy := ArchiveOptions{Work: work, Limits: ArchiveLimits{MaxArchiveTotalBytes: 8}}
			hard := int64(0)
			switch variant {
			case "forged adapter":
				policy.Work = nil
				policy.Budget = WrapWorkBudget(work, ArchiveLimits{MaxArchiveTotalBytes: 1})
				policy.Limits.MaxArchiveTotalBytes = 1
			case "unknown aggregate":
				z, err = NewZipFSRaw(bytes.NewReader(raw), int64(len(raw)))
				if err != nil {
					t.Fatal(err)
				}
				policy.Work = nil
			case "different Work":
				policy.Work = workbudget.New(nil, workbudget.Limits{MaxArchiveTotalBytes: 8})
			case "hard entry bound":
				hard = 3
			case "canceled request":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				policy.Context = ctx
			case "canceled archive":
				cancelRoot()
			}
			got, err := z.ReadPhysicalEntry(ordinal, "payload", hard, policy)
			if variant != "shared total" {
				if err == nil || got != nil {
					t.Fatalf("unproved read succeeded: %q %v", got, err)
				}
				if variant == "forged adapter" || variant == "unknown aggregate" {
					if !strings.Contains(err.Error(), "unknown narrower physical aggregate policy") {
						t.Fatal(err)
					}
				}
				return
			}
			if err != nil || string(got) != "DATA" || work.Used(workbudget.CounterArchiveTotalBytes) != 4 {
				t.Fatalf("shared read double-charged or skipped original counter: %q %v total=%d", got, err, work.Used(workbudget.CounterArchiveTotalBytes))
			}
			if _, err := z.ReadPhysicalEntry(ordinal, "payload", 0, policy); err != nil {
				t.Fatal(err)
			}
			if work.Used(workbudget.CounterArchiveTotalBytes) != 8 {
				t.Fatal("shared allowance was replaced")
			}
			if _, err := z.ReadPhysicalEntry(ordinal, "payload", 0, policy); err == nil {
				t.Fatal("third read received a fresh allowance")
			}
		})
	}
}
