package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialFlatCompletedComponentRequiresOriginalCompleteRefusal(t *testing.T) {
	for _, variant := range []string{"original", "nil entry", "unknown graph", "acyclic graph", "nil policy", "missing transaction", "nil retained policy", "independent protocol", "unfinished", "successful", "partial", "empty nonnil result", "empty owners", "oversize owners", "missing owner", "different component", "different order", "truncated component", "foreign policy size", "foreign policy value", "scan limit", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			snapshot := map[string]string{"source-policy": "original"}
			tx := &nativeMemberTransaction{completed: true, owners: []string{"A", "B"}, snapshot: map[string]string{"source-policy": "original"}}
			entry := &nativeMemberCacheEntry{transaction: tx, dependenciesKnown: true, dependenciesCyclic: true, dependencyParticipants: []string{"A", "B"}}
			owner := "A"
			var work *workbudget.Budget
			switch variant {
			case "nil entry":
				entry = nil
			case "unknown graph":
				entry.dependenciesKnown = false
			case "acyclic graph":
				entry.dependenciesCyclic = false
			case "nil policy":
				snapshot = nil
			case "missing transaction":
				entry.transaction = nil
			case "nil retained policy":
				tx.snapshot = nil
			case "independent protocol":
				tx.independent = true
			case "unfinished":
				tx.completed = false
			case "successful":
				tx.entries = map[string]*nativeMemberCacheEntry{"A": {family: &nativeMemberFamily{}}, "B": {family: &nativeMemberFamily{}}}
			case "partial":
				tx.entries = map[string]*nativeMemberCacheEntry{"B": {family: &nativeMemberFamily{}}}
			case "empty nonnil result":
				tx.entries = map[string]*nativeMemberCacheEntry{}
			case "empty owners":
				tx.owners = nil
			case "oversize owners":
				for len(tx.owners) <= nativeMemberDependencyComponentLimit {
					tx.owners = append(tx.owners, "C")
				}
			case "missing owner":
				owner = "Outside"
			case "different component":
				tx.owners[1] = "C"
			case "different order":
				tx.owners[0], tx.owners[1] = tx.owners[1], tx.owners[0]
			case "truncated component":
				entry.dependencyParticipants = entry.dependencyParticipants[:1]
			case "foreign policy size":
				tx.snapshot["extra"] = "changed"
			case "foreign policy value":
				tx.snapshot["source-policy"] = "changed"
			case "scan limit":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberCompletedComponentRefusal(entry, owner, snapshot, work); got != (variant == "original") {
				t.Fatalf("completed original component refusal=%v", got)
			}
			if entry != nil && entry.flatSourceCommitted {
				t.Fatal("query published a source namespace")
			}
		})
	}
}
