package javaclassparser

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
)

func independentTransactionFixture(t *testing.T) map[string][]byte {
	t.Helper()
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("original native8 compiler")
	}
	return compileIndependentOpaqueParentFixture(t, compiler, independentSourceCycleFixture, "none")
}
func TestAdversarialIndependentSourceTransactionKeepsAtomicSourceAndPrivateScopes(t *testing.T) {
	files := independentTransactionFixture(t)
	for _, variant := range []string{"original", "missing child", "foreign child", "private method", "flat pinned", "source capacity"} {
		t.Run(variant, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				name := "forward"
				if reverse {
					name = "reverse"
				}
				t.Run(name, func(t *testing.T) {
					input := map[string][]byte{}
					for key, raw := range files {
						input[key] = bytes.Clone(raw)
					}
					switch variant {
					case "missing child":
						delete(input, "CyclicB$Capsule$Worker.class")
					case "foreign child":
						input["CyclicB$Capsule$Worker.class"] = bytes.Clone(input["CyclicA$Capsule$Worker.class"])
					case "private method":
						object, _ := Parse(input["CyclicB$Capsule.class"])
						for _, method := range object.Methods {
							if name, _ := sourceBridgeUTF8(object, method.NameIndex); name == "get" {
								method.AccessFlags |= 2
							}
						}
						input["CyclicB$Capsule.class"] = object.Bytes()
					}
					z := nativeArchive(t, input)
					defer z.Close()
					snap := snapshotJDECEnv()
					policy, _ := json.Marshal(snap)
					if variant == "flat pinned" {
						entry := z.nativeMemberPolicyEntry("CyclicB$Capsule\x00" + string(policy))
						entry.flatSourceCommitted = true
					}
					if variant == "source capacity" {
						z.nativeMembersBytes = 16 << 20
					}
					roots := []string{"CyclicA$Capsule", "CyclicB$Capsule"}
					if reverse {
						roots[0], roots[1] = roots[1], roots[0]
					}
					for _, owner := range roots {
						root, err := Parse(input[owner+".class"])
						if err != nil {
							t.Fatal(err)
						}
						entry := z.nativeMemberIndependentEntry(root)
						if variant == "original" {
							if entry == nil || entry.family == nil || !strings.Contains(entry.source, "class Worker") {
								t.Fatal("complete independent component missing", owner)
							}
							other := roots[0]
							if other == owner {
								other = roots[1]
							}
							if entry.family.lexicalObjects[other] != nil || entry.family.children[other] != nil {
								t.Fatal("peer declaration became private/lexical ownership")
							}
						}
					}
					txa := z.nativeMembersCache["independent-static\x00CyclicA$Capsule\x00"+string(policy)]
					txb := z.nativeMembersCache["independent-static\x00CyclicB$Capsule\x00"+string(policy)]
					if variant == "original" {
						if txa == nil || txb == nil || txa.transaction == nil || txa.transaction != txb.transaction || len(txa.transaction.entries) != 2 {
							t.Fatal("missing common atomic transaction")
						}
					} else {
						for _, entry := range []*nativeMemberCacheEntry{txa, txb} {
							if entry != nil && entry.transaction != nil && len(entry.transaction.entries) != 0 {
								t.Fatal("failed component partially published")
							}
						}
					}
				})
			}
		})
	}
}
func TestAdversarialIndependentSourceTransactionConcurrentReadOrders(t *testing.T) {
	files := independentTransactionFixture(t)
	z := nativeArchive(t, files)
	defer z.Close()
	var wg sync.WaitGroup
	results := make([]*nativeMemberCacheEntry, 16)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := []string{"CyclicA$Capsule.class", "CyclicB$Capsule.class"}[i%2]
			root, err := Parse(bytes.Clone(files[name]))
			if err != nil {
				t.Error(err)
				return
			}
			results[i] = z.nativeMemberIndependentEntry(root)
		}(i)
	}
	wg.Wait()
	for i, result := range results {
		if result == nil || result.family == nil || !strings.Contains(result.source, "class Worker") {
			t.Fatal("concurrent incomplete source", i)
		}
	}
}
