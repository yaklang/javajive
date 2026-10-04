package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeJointAnonymousAccessRequiresOriginalEnclosingCommit(t *testing.T) {
	files := nativeCompileDebugClasses(t, nativeJointPrivateAccessFixture, "none")
	const owner = "JointPrivateOwner"
	const user = owner + "$1"
	const target = owner + "$Item"
	for _, variant := range []string{"original", "absent joint plan", "failed joint plan", "wrong joint owner", "missing child", "nil object", "foreign identity", "wrong enclosing identity", "wrong enclosing method", "missing enclosing", "duplicate enclosing", "named self row", "missing self row", "uncommitted user", "foreign user", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(files[owner+".class"])
			if err != nil {
				t.Fatal(err)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original named declaration proof absent")
			}
			p.anonymous = d.planNativeAnonymousFamily()
			if p.anonymous == nil || p.anonymous.children[user] == nil {
				t.Fatal("original anonymous declaration proof absent")
			}
			child := p.anonymous.children[user]
			var work *workbudget.Budget
			caller := user
			switch variant {
			case "absent joint plan":
				p.anonymous = nil
			case "failed joint plan":
				p.anonymous.failed = true
			case "wrong joint owner":
				p.anonymous.owner += "Other"
			case "missing child":
				delete(p.anonymous.children, user)
			case "nil object":
				child.object = nil
			case "foreign identity":
				child.object = root
			case "wrong enclosing identity":
				for _, attr := range child.object.Attributes {
					if a, ok := attr.(*UnparsedAttribute); ok && a.Name == "EnclosingMethod" {
						a.Info[0], a.Info[1] = byte(child.object.ThisClass>>8), byte(child.object.ThisClass)
					}
				}
			case "wrong enclosing method":
				child.method = "other()V"
			case "missing enclosing", "duplicate enclosing":
				var enclosing AttributeInfo
				kept := []AttributeInfo{}
				for _, attr := range child.object.Attributes {
					if a, ok := attr.(*UnparsedAttribute); ok && a.Name == "EnclosingMethod" {
						enclosing = attr
					} else {
						kept = append(kept, attr)
					}
				}
				if enclosing == nil {
					t.Fatal("no original enclosing attribute")
				}
				if variant == "missing enclosing" {
					child.object.Attributes = kept
				} else {
					child.object.Attributes = append(child.object.Attributes, enclosing)
				}
			case "named self row", "missing self row":
				for _, attr := range child.object.Attributes {
					if a, ok := attr.(*InnerClassesAttribute); ok {
						kept := a.Classes[:0]
						for _, row := range a.Classes {
							if row.InnerClassInfoIndex == child.object.ThisClass {
								if variant == "missing self row" {
									continue
								}
								row.InnerNameIndex = root.Methods[0].NameIndex
							}
							kept = append(kept, row)
						}
						a.Classes = kept
					}
				}
			case "uncommitted user":
				caller = "JointPrivateOwner$Unknown"
			case "foreign user":
				caller = "Other$1"
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				if !nativeProofWork(work, 1) {
					t.Fatal("fixture budget setup")
				}
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			index := &nativeMemberIndex{typeUsers: map[string]map[string]bool{target: {caller: true}}}
			if got := z.nativeMemberAccessRepresentable(p, index, work); got != (variant == "original") {
				t.Fatalf("private access accepted=%v", got)
			}
		})
	}
}
