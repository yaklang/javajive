package javaclassparser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Transaction discovery cannot make an unrepresentable owned child valid.
// Count actual discovery work rather than timing: a failed local
// ownership plan must never explore its external declaration dependency graph,
// including repeated source-name queries and concurrent readers.
func TestNativeSourceTransactionRequiresOriginalLocalAdmissionBeforeDiscovery(t *testing.T) {
	const fixture = `class AdmissionOwner{static class Child{}ExternalFamily.Value foreign;}class ExternalFamily{static class Value{}}`
	files := nativeCompileClasses(t, fixture)
	child, err := Parse(files["AdmissionOwner$Child.class"])
	if err != nil {
		t.Fatal(err)
	}
	child.MajorVersion = 53 // independently unrepresentable by the original member profile
	files["AdmissionOwner$Child.class"] = child.Bytes()
	for _, concurrent := range []bool{false, true} {
		t.Run(map[bool]string{false: "sequential", true: "concurrent"}[concurrent], func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			work := workbudget.New(context.Background(), workbudget.Limits{})
			z.archive = &archiveFSState{ctx: context.Background(), budget: filesys.WrapWorkBudget(work, filesys.ArchiveLimits{})}
			root, err := Parse(files["AdmissionOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			// Prime the ordinary failed plan, as the production source path does.
			entry := z.nativeMemberEntry(root)
			if entry == nil || entry.family != nil {
				t.Fatal("original local rejection missing")
			}
			before := work.Used(workbudget.CounterGraphScans)
			results := make([]*nativeMemberCacheEntry, 12)
			if concurrent {
				var readers sync.WaitGroup
				for i := range results {
					readers.Add(1)
					go func(i int) { defer readers.Done(); results[i] = z.nativeMemberTransactionEntry(root) }(i)
				}
				readers.Wait()
			} else {
				for i := range results {
					results[i] = z.nativeMemberTransactionEntry(root)
				}
			}
			for _, result := range results {
				if result != nil {
					t.Fatal("failed original plan published a transaction")
				}
			}
			if after := work.Used(workbudget.CounterGraphScans); after != before {
				t.Fatalf("failed local admission repeated discovery work: %d", after-before)
			}
			if len(z.nativeMemberTransactions) != 0 || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
				t.Fatal("local rejection allocated or published transaction source")
			}
		})
	}
}

// Every local declaration is valid and representable; the complete reachable
// graph exceeds the unchanged 64-family bound. This is a negative discovery
// result, not a failed local plan. Repeated queries must pay constant lookup
// work rather than parse and decompress all the same families again.
func TestNativeSourceTransactionMemoizesBoundedNegativeDependencyDiscovery(t *testing.T) {
	var source strings.Builder
	source.WriteString("class AdmissionRoot{static class Child{}Family0.Item dependency;}")
	for i := 0; i < 65; i++ {
		fmt.Fprintf(&source, "class Family%d{static class Item{", i)
		if i < 64 {
			fmt.Fprintf(&source, "Family%d.Item dependency;", i+1)
		}
		source.WriteString("}}")
	}
	source.WriteString(`class AdmissionDriver{public static void main(String[]args){if(new AdmissionRoot.Child().getClass().getDeclaringClass()!=AdmissionRoot.class)throw new AssertionError("original ownership");System.out.println("original:many:declaration:families");}}`)
	files := nativeCompileClasses(t, source.String())
	_, java := t04Tools(t)
	original := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "AdmissionDriver"); got != "original:many:declaration:families\n" {
		t.Fatalf("independent original JVM=%q", got)
	}
	for _, concurrent := range []bool{false, true} {
		t.Run(map[bool]string{false: "sequential", true: "concurrent"}[concurrent], func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			work := workbudget.New(context.Background(), workbudget.Limits{})
			z.archive = &archiveFSState{ctx: context.Background(), budget: filesys.WrapWorkBudget(work, filesys.ArchiveLimits{})}
			root, err := Parse(files["AdmissionRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			if z.nativeMemberTransactionEntry(root) != nil {
				t.Fatal("unbounded dependency component admitted")
			}
			before := work.Used(workbudget.CounterGraphScans)
			const queries = 12
			results := make([]*nativeMemberCacheEntry, queries)
			if concurrent {
				var readers sync.WaitGroup
				for i := range results {
					readers.Add(1)
					go func(i int) { defer readers.Done(); results[i] = z.nativeMemberTransactionEntry(root) }(i)
				}
				readers.Wait()
			} else {
				for i := range results {
					results[i] = z.nativeMemberTransactionEntry(root)
				}
			}
			for _, result := range results {
				if result != nil {
					t.Fatal("bounded negative result turned into a partial transaction")
				}
			}
			if delta := work.Used(workbudget.CounterGraphScans) - before; delta > queries {
				t.Fatalf("negative discovery repeated nonconstant work: %d for %d queries", delta, queries)
			}
			if len(z.nativeMemberTransactions) != 0 || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
				t.Fatal("negative discovery published source or ownership")
			}
		})
	}
}
