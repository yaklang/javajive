package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeInvocationRegistrationRequiresOwnedCompilerEvents(t *testing.T) {
	marker := "/*jdec-owned-getter:0:Owner:value*/"
	constructor := "/*jdec-owned-constructor:Owner$Child:()V*/"
	for _, variant := range []string{"getter", "constructor", "nested", "quoted", "ordinary comment", "line comment", "foreign getter", "foreign constructor", "bad ordinal", "unclosed quote", "unclosed comment", "failed family", "nil family", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			p := &nativeMemberFamily{getters: map[string]*nativeMemberPrivateGetter{"g": {owner: "Owner", field: "value", ordinal: 0}}}
			constructors := map[string]bool{"Owner$Child:()V": true}
			source := "(Owner" + marker + ".value)"
			want := "(Owner.value)"
			events := marker
			var work *workbudget.Budget
			valid := true
			switch variant {
			case "constructor":
				source = "(new Child()" + constructor + ")"
				want = "(new Child())"
				events = constructor
			case "nested":
				source = "(Owner" + marker + ".value.link(new Child()" + constructor + "))"
				want = "(Owner.value.link(new Child()))"
				events = marker + constructor
			case "quoted":
				source = "(\"" + marker + "\\\"\"+Owner" + marker + ".value)"
				want = "(\"" + marker + "\\\"\"+Owner.value)"
			case "ordinary comment":
				source = "(Owner/* keep */" + marker + ".value)"
				want = "(Owner/* keep */.value)"
			case "line comment":
				source = "(Owner// " + marker + "\n" + marker + ".value)"
				want = "(Owner// " + marker + "\n.value)"
			case "foreign getter":
				source = strings.Replace(source, "Owner:value", "Foreign:value", 1)
				valid = false
			case "foreign constructor":
				source += strings.Replace(constructor, "Child", "Other", 1)
				valid = false
			case "bad ordinal":
				source = strings.Replace(source, "getter:0:", "getter:00:", 1)
				valid = false
			case "unclosed quote":
				source += "\""
				valid = false
			case "unclosed comment":
				source += "/*"
				valid = false
			case "failed family":
				p.failed = true
				valid = false
			case "nil family":
				p = nil
				valid = false
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				valid = false
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				valid = false
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
				valid = false
			}
			got, registration, known := nativeInvocationReceiverSource(p, source, constructors, work)
			if known != valid {
				t.Fatalf("admitted=%v", known)
			}
			if valid {
				if got != want || registration != events {
					t.Fatalf("receiver=%q events=%q", got, registration)
				}
			} else if got != source || registration != "" {
				t.Fatal("refusal changed source/events")
			}
		})
	}
}
