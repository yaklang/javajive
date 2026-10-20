package javaclassparser

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialIndependentSourceTransactionRequiresExactOriginalProtocol(t *testing.T) {
	files := independentTransactionFixture(t)
	for _, variant := range []string{"original", "nil root", "nil policy", "native off", "duplicate", "singleton", "foreign root", "foreign participant", "too many", "normal conflict", "independent conflict", "independent flat", "cache limit", "scan limit", "memory limit", "canceled", "retained source", "shared source"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["CyclicA$Capsule.class"])
			if err != nil {
				t.Fatal(err)
			}
			snap := snapshotJDECEnv()
			policy, _ := json.Marshal(snap)
			names := []string{"CyclicB$Capsule", "CyclicA$Capsule"}
			ctx := context.Background()
			limits := workbudget.Limits{}
			var conflict *nativeMemberCacheEntry
			foreignTransaction := &nativeMemberTransaction{}
			switch variant {
			case "nil root":
				root = nil
			case "nil policy":
				snap = nil
			case "native off":
				snap["JDEC_NATIVE_MEMBER_OFF"] = "1"
			case "duplicate":
				names = []string{"CyclicA$Capsule", "CyclicA$Capsule"}
			case "singleton":
				names = names[1:]
			case "foreign root":
				root, _ = Parse(files["CyclicA.class"])
			case "foreign participant":
				names[0] = "CyclicB"
			case "too many":
				for len(names) <= 64 {
					names = append(names, "Extra"+strconv.Itoa(len(names)))
				}
			case "normal conflict", "independent conflict", "independent flat":
				prefix := ""
				if variant != "normal conflict" {
					prefix = "independent-static\x00"
				}
				conflict = z.nativeMemberPolicyEntry(prefix + "CyclicB$Capsule\x00" + string(policy))
				if variant == "independent flat" {
					conflict.flatSourceCommitted = true
				} else {
					conflict.transaction = foreignTransaction
				}
			case "cache limit":
				z.nativeMembersCache = map[string]*nativeMemberCacheEntry{}
				for i := 0; i < 512; i++ {
					z.nativeMembersCache[strconv.Itoa(i)] = &nativeMemberCacheEntry{}
				}
			case "scan limit":
				limits.MaxGraphScans = 1
			case "memory limit":
				limits.MaxOutputBytes = 1
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "retained source":
				z.nativeMembersBytes = (16 << 20) - 1
			case "shared source":
				z.sourceOwnership.bytes = (16 << 20) - 1
			}
			z.archive = &archiveFSState{ctx: ctx, budget: filesys.WrapWorkBudget(workbudget.New(ctx, limits), filesys.ArchiveLimits{})}
			memberBytes, sharedBytes := z.nativeMembersBytes, z.sourceOwnership.bytes
			entry := z.nativeIndependentTransactionEntry(root, names, snap)
			if variant == "original" {
				if entry == nil || entry.family == nil || len(z.nativeMemberTransactions) != 1 {
					t.Fatal("original complete component refused")
				}
				var bytes int64
				for _, tx := range z.nativeMemberTransactions {
					if len(tx.entries) != 2 {
						t.Fatal("incomplete component")
					}
					for _, result := range tx.entries {
						bytes += int64(len(result.source))
					}
				}
				if z.nativeMembersBytes != bytes || z.sourceOwnership.bytes != bytes {
					t.Fatal("atomic source reservation mismatch")
				}
				return
			}
			if entry != nil || z.nativeMembersBytes != memberBytes || z.sourceOwnership.bytes != sharedBytes {
				t.Fatal("unproved protocol published or reserved source")
			}
			if conflict != nil && variant != "independent flat" && conflict.transaction != foreignTransaction {
				t.Fatal("existing transaction overwritten")
			}
			for _, tx := range z.nativeMemberTransactions {
				if len(tx.entries) != 0 {
					t.Fatal("partial component escaped")
				}
			}
		})
	}
}

func TestAdversarialIndependentSourceTransactionRetainsPolicyAndOneReservation(t *testing.T) {
	files := independentTransactionFixture(t)
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["CyclicA$Capsule.class"])
	if err != nil {
		t.Fatal(err)
	}
	snap := snapshotJDECEnv()
	names := []string{"CyclicB$Capsule", "CyclicA$Capsule"}
	first := z.nativeIndependentTransactionEntry(root, names, snap)
	if first == nil || first.family == nil {
		t.Fatal("original complete component")
	}
	bytes := z.nativeMembersBytes
	for _, tx := range z.nativeMemberTransactions {
		snap["JDEC_NATIVE_MEMBER_OFF"] = "1"
		if tx.snapshot["JDEC_NATIVE_MEMBER_OFF"] != "" {
			t.Fatal("retained policy aliases caller map")
		}
		delete(snap, "JDEC_NATIVE_MEMBER_OFF")
	}
	names[0], names[1] = names[1], names[0]
	if again := z.nativeIndependentTransactionEntry(root, names, snap); again != first || len(z.nativeMemberTransactions) != 1 || z.nativeMembersBytes != bytes || z.sourceOwnership.bytes != bytes {
		t.Fatal("read order duplicated transaction/reservation")
	}
}
