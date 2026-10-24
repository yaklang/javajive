package javaclassparser

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialJarReleaseSelectionSharesOriginalWork(t *testing.T) {
	manifest := []byte("Manifest-Version: 1.0\nMulti-Release: true\n\n")
	raw := jarReleaseTestZIP(t, []jarReleaseTestEntry{{"META-INF/MANIFEST.MF", manifest}, {"payload", []byte("BASE")}, {"META-INF/versions/9/payload", []byte("NEXT")}})
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit shared Work", true: "legacy metered request"}[legacy], func(t *testing.T) {
			work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 10000, MaxArchiveTotalBytes: 10000})
			opts := []filesys.ZipFSOption{}
			if !legacy {
				opts = append(opts, filesys.WithArchiveOptions(filesys.ArchiveOptions{Work: work, TargetRelease: 21}))
			}
			zip, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)), opts...)
			if err != nil {
				t.Fatal(err)
			}
			z := NewJarFS(zip)
			p, err := z.boundJarReleaseSelection()
			if err != nil {
				t.Fatal(err)
			}
			if legacy {
				if p.legacyWork == nil {
					t.Fatal("legacy selection has no request Work")
				}
				work = p.legacyWork
			} else if p.legacyWork != nil {
				t.Fatal("shared original Work was replaced")
			}
			before := work.Used(workbudget.CounterGraphScans)
			if err := p.work(7); err != nil {
				t.Fatal(err)
			}
			if got := work.Used(workbudget.CounterGraphScans) - before; got != 7 {
				t.Fatalf("same Work charged %d for seven operations", got)
			}
			if multi, err := z.RuntimeMultiRelease(); err != nil || !multi {
				t.Fatalf("metadata=%v %v", multi, err)
			}
			if got := work.Used(workbudget.CounterArchiveTotalBytes); got != int64(len(manifest)) {
				t.Fatalf("manifest bytes=%d", got)
			}
			if _, err := z.RuntimeMultiRelease(); err != nil {
				t.Fatal(err)
			}
			if got := work.Used(workbudget.CounterArchiveTotalBytes); got != int64(len(manifest)) {
				t.Fatalf("cached metadata reread %d bytes", got)
			}
			view := z.sourceReleaseView(9)
			if view == nil || view.jarReleaseSelection != p {
				t.Fatal("physical source namespace received another allowance")
			}
			if got, err := view.readJarEntry("payload"); err != nil || string(got) != "NEXT" {
				t.Fatalf("source selection %q %v", got, err)
			}
			if got := work.Used(workbudget.CounterArchiveTotalBytes); got != int64(len(manifest)+4) {
				t.Fatalf("physical read escaped shared total: %d", got)
			}
		})
	}
}

func TestAdversarialJarReleaseSelectionWarmCachesRespectCancellation(t *testing.T) {
	base, variant := jarReleaseChoiceBytes(t)
	raw := jarReleaseTestZIP(t, []jarReleaseTestEntry{{"META-INF/MANIFEST.MF", []byte("Manifest-Version: 1.0\nMulti-Release: true\n\n")}, {"mrprobe/Choice.class", base}, {"META-INF/versions/9/mrprobe/Choice.class", variant}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	work := workbudget.New(context.Background(), workbudget.Limits{})
	zip, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)), filesys.WithArchiveOptions(filesys.ArchiveOptions{Work: work, Context: ctx, TargetRelease: 21}))
	if err != nil {
		t.Fatal(err)
	}
	z, expanded := NewJarFS(zip), NewExpandedZipFS(zip, zip)
	if multi, err := z.RuntimeMultiRelease(); err != nil || !multi {
		t.Fatalf("metadata warmup %v %v", multi, err)
	}
	if !z.originalMemberIndex().valid {
		t.Fatal("original index warmup failed")
	}
	if _, err := expanded.ReadFile("mrprobe/Choice.class"); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := z.RuntimeMultiRelease(); err == nil {
		t.Fatal("cached flag bypassed cancellation")
	}
	if _, err := z.readJarEntry("mrprobe/Choice.class"); err == nil {
		t.Fatal("selected read bypassed cancellation")
	}
	if _, err := expanded.Open("mrprobe/Choice.class"); err == nil {
		t.Fatal("expanded cached selection bypassed cancellation")
	}
	if _, ok := z.enumSiblingResolver()("mrprobe/Choice"); ok {
		t.Fatal("resolver used canceled request")
	}
	if z.originalMemberIndex().valid {
		t.Fatal("cached original index certified canceled request")
	}
}

func TestAdversarialExpandedLocalOpenFileKeepsCallerFlags(t *testing.T) {
	e := NewExpandedZipFS(filesys.NewRelLocalFs(t.TempDir()), nil)
	f, err := e.OpenFile("created.txt", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	w, ok := f.(io.Writer)
	if !ok {
		t.Fatal("underlying writable file replaced")
	}
	if _, err := w.Write([]byte("local")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if f, err := e.OpenFile("created.txt", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600); err == nil {
		f.Close()
		t.Fatal("O_EXCL was discarded")
	}
	if got, err := e.ReadFile("created.txt"); err != nil || string(got) != "local" {
		t.Fatalf("local file %q %v", got, err)
	}
}
