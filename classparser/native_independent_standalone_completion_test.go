package javaclassparser

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
)

func independentStandaloneCompletionInput(t *testing.T) map[string][]byte {
	t.Helper()
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("original native8 compiler")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, independentStandaloneCompletionFixture, "GenericBoundary", "none", true)
	outer, err := Parse(files["GenericBoundary.class"])
	if err != nil {
		t.Fatal(err)
	}
	outer.MajorVersion = 55
	files["GenericBoundary.class"] = outer.Bytes()
	delete(files, "GenericBoundary$MetadataOnly.class")
	return files
}

func TestAdversarialIndependentStandaloneCompletionPublishesOnceAcrossAliases(t *testing.T) {
	files := independentStandaloneCompletionInput(t)
	z := nativeArchive(t, files)
	defer z.Close()
	var wg sync.WaitGroup
	results := make([]*nativeMemberCacheEntry, 16)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := []string{"GenericBoundary$Loaded.class", "GenericBoundary$Loaded$AbstractBase.class", "GenericBoundary$Loaded$AbstractBase$ForMissing.class"}[i%3]
			object, err := Parse(bytes.Clone(files[name]))
			if err != nil {
				t.Error(err)
				return
			}
			results[i] = z.nativeMemberIndependentEntry(object)
		}(i)
	}
	wg.Wait()
	for _, result := range results {
		if result == nil || result != results[0] || result.family == nil || !strings.Contains(result.source, "class ForMissing") || result.family.lexicalObjects["GenericBoundary"] != nil || result.family.lexicalObjects["GenericPeer"] != nil {
			t.Fatal("incomplete, repeated or foreign-owned standalone source")
		}
	}
	size := int64(len(results[0].source))
	if z.nativeMembersBytes != size || z.sourceOwnership.bytes != size || len(z.nativeMemberTransactions) != 0 {
		t.Fatal("standalone source was not reserved exactly once", z.nativeMembersBytes, z.sourceOwnership.bytes)
	}
	policy, _ := json.Marshal(snapshotJDECEnv())
	entry := z.nativeMembersCache["independent-static\x00GenericBoundary$Loaded\x00"+string(policy)]
	if entry == nil || !entry.flatSourceCommitted || entry.standalone != results[0] || entry.transaction != nil {
		t.Fatal("completed standalone partition was not pinned")
	}
}

func TestAdversarialIndependentStandaloneCompletionRetainsNamespaceAndResourceGuards(t *testing.T) {
	files := independentStandaloneCompletionInput(t)
	for _, variant := range []string{"original", "missing owned child", "ordinary flat pin", "ordinary transaction", "independent flat pin", "independent transaction", "local source capacity", "shared source capacity", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			input := map[string][]byte{}
			for name, raw := range files {
				input[name] = bytes.Clone(raw)
			}
			if variant == "missing owned child" {
				delete(input, "GenericBoundary$Loaded$AbstractBase.class")
			}
			z := nativeArchive(t, input)
			defer z.Close()
			policy, _ := json.Marshal(snapshotJDECEnv())
			ordinary := z.nativeMemberPolicyEntry("GenericBoundary$Loaded\x00" + string(policy))
			independent := z.nativeMemberPolicyEntry("independent-static\x00GenericBoundary$Loaded\x00" + string(policy))
			switch variant {
			case "ordinary flat pin":
				ordinary.flatSourceCommitted = true
			case "ordinary transaction":
				ordinary.transaction = &nativeMemberTransaction{}
			case "independent flat pin":
				independent.flatSourceCommitted = true
			case "independent transaction":
				independent.transaction = &nativeMemberTransaction{independent: true}
			case "local source capacity":
				z.nativeMembersBytes = 16 << 20
			case "shared source capacity":
				z.sourceOwnership.bytes = 16 << 20
			case "memory", "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				limits := workbudget.Limits{}
				if variant == "canceled" {
					cancel()
				} else {
					limits.MaxOutputBytes = 1
				}
				z.archive = &archiveFSState{ctx: ctx, budget: filesys.WrapWorkBudget(workbudget.New(ctx, limits), filesys.ArchiveLimits{})}
			}
			beforeLocal, beforeShared := z.nativeMembersBytes, z.sourceOwnership.bytes
			root, err := Parse(input["GenericBoundary$Loaded.class"])
			if err != nil {
				t.Fatal(err)
			}
			result := z.nativeMemberIndependentEntry(root)
			if variant == "original" {
				if result == nil || result.family == nil || !strings.Contains(result.source, "class ForMissing") {
					t.Fatal("original standalone source refused")
				}
			} else if result != nil && result.family != nil || independent.standalone != nil || z.nativeMembersBytes != beforeLocal || z.sourceOwnership.bytes != beforeShared {
				t.Fatal("failed standalone attempt published or reserved source")
			}
		})
	}
}
