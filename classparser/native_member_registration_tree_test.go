package javaclassparser

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func nativeRegistrationTreeTestPlan(t *testing.T, files map[string][]byte) *nativeMemberFamily {
	t.Helper()
	z := nativeArchive(t, files)
	t.Cleanup(func() { z.Close() })
	obj, err := Parse(files["NestedLayoutOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	p := z.nativeMemberReader(obj).planNativeMemberFamily()
	if p == nil || p.failed || len(p.getters) != 3 {
		t.Fatal("original nested family proof")
	}
	return p
}

func nativeRegistrationTreeTestSources(p *nativeMemberFamily, words [5]int) (string, []string, []string) {
	marker := func(i int) string {
		for _, g := range p.getters {
			if g.ordinal == i*100 {
				return fmt.Sprintf("/*jdec-owned-getter:%d:%s:%s*/", g.ordinal, g.owner, g.field)
			}
		}
		panic("missing original ordinal")
	}
	fixed := []string{"Object first=mark(\"L\");", "Object first(){return null;" + marker(words[0]) + "}", "Object second=mark(\"R\");", "Object second(){return null;" + marker(words[1]) + "}"}
	start := "class Start{Object read(){return null;" + marker(words[2]) + "}}"
	reader := "class Reader{Object read(){return null;" + marker(words[4]) + "}}"
	body := "class Scope{Object first=mark(\"F\");Object read(){return null;" + marker(words[3]) + "}Object last=mark(\"S\");}"
	scope := body[:len(body)-1] + reader + "}"
	p.registrationLayouts = map[string]*nativeMemberRegistrationScope{p.owner + "$Scope": {owner: p.owner + "$Scope", source: body, declaration: scope, members: []string{reader}, memberOwners: []string{p.owner + "$Scope$Reader"}}}
	return "class NestedLayoutOwner{" + strings.Join(fixed, "") + "}", []string{start, scope}, fixed
}

// Independently enumerate the root's four whole declarations and both internal
// Scope orders. The oracle knows only integers and legal lexical interleavings;
// it never uses production markers, order state, memoization or recursive search.
func TestNativeAccessorNestedLayoutAgainstExhaustiveOracle(t *testing.T) {
	files := nativeCompileClasses(t, nativeAccessorNestedLayoutFixture)
	p := nativeRegistrationTreeTestPlan(t, files)
	for sample := 0; sample < 243; sample++ {
		words := [5]int{}
		value := sample
		for i := range words {
			words[i], value = value%3, value/3
		}
		expected := false
		var enumerate func([]int, uint8)
		enumerate = func(path []int, selected uint8) {
			if selected != 15 {
				for i := 0; i < 4; i++ {
					if selected&(1<<uint(i)) == 0 && (i != 1 || selected&1 != 0) {
						enumerate(append(path, i), selected|1<<uint(i))
					}
				}
				return
			}
			for _, innerFirst := range []bool{false, true} {
				var sequence []int
				for _, n := range path {
					if n != 3 {
						sequence = append(sequence, words[n])
					} else if innerFirst {
						sequence = append(sequence, words[4], words[3])
					} else {
						sequence = append(sequence, words[3], words[4])
					}
				}
				seen := [3]bool{}
				next := 0
				valid := true
				for _, word := range sequence {
					if !seen[word] {
						if word != next {
							valid = false
							break
						}
						seen[word], next = true, next+1
					}
				}
				expected = expected || valid && next == 3
			}
		}
		enumerate(nil, 0)
		source, members, fixed := nativeRegistrationTreeTestSources(p, words)
		got, known := nativeMemberRegistrationTreeLayout(p, source, members, nil, []string{p.owner + "$Start", p.owner + "$Scope"})
		if known != expected {
			t.Fatalf("sample=%v expected=%v known=%v source=%s", words, expected, known, got)
		}
		if !known {
			continue
		}
		previous := -1
		for _, declaration := range fixed {
			at := strings.Index(got, declaration)
			if at <= previous {
				t.Fatal("fixed enclosing declaration moved")
			}
			previous = at
		}
		if strings.Index(got, "Object first=mark(\"F\");") > strings.Index(got, "Object last=mark(\"S\");") || strings.Count(got, "class Reader{") != 1 {
			t.Fatal("nested field order or ownership changed")
		}
	}
}

func TestNativeAccessorNestedLayoutRejectsUnsealedBoundaries(t *testing.T) {
	files := nativeCompileClasses(t, nativeAccessorNestedLayoutFixture)
	for _, variant := range []string{"valid", "wrong owner", "stale source", "foreign object", "missing child", "fixed cycle", "memory", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			p := nativeRegistrationTreeTestPlan(t, files)
			source, members, _ := nativeRegistrationTreeTestSources(p, [5]int{0, 0, 0, 2, 1})
			layout := p.registrationLayouts[p.owner+"$Scope"]
			names := []string{p.owner + "$Start", p.owner + "$Scope"}
			var work *workbudget.Budget
			switch variant {
			case "wrong owner":
				layout.owner = p.owner + "$Scope$Reader"
			case "stale source":
				layout.source = strings.Replace(layout.source, "mark(\"F\")", "mark(\"altered\")", 1)
			case "foreign object":
				p.children[layout.owner].object = p.children[p.owner+"$Start"].object
			case "missing child":
				delete(p.children, layout.owner)
			case "fixed cycle":
				// Both private symbols remain in the same executable method;
				// no nested declaration move can reverse its events.
				layout.source = strings.Replace(layout.source, "Object read(){return null;", "Object read(){return null;/*jdec-owned-getter:100:NestedLayoutOwner:right*/", 1)
				layout.source = strings.Replace(layout.source, "/*jdec-owned-getter:200:NestedLayoutOwner:last*/", "/*jdec-owned-getter:200:NestedLayoutOwner:last*//*jdec-owned-getter:0:NestedLayoutOwner:left*/", 1)
				layout.members = nil
				layout.memberOwners = nil
				members[1] = layout.source
				layout.declaration = layout.source
				source = "class NestedLayoutOwner{}"
				members = members[1:]
				names = names[1:]
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 128})
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got, known := nativeMemberRegistrationTreeLayout(p, source, members, work, names); known != (variant == "valid") {
				t.Fatalf("admitted=%v source=%s", known, got)
			}
		})
	}
}
