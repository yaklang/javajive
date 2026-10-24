package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeDeferredLambdaAccessorCommentsKeepTokensAndCompilerPhases(t *testing.T) {
	const direct = "/*jdec-owned-getter:0:Owner:value*/"
	const later = "/*jdec-owned-getter:100:Owner:second*/"
	const earlier = "/*jdec-owned-getter:200:Owner:first*/"
	first := "(int x) -> {Owner.first(x)" + earlier + ";}"
	second := "(int y) -> {Owner.second(y)" + later + ";}"
	for _, variant := range []string{"original", "quoted ordinary marker", "wrong emitted order", "missing body", "duplicate body", "quoted body", "nested body", "foreign marker", "unclosed source", "trailing declaration", "failed family", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			p := &nativeMemberFamily{getters: map[string]*nativeMemberPrivateGetter{"direct": {owner: "Owner", field: "value", ordinal: 0}, "first": {owner: "Owner", field: "first", ordinal: 200}, "second": {owner: "Owner", field: "second", ordinal: 100}}}
			source := "class Child{void first(){consume(" + first + ");}void second(){consume(" + second + ");}int direct(){return Owner.value" + direct + ";}}"
			packets := []string{second, first} // actual original emitted method order
			var work *workbudget.Budget
			switch variant {
			case "quoted ordinary marker":
				source = strings.Replace(source, "int direct(){", "String literal=\""+earlier+"\";int direct(){", 1)
			case "missing body":
				packets[1] = strings.Replace(first, "Owner.first", "Owner.changed", 1)
			case "wrong emitted order":
				packets[0], packets[1] = packets[1], packets[0]
			case "duplicate body":
				source = strings.Replace(source, "int direct(){", "void another(){consume("+first+");}int direct(){", 1)
			case "quoted body":
				source = strings.Replace(source, "consume("+first+")", "consume(\""+first+"\")", 1)
			case "nested body":
				packets[0] = "(int y) -> {consume(" + first + ");Owner.second(y)" + later + ";}"
				source = strings.Replace(source, second, packets[0], 1)
			case "foreign marker":
				packets[1] = strings.Replace(first, "Owner:first", "Foreign:first", 1)
				source = strings.Replace(source, first, packets[1], 1)
			case "unclosed source":
				source = source[:len(source)-1]
			case "trailing declaration":
				source += "class Another{}"
			case "failed family":
				p.failed = true
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got, known := nativeDeferredAccessorComments(p, source, packets, nil, work)
			want := variant == "original" || variant == "quoted ordinary marker"
			if known != want {
				t.Fatalf("compiler phase projection=%v want=%v", known, want)
			}
			if !known {
				if got != source {
					t.Fatal("refusal changed executable source or markers")
				}
				return
			}
			without := func(s string) string {
				for _, marker := range []string{direct, later, earlier} {
					s = strings.ReplaceAll(s, marker, "")
				}
				return s
			}
			if without(got) != without(source) || len(got) != len(source) || !nativeMemberPrivateGetterSourceClosed(p, got, nil) || nativeMemberPrivateGetterSourceClosed(p, source, nil) {
				t.Fatal("phase schedule changed runtime tokens or did not retain every original registration")
			}
		})
	}
}
