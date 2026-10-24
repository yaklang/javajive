package fieldbinding

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestOriginalFieldSelectionSeparatesDescriptorResolutionAndSourceLookup(t *testing.T) {
	for _, variant := range []string{"original", "inherited unchanged", "different shadow descriptor", "inherited symbolic owner", "duplicate source name", "missing source declaration", "partial source declaration", "wrong source identity", "missing original declaration", "wrong original descriptor", "static original", "inaccessible target", "private foreign field", "private own field", "package field", "cross package field", "protected subclass qualifier", "protected unrelated qualifier", "ambiguous interfaces", "ambiguous symbolic target", "partial ambiguous source", "interface and superclass ambiguity", "cyclic ancestry", "unrelated receiver", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			classes := map[string]Class{
				"java/lang/Object": {Name: "java/lang/Object", Accessible: true, Complete: true},
				"p/Base":           {Name: "p/Base", Super: "java/lang/Object", Accessible: true, Complete: true, Fields: []Field{{"word", "I", 1}}},
				"q/Middle":         {Name: "q/Middle", Super: "p/Base", Accessible: true, Complete: true, Fields: []Field{{"word", "I", 1}}},
				"q/Child":          {Name: "q/Child", Super: "q/Middle", Accessible: true, Complete: true, Fields: []Field{{"word", "I", 1}}},
				"q/Access":         {Name: "q/Access", Super: "java/lang/Object", Accessible: true, Complete: true},
			}
			modify := func(name string, fn func(*Class)) { v := classes[name]; fn(&v); classes[name] = v }
			owner, caller := "p/Base", "q/Access"
			want, wantKnown := "p/Base", true
			var work *workbudget.Budget
			switch variant {
			case "inherited unchanged":
				modify("q/Child", func(c *Class) { c.Fields = nil })
				modify("q/Middle", func(c *Class) { c.Fields = nil })
				want = ""
			case "different shadow descriptor":
				modify("q/Child", func(c *Class) { c.Fields = []Field{{"word", "J", 1}} })
			case "inherited symbolic owner":
				owner = "q/Middle"
				modify(owner, func(c *Class) { c.Fields = []Field{{"word", "J", 1}} })
			case "duplicate source name":
				modify("q/Child", func(c *Class) { c.Fields = append(c.Fields, Field{"word", "J", 1}) })
				want, wantKnown = "", false
			case "missing source declaration":
				delete(classes, "q/Child")
				want, wantKnown = "", false
			case "partial source declaration":
				modify("q/Child", func(c *Class) { c.Complete = false })
				want, wantKnown = "", false
			case "wrong source identity":
				modify("q/Child", func(c *Class) { c.Name = "decoy" })
				want, wantKnown = "", false
			case "missing original declaration":
				delete(classes, "p/Base")
				want, wantKnown = "", false
			case "wrong original descriptor":
				modify("p/Base", func(c *Class) { c.Fields = []Field{{"word", "J", 1}} })
				want, wantKnown = "", false
			case "static original":
				modify("p/Base", func(c *Class) { c.Fields = []Field{{"word", "I", 9}} })
				want, wantKnown = "", false
			case "inaccessible target":
				modify("p/Base", func(c *Class) { c.Accessible = false })
				want, wantKnown = "", false
			case "private foreign field":
				modify("p/Base", func(c *Class) { c.Fields = []Field{{"word", "I", 2}} })
				want, wantKnown = "", false
			case "private own field":
				caller = "p/Base"
				modify("p/Base", func(c *Class) { c.Fields = []Field{{"word", "I", 2}} })
			case "package field":
				caller = "p/Access"
				modify("p/Base", func(c *Class) { c.Fields = []Field{{"word", "I", 0}} })
			case "cross package field":
				modify("p/Base", func(c *Class) { c.Fields = []Field{{"word", "I", 0}} })
				want, wantKnown = "", false
			case "protected subclass qualifier":
				caller = "q/Middle"
				modify("q/Middle", func(c *Class) { c.Fields = nil })
				modify("p/Base", func(c *Class) { c.Fields = []Field{{"word", "I", 4}} })
				want = "q/Middle"
			case "protected unrelated qualifier":
				modify("p/Base", func(c *Class) { c.Fields = []Field{{"word", "I", 4}} })
				want, wantKnown = "", false
			case "ambiguous interfaces", "ambiguous symbolic target", "partial ambiguous source", "interface and superclass ambiguity":
				classes["q/A"] = Class{Name: "q/A", Fields: []Field{{"word", "I", 9}}, Accessible: true, Complete: true}
				classes["q/B"] = Class{Name: "q/B", Fields: []Field{{"word", "J", 9}}, Accessible: true, Complete: true}
				modify("q/Child", func(c *Class) { c.Fields = nil; c.Interfaces = []string{"q/A", "q/B"} })
				// Complete Java lookup ambiguity can be removed by a proved widening
				// view. Missing declarations and an ambiguous JVM target cannot.
				if variant == "ambiguous symbolic target" {
					owner = "q/Child"
					modify("q/B", func(c *Class) { c.Fields[0].Descriptor = "I" })
					want, wantKnown = "", false
				}
				if variant == "partial ambiguous source" {
					delete(classes, "q/B")
					want, wantKnown = "", false
				}
				if variant == "interface and superclass ambiguity" {
					modify("q/Child", func(c *Class) { c.Interfaces = []string{"q/A"} })
				}
			case "cyclic ancestry":
				modify("q/Child", func(c *Class) { c.Fields = nil; c.Super = "q/Child" })
				want, wantKnown = "", false
			case "unrelated receiver":
				modify("q/Child", func(c *Class) { c.Super = "java/lang/Object" })
				want, wantKnown = "", false
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				want, wantKnown = "", false
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
				want, wantKnown = "", false
			}
			provider := func(n string) (Class, bool) { c, ok := classes[n]; return c, ok }
			metadata := func(n string) (callbinding.Class, bool) {
				c, ok := classes[n]
				parents := append([]string(nil), c.Interfaces...)
				if c.Super != "" {
					parents = append(parents, c.Super)
				}
				return callbinding.Class{Name: c.Name, Parents: parents, ParentsComplete: c.Complete}, ok
			}
			widen := func(a, b string) bool { return callbinding.Assignable("L"+a+";", "L"+b+";", metadata) }
			cast, known := Selection(provider, widen, "q/Child", owner, "word", "I", caller, work)
			if cast != want || known != wantKnown {
				t.Fatalf("cast=%q known=%v want=%q %v", cast, known, want, wantKnown)
			}
		})
	}
}
