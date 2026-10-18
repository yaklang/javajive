package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialJarReleaseSelectionNeverFallsBackAfterUnknown(t *testing.T) {
	manifest := []byte("Manifest-Version: 1.0\nMulti-Release: true\n\n")
	for _, variant := range []string{"duplicate selected variant", "unrelated duplicate resource", "entry byte budget", "aggregate byte budget", "graph budget", "explicit context", "physical entry bound", "manifest stream bound"} {
		t.Run(variant, func(t *testing.T) {
			entries := []jarReleaseTestEntry{{"META-INF/MANIFEST.MF", manifest}, {"payload", []byte("BASE")}, {"META-INF/versions/9/payload", []byte("NEXT")}}
			limits := workbudget.Limits{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch variant {
			case "duplicate selected variant":
				entries = append(entries, jarReleaseTestEntry{"META-INF/versions/9/payload", []byte("OTHER")})
			case "unrelated duplicate resource":
				entries = append(entries, jarReleaseTestEntry{"unrelated", []byte("a")}, jarReleaseTestEntry{"unrelated", []byte("b")})
			case "entry byte budget":
				limits.MaxArchiveEntryBytes = 5
			case "aggregate byte budget":
				limits.MaxArchiveTotalBytes = 20
			case "graph budget":
				limits.MaxGraphScans = 1
			case "physical entry bound":
				for i := len(entries); i <= jarManifestEntryLimit; i++ {
					entries = append(entries, jarReleaseTestEntry{fmt.Sprintf("e/%d", i), nil})
				}
			case "manifest stream bound":
				entries[0].data = append(manifest, bytes.Repeat([]byte{' '}, jarManifestByteLimit)...)
			}
			raw := jarReleaseTestZIP(t, entries)
			work := workbudget.New(context.Background(), limits)
			zip, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)), filesys.WithArchiveOptions(ArchiveOptionsFrom(DecompileOptions{Work: work, Context: ctx, TargetRelease: 21, Limits: limits})))
			if variant == "duplicate selected variant" || variant == "unrelated duplicate resource" {
				// The safe constructor rejects duplicates before selection. Keep
				// its guard intact and exercise the supported legacy raw view,
				// whose exact central-directory catalog can contain duplicates.
				zip, err = filesys.NewZipFSRaw(bytes.NewReader(raw), int64(len(raw)))
				if err == nil {
					zip = zip.ReleaseView(21)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if variant == "explicit context" {
				cancel()
			}
			z := NewJarFS(zip)
			got, err := z.readJarEntry("payload")
			if variant == "unrelated duplicate resource" {
				if err != nil || string(got) != "NEXT" {
					t.Fatalf("unrelated resource denied selected original entry: %q %v", got, err)
				}
				return
			}
			if err == nil || got != nil {
				t.Fatalf("unknown selection fell back to original base/another entry: %q %v", got, err)
			}
			if variant == "duplicate selected variant" && !strings.Contains(err.Error(), "duplicate name") {
				t.Fatal(err)
			}
			if variant == "physical entry bound" && !strings.Contains(err.Error(), "archive entry bound") {
				t.Fatal(err)
			}
		})
	}
}

func TestAdversarialJarReleaseSelectionIndexesVariantsOnce(t *testing.T) {
	entries := []jarReleaseTestEntry{{"META-INF/MANIFEST.MF", []byte("Manifest-Version: 1.0\nMulti-Release: true\n\n")}, {"payload", []byte("BASE")}}
	for release := 9; release < 265; release++ {
		entries = append(entries, jarReleaseTestEntry{fmt.Sprintf("META-INF/versions/%d/payload", release), []byte(fmt.Sprint(release))})
	}
	raw := jarReleaseTestZIP(t, entries)
	work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 100000})
	zip, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)), filesys.WithArchiveOptions(filesys.ArchiveOptions{Work: work}))
	if err != nil {
		t.Fatal(err)
	}
	p := newJarReleaseSelection(zip, nil)
	if _, err := p.catalog(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.runtimeMultiRelease(); err != nil {
		t.Fatal(err)
	}
	for _, release := range []int{8, 9, 17, 65, 264, 300} {
		t.Run(fmt.Sprint(release), func(t *testing.T) {
			before := work.Used(workbudget.CounterGraphScans)
			got, err := p.selectPath("payload", release, jarReleaseRuntime)
			want := "payload"
			if release >= 9 {
				selected := release
				if selected > 264 {
					selected = 264
				}
				want = fmt.Sprintf("META-INF/versions/%d/payload", selected)
			}
			if err != nil || got != want {
				t.Fatalf("selected %q %v want %q", got, err, want)
			}
			// The warmed 256-variant catalog is indexed. A lookup may charge
			// key bytes and logarithmic comparisons, never another catalog scan.
			if delta := work.Used(workbudget.CounterGraphScans) - before; delta > 48 {
				t.Fatalf("warmed lookup rescanned catalog: %d operations", delta)
			}
		})
	}
}
