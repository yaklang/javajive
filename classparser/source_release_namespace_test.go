package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
	"sync"
	"testing"
)

func TestSourceReleaseNamespaceRequiresCanonicalPhysicalIdentity(t *testing.T) {
	for _, name := range []string{"A.class", "A.raw", "META-INF/versions/8/A.class", "META-INF/versions/09/A.class", "META-INF/versions/+9/A.class", "META-INF/versions/9/../A.class", "META-INF/versions/9/A.raw", "META-INF/versions/9//A.class", "META-INF/versions/99999999999999999999/A.class"} {
		if _, _, ok := physicalClassNamespace(name); ok {
			t.Fatalf("unsafe namespace %q", name)
		}
	}
	for _, name := range []string{"META-INF/versions/9/A.class", "META-INF/versions/11/p/Owner$Child.class"} {
		if _, _, ok := physicalClassNamespace(name); !ok {
			t.Fatalf("canonical namespace %q", name)
		}
	}
}

func TestSourceReleaseViewsShareRetainedMemoryAndRespectCancellation(t *testing.T) {
	z := nativeArchive(t, map[string][]byte{"note.txt": []byte("view")})
	var wg sync.WaitGroup
	views := make(chan *JarFS, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); views <- z.sourceReleaseView(9) }()
	}
	wg.Wait()
	close(views)
	selected := z.sourceReleaseView(9)
	for v := range views {
		if v != selected || v.sourceOwnership != z.sourceOwnership {
			t.Fatal("namespace split cache or retained budget")
		}
	}
	for release := 9; release < 25; release++ {
		if z.sourceReleaseView(release) == nil {
			t.Fatalf("early view limit %d", release)
		}
	}
	if z.sourceReleaseView(25) != nil {
		t.Fatal("unbounded namespace retention")
	}
	for i := 0; i < 4; i++ {
		if !z.sourceReleaseView(9 + i).reserveOwnershipSource(4 << 20) {
			t.Fatal("early retained memory limit")
		}
	}
	if z.reserveOwnershipSource(1) || selected.reserveOwnershipSource(1) || z.reserveOwnershipSource(-1) {
		t.Fatal("namespace multiplied retained source limit")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	work := workbudget.New(canceled, workbudget.Limits{})
	z2 := nativeArchive(t, map[string][]byte{"note.txt": []byte("cancel")})
	z2.archive = &archiveFSState{budget: filesys.WrapWorkBudget(work, filesys.ArchiveLimits{}), ctx: canceled}
	if z2.sourceReleaseView(9).reserveOwnershipSource(1) {
		t.Fatal("canceled shared budget accepted")
	}
}
