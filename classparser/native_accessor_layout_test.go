package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
	"testing"
)

func TestNativeAccessorDeclarationLayoutPreservesEnclosingOrder(t *testing.T) {
	files := nativeCompileClasses(t, nativeAccessorAnonymousConstructorMarkerFixture())
	for _, variant := range []string{"original", "already ordered", "quoted fake event", "line-comment fake event", "unknown event", "missing constructor event", "fixed declaration cycle", "broken quote", "broken brace", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["AccessScopeOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original member family")
			}
			getter := "/*jdec-owned-getter:0:AccessScopeOwner:token*/"
			constructor := "/*" + nativeConstructorRegistrationPrefix + "AccessScopeOwner$Reader:()V*/"
			fixed := []string{"Object first=mark(\"first\");", "Object before(){return null;}", "Object create(){return null;" + constructor + "}", "Object second=mark(\"second\");", "Object after(){return null;" + getter + "}"}
			members := []string{"\nstatic class Reader{Object get(){return null;" + getter + "}}\n"}
			var work *workbudget.Budget
			switch variant {
			case "already ordered":
				fixed[2] = "Object create(){return null;" + getter + constructor + "}"
			case "quoted fake event":
				members[0] = "class Reader{String s=\"" + getter + "\";}"
			case "line-comment fake event":
				members[0] = "class Reader{//" + getter + "\n}"
			case "unknown event":
				members[0] = strings.ReplaceAll(members[0], "AccessScopeOwner:token", "Foreign:token")
			case "missing constructor event":
				fixed[2] = "Object create(){return null;}"
			case "fixed declaration cycle":
				fixed[0] = "Object early(){return null;" + constructor + "}"
				fixed[2] = "Object create(){return null;" + getter + "}"
				members = nil
			case "broken quote":
				members[0] = "class Reader{String s=\"unterminated;}"
			case "broken brace":
				fixed[0] = "Object first=mark(\"first\");}"
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 128})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			source := "class AccessScopeOwner{" + strings.Join(fixed, "") + "}"
			got, known := nativeMemberRegistrationLayout(p, source, members, work)
			want := variant == "original" || variant == "already ordered"
			if known != want {
				t.Fatalf("layout admitted=%v source=%s", known, got)
			}
			if !known {
				return
			}
			previous := -1
			for _, declaration := range fixed {
				at := strings.Index(got, declaration)
				if at <= previous {
					t.Fatalf("enclosing declaration moved or edited: %s", got)
				}
				previous = at
			}
			if strings.Count(got, "static class Reader") != 1 {
				t.Fatal("owned declaration lost or duplicated")
			}
			if variant == "already ordered" && got != source[:len(source)-1]+strings.Join(members, "")+"}" {
				t.Fatal("valid layout changed")
			}
		})
	}
}

func TestNativeAccessorDeclarationBoundariesExcludeInitializerBodies(t *testing.T) {
	for _, source := range []string{
		`@A(value={"}",";"}) Object read(){return null;}Object x=new Object(){Object y(){return null;}};`,
		`int[] values=new int[]{1,2};static{int x=1;}void m(){if(true){return;}}`,
		`Object x=()->{return ";";};Object y=null;`,
		`String x="class Fake{ /*jdec-owned-getter:0:Fake:x*/ }";/*}*/Object m(){return null;}`,
	} {
		declarations, known := nativeMemberLayoutDeclarations(source)
		if !known || strings.Join(declarations, "") != source {
			t.Fatalf("boundaries %v %q", known, declarations)
		}
		if len(declarations) < 2 {
			t.Fatal("initializer swallowed following declaration")
		}
	}
	for _, source := range []string{`Object x="unterminated;`, `Object x=(null;`, `void m(){`, `Object x=null;}`, `Object x=null;/* open`, `int x=1`} {
		if _, known := nativeMemberLayoutDeclarations(source); known {
			t.Fatalf("malformed boundary admitted: %s", source)
		}
	}
}

// An independent exhaustive scheduler for small declaration sets checks the
// bounded solver against all legal interleavings. It does not use the production
// event reader/state, and keeps the two enclosing declarations in their order.
func TestNativeAccessorDeclarationLayoutAgainstExhaustiveOracle(t *testing.T) {
	for sample := 0; sample < 243; sample++ {
		words := [5]int{}
		value := sample
		for i := range words {
			words[i] = value % 3
			value /= 3
		}
		var oracle func(uint8, [3]bool, int) bool
		oracle = func(selected uint8, seen [3]bool, next int) bool {
			if selected == 31 {
				return next == 3
			}
			for node, word := range words {
				bit := uint8(1) << uint(node)
				if selected&bit != 0 || node == 1 && selected&1 == 0 {
					continue
				}
				copy := seen
				n := next
				if !copy[word] {
					if word != n {
						continue
					}
					copy[word] = true
					n++
				}
				if oracle(selected|bit, copy, n) {
					return true
				}
			}
			return false
		}
		expected := oracle(0, [3]bool{}, 0)
		p := &nativeMemberFamily{owner: "ScheduleScope", getters: map[string]*nativeMemberPrivateGetter{}}
		for i := 0; i < 3; i++ {
			field := string(rune('a' + i))
			p.getters[field] = &nativeMemberPrivateGetter{owner: p.owner, field: field, ordinal: i * 100}
		}
		marker := func(word int) string {
			field := string(rune('a' + word))
			return "/*jdec-owned-getter:" + strconv.Itoa(word*100) + ":" + p.owner + ":" + field + "*/"
		}
		fixed0 := "Object first(){return null;" + marker(words[0]) + "}"
		fixed1 := "Object second(){return null;" + marker(words[1]) + "}"
		source := "class " + p.owner + "{" + fixed0 + fixed1 + "}"
		var members []string
		for i := 2; i < 5; i++ {
			members = append(members, "class N"+strconv.Itoa(i)+"{Object get(){return null;"+marker(words[i])+"}}")
		}
		result, known := nativeMemberRegistrationLayout(p, source, members, nil)
		if known != expected {
			t.Fatalf("sample=%v expected=%v got=%v source=%s", words, expected, known, result)
		}
		if known && strings.Index(result, fixed0) > strings.Index(result, fixed1) {
			t.Fatal("enclosing methods reordered")
		}
	}
}
