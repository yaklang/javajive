package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestNativeSourceTransactionAtomicFailureAndReadOrder(t *testing.T) {
	files := nativeCompileClasses(t, nativeCyclicDependencyFixture)
	for _, variant := range []string{"original", "missing member", "malformed member identity", "unrepresentable participant"} {
		for _, order := range [][]string{{"CycleScopeA", "CycleScopeB"}, {"CycleScopeB", "CycleScopeA"}} {
			t.Run(variant+strings.Join(order, ":"), func(t *testing.T) {
				input := map[string][]byte{}
				for name, raw := range files {
					input[name] = append([]byte(nil), raw...)
				}
				switch variant {
				case "missing member":
					delete(input, "CycleScopeB$Value.class")
				case "malformed member identity":
					input["CycleScopeB$Value.class"] = input["CycleScopeA$Value.class"]
				case "unrepresentable participant":
					object, _ := Parse(input["CycleScopeB.class"])
					object.MajorVersion = 53
					input["CycleScopeB.class"] = object.Bytes()
				}
				z := nativeArchive(t, input)
				defer z.Close()
				for _, name := range order {
					root, _ := Parse(input[name+".class"])
					entry := z.nativeMemberTransactionEntry(root)
					if (entry != nil) != (variant == "original") {
						t.Fatalf("transaction %s result=%v", name, entry != nil)
					}
					if variant == "original" {
						p := entry.family
						foreign := "CycleScopeA$Value"
						if name == "CycleScopeA" {
							foreign = "CycleScopeB$Value"
						}
						if p.children[foreign] != nil || p.lexicalObjects[foreign] != nil || p.sourceDependencies[foreign] == "" {
							t.Fatal("cyclic spelling imported private ownership")
						}
					}
				}
				if variant != "original" {
					if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
						t.Fatal("failed transaction reserved or published source")
					}
					for _, transaction := range z.nativeMemberTransactions {
						if len(transaction.entries) != 0 {
							t.Fatal("partial transaction published")
						}
					}
				}
			})
		}
	}
}

func TestNativeSourceTransactionConcurrentReadersPublishOneCompleteComponent(t *testing.T) {
	files := nativeCompileClasses(t, nativeCyclicDependencyFixture)
	z := nativeArchive(t, files)
	defer z.Close()
	roots := []*ClassObject{}
	for _, name := range []string{"CycleScopeA", "CycleScopeB"} {
		obj, _ := Parse(files[name+".class"])
		roots = append(roots, obj)
	}
	start := make(chan struct{})
	entries := make([]*nativeMemberCacheEntry, 8)
	var readers sync.WaitGroup
	for i := range entries {
		readers.Add(1)
		go func(i int) { defer readers.Done(); <-start; entries[i] = z.nativeMemberTransactionEntry(roots[i%2]) }(i)
	}
	close(start)
	readers.Wait()
	var bytes int64
	for i, entry := range entries {
		if entry == nil || entry.family == nil || entry.family.owner != roots[i%2].GetClassName() {
			t.Fatal("incomplete concurrent family")
		}
		if entry != entries[i%2] {
			t.Fatal("read order duplicated source transaction")
		}
		if i < 2 {
			bytes += int64(len(entry.source))
		}
	}
	if len(z.nativeMemberTransactions) != 1 || z.nativeMembersBytes != bytes || z.sourceOwnership.bytes != bytes {
		t.Fatal("transaction not reserved exactly once")
	}
}

func TestNativeSourceTransactionResourceRefusalsDoNotPublish(t *testing.T) {
	files := nativeCompileClasses(t, nativeCyclicDependencyFixture)
	for _, variant := range []string{"nil object", "nil archive", "budget", "memory", "canceled", "cache limit", "retained source limit", "shared release source limit", "native off"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["CycleScopeA.class"])
			ctx := context.Background()
			limits := workbudget.Limits{}
			switch variant {
			case "nil object":
				root = nil
			case "nil archive":
				var missing *JarFS
				if missing.nativeMemberTransactionEntry(root) != nil {
					t.Fatal("nil archive admitted")
				}
				return
			case "budget":
				limits.MaxGraphScans = 1
			case "memory":
				limits.MaxOutputBytes = 1
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "cache limit":
				z.nativeMembersCache = map[string]*nativeMemberCacheEntry{}
				for i := 0; i < 512; i++ {
					z.nativeMembersCache[strconv.Itoa(i)] = &nativeMemberCacheEntry{}
				}
			case "retained source limit":
				z.nativeMembersBytes = (16 << 20) - 1
			case "shared release source limit":
				z.sourceOwnership.bytes = (16 << 20) - 1
			case "native off":
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			}
			z.archive = &archiveFSState{ctx: ctx, budget: filesys.WrapWorkBudget(workbudget.New(ctx, limits), filesys.ArchiveLimits{})}
			memberBytes, sharedBytes := z.nativeMembersBytes, z.sourceOwnership.bytes
			if z.nativeMemberTransactionEntry(root) != nil {
				t.Fatal("incomplete/limited transaction published")
			}
			if z.nativeMembersBytes != memberBytes || z.sourceOwnership.bytes != sharedBytes {
				t.Fatal("failed transaction changed source allowance")
			}
			for _, transaction := range z.nativeMemberTransactions {
				if len(transaction.entries) != 0 {
					t.Fatal("partial limited transaction published")
				}
			}
		})
	}
}
