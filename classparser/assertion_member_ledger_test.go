package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAssertionMemberLedgerRequiresThePublishedOriginalProtocol(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class LedgerAssertion {static int run(int n){assert n>=0:n;return n*3;}}`, "none")
	for _, scenario := range []string{"closed", "no state", "different protocol", "unconsumed", "ordinary flag", "stale status owner", "stale read", "wrong owner", "wrong descriptor", "stub", "degraded", "canceled", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(files["LedgerAssertion.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(obj)
			d.options.Mode, d.options.TargetSourceVersion = Precision, 8
			d.report = &DecompileResult{Members: newMemberLedger(obj)}
			source, err := d.DumpClass()
			if err != nil || d.nativeStandaloneAssertion == nil {
				t.Fatal("authored publication absent", err)
			}
			for i := range d.report.Members {
				row := &d.report.Members[i]
				if row.Name == nativeAssertionField || row.Name == "<clinit>" {
					row.State, row.Evidence = "unsupported", "no publication receipt"
				}
			}
			switch scenario {
			case "no state":
				d.nativeStandaloneAssertion = nil
			case "different protocol":
				d.nativeSourceAssertions = &nativeMemberAssertion{}
			case "unconsumed":
				d.nativeStandaloneAssertion.consumed = nil
			case "ordinary flag":
				obj.Fields[0].AccessFlags &^= 0x1000
			case "stale status owner":
				d.nativeStandaloneAssertion.packet.statusOwner = "unrelated/Owner"
			case "stale read":
				for _, reads := range d.nativeStandaloneAssertion.packet.reads {
					for _, packet := range reads {
						packet.throwPC++
					}
				}
			case "wrong owner", "wrong descriptor", "stub":
				for i := range d.report.Members {
					row := &d.report.Members[i]
					if scenario == "wrong owner" {
						row.Owner = "unrelated/Owner"
					} else if scenario == "wrong descriptor" {
						row.Descriptor = "unrelated"
					} else {
						row.State = "stub"
					}
				}
			case "degraded":
				d.sourceDegraded = true
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "budget":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			}
			d.recordStandaloneAssertionMembers(source)
			for _, row := range d.report.Members {
				if row.Name != nativeAssertionField && row.Name != "<clinit>" {
					continue
				}
				if got := row.State == "regenerated" && row.Evidence != ""; got != (scenario == "closed") {
					t.Fatalf("unproved regeneration: %+v", row)
				}
			}
		})
	}
}
