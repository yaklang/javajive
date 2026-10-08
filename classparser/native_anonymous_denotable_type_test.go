package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAnonymousDenotabilityRequiresOriginalCompleteLexicalRole(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"EnumForestOwner.java": anonymousThisAliasFixture}, "none", "8")
	for _, variant := range []string{"original", "named owner", "missing unit", "foreign object", "missing object", "missing group", "wrong group owner", "foreign forest", "foreign child", "wrong method", "aliased binary key", "missing original self row", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			root, err := Parse(files["EnumForestOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := archive.nativeMemberReader(root)
			plan := d.planNativeMemberFamily()
			if plan == nil || !d.planNativeMemberAnonymousScopes(plan) || plan.anonymousForest == nil {
				t.Fatal("original forest")
			}
			forest := plan.anonymousForest
			name := "EnumForestOwner$1"
			child := forest.units[name]
			group := forest.groups["EnumForestOwner"]
			if child == nil || group == nil {
				t.Fatal("original role")
			}
			switch variant {
			case "named owner":
				name = "EnumForestOwner"
			case "missing unit":
				delete(forest.units, name)
			case "foreign object":
				copy := *child.object
				child.object = &copy
			case "missing object":
				delete(forest.objects, name)
			case "missing group":
				delete(forest.groups, "EnumForestOwner")
			case "wrong group owner":
				group.owner = "Other"
			case "foreign forest":
				group.forest = &nativeAnonymousForest{}
			case "foreign child":
				copy := *child
				group.children[name] = &copy
			case "wrong method":
				child.method = "other()V"
			case "aliased binary key":
				name = "Other$1"
				forest.units[name], forest.objects[name], group.children[name] = child, child.object, child
			case "missing original self row":
				for _, attr := range child.object.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						table.Classes = nil
					}
				}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				if err := d.Work.Charge(workbudget.CounterGraphScans, 1); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.nativeAnonymousForest = forest
			d.FuncCtx = &class_context.ClassContext{}
			d.wireNativeAnonymousSource()
			if d.FuncCtx.SourceClassDenotable == nil {
				t.Fatal("production type callback missing")
			}
			denotable, known := d.FuncCtx.SourceClassDenotable(name)
			if denotable || known != (variant == "original") {
				t.Fatalf("denotable=%t known=%t", denotable, known)
			}
		})
	}
}
