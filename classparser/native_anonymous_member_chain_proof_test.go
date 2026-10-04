package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousNamedCaptureArchiveReadRequiresCommittedForest(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousMixedChainFixture)
	for _, variant := range []string{"original", "no forest", "wrong forest", "missing unit", "missing group", "wrong group", "failed group", "missing owner", "static owner", "missing read", "wrong read owner", "wrong read field", "wrong descriptor", "not lexical THIS", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["NestedOwner.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original committed mixed forest")
			}
			const user = "NestedOwner$Layer$Middle$1"
			const owner = "NestedOwner$Layer"
			f := p.anonymousForest
			child := f.units[user]
			g := p.anonymousUnits[user]
			var read *nativeMemberLexicalRead
			for _, r := range f.reads[user]["origin()Ljava/lang/Object;"] {
				if r.owner == owner {
					read = r
				}
			}
			if read == nil {
				t.Fatal("original final root read")
			}
			var work *workbudget.Budget
			switch variant {
			case "no forest":
				p.anonymousForest = nil
			case "wrong forest":
				g.forest = &nativeAnonymousForest{}
			case "missing unit":
				delete(f.units, user)
			case "missing group":
				delete(p.anonymousUnits, user)
			case "wrong group":
				f.groups[g.owner] = &nativeAnonymousFamily{}
			case "failed group":
				g.failed = true
			case "missing owner":
				delete(p.children, owner)
			case "static owner":
				p.children[owner].static = true
			case "missing read":
				delete(f.reads[user]["origin()Ljava/lang/Object;"], read.pc)
			case "wrong read owner":
				read.owner = "Foreign"
			case "wrong read field":
				read.field = "foreign"
			case "wrong descriptor":
				read.descriptor = "Ljava/lang/Object;"
			case "not lexical THIS":
				delete(f.lexicalThis[user]["origin()Ljava/lang/Object;"], read.pc)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberProjectedAnonymousCaptureRead(p, child, owner, work); got != (variant == "original") {
				t.Fatalf("archive read admitted=%v", got)
			}
		})
	}
}
