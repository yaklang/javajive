package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeAnonymousBranchLayoutNeedsClosedOrdinalIntervals(t *testing.T) {
	marker := func(n string) string { return "/*jdec-owned-anonymous-ordinal:" + n + ":Owner*/new Parent(){}" }
	cases := []struct {
		name, left, right string
		swap, failed      bool
	}{
		{"reverse", marker("3"), marker("1") + marker("2"), true, false},
		{"already ordered", marker("1"), marker("2"), false, false},
		{"overlap", marker("2") + marker("3"), marker("1") + marker("2"), false, false},
		{"interleaved", marker("1") + marker("3"), marker("2"), false, false},
		{"duplicate", marker("2") + marker("2"), marker("1"), false, false},
		{"non-increasing", marker("3") + marker("2"), marker("1"), false, false},
		{"one arm", marker("2"), "return 7;", false, false},
		{"empty arm", "", marker("1"), false, false},
		{"unowned ordinal", marker("99"), marker("1"), false, true},
		{"zero ordinal", marker("0"), marker("1"), false, true},
		{"malformed ordinal", marker("oops"), marker("1"), false, true},
		{"malformed literal", marker("2") + "\"unterminated", marker("1"), false, true},
		{"literal spoof", `String s="/*jdec-owned-anonymous-ordinal:1:Owner*/";` + marker("2"), marker("1"), true, false},
		{"line comment spoof", "// " + marker("1") + "\n" + marker("2"), marker("1"), true, false},
		{"other scope", strings.ReplaceAll(marker("1"), ":Owner*/", ":Other*/") + marker("2"), marker("1"), true, false},
		{"no proof", "return 2;", "return 1;", false, false},
		{"wrong child identity", marker("2"), marker("1"), false, true},
		{"budget", marker("2"), marker("1"), false, true},
		{"canceled", marker("2"), marker("1"), false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &nativeAnonymousFamily{owner: "Owner", children: map[string]*nativeAnonymousClass{"Owner$1": {ordinal: 1}, "Owner$2": {ordinal: 2}, "Owner$3": {ordinal: 3}}}
			var work *workbudget.Budget
			if tc.name == "wrong child identity" {
				p.children["Owner$2"].ordinal = 3
			}
			if tc.name == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			}
			if tc.name == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousBranchSwap(p, tc.left, tc.right, work); got != tc.swap || p.failed != tc.failed {
				t.Fatalf("swap=%v failed=%v", got, p.failed)
			}
		})
	}
}
