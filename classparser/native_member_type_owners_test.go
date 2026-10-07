package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
	"testing"
)

func TestNativeLexicalTypeOwnersRequireOriginalSourceOwnership(t *testing.T) {
	files := nativeCompileClasses(t, lexicalReceiverBindingFixture)
	for _, variant := range []string{"original", "missing child", "nil object", "wrong object", "wrong owner", "wrong name", "static cut", "missing root", "missing original member", "failed family", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(append([]byte(nil), files["BindingOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			child := p.children["BindingOwner$Gate"]
			if child == nil {
				t.Fatal("original Gate")
			}
			switch variant {
			case "missing child":
				delete(p.children, "BindingOwner$Gate")
			case "nil object":
				child.object = nil
			case "wrong object":
				child.object = root
			case "wrong owner":
				child.owner = "OtherScope"
			case "wrong name":
				child.name = "OtherGate"
			case "static cut":
				child.static = true
			case "missing root":
				delete(p.lexicalObjects, "BindingOwner")
			case "missing original member":
				child.object.Attributes = nil
			case "failed family":
				p.failed = true
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			path, known := d.nativeMemberTypeOwners(p)("BindingOwner$Gate")
			if known != (variant == "original") || known && !reflect.DeepEqual(path, []string{"BindingOwner", "BindingOwner$Gate"}) {
				t.Fatalf("path=%v known=%v", path, known)
			}
		})
	}
}
