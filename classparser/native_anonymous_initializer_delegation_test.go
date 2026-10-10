package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousInitializerDelegationRequiresOneOriginalSuperRoot(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"DelegationOwner.java": `class DelegationOwner{final Object value=new Object(){};DelegationOwner(){this(7);}DelegationOwner(long n){this((int)n);}DelegationOwner(int n){}}`}, "none", "8")
	for _, variant := range []string{"root", "first delegate", "wide delegate", "missing descriptor", "unknown descriptor", "missing root", "duplicate declaration", "missing code", "duplicate code", "static constructor", "bad descriptor", "wrong allocation PC", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			obj, err := Parse(files["DelegationOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := archive.nativeMemberReader(obj)
			plan := d.planNativeAnonymousFamily()
			if plan == nil || plan.children["DelegationOwner$1"] == nil {
				t.Fatal("original ownership certificate")
			}
			child := plan.children["DelegationOwner$1"]
			d.FuncCtx = &class_context.ClassContext{CurrentMethodDesc: "(I)V"}
			var root *MemberInfo
			for _, method := range obj.Methods {
				desc, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
				if desc == "(I)V" {
					root = method
				}
			}
			if root == nil {
				t.Fatal("original root")
			}
			switch variant {
			case "first delegate":
				d.FuncCtx.CurrentMethodDesc = "()V"
			case "wide delegate":
				d.FuncCtx.CurrentMethodDesc = "(J)V"
			case "missing descriptor":
				d.FuncCtx.CurrentMethodDesc = ""
			case "unknown descriptor":
				d.FuncCtx.CurrentMethodDesc = "(D)V"
			case "missing root":
				for i, m := range obj.Methods {
					if m == root {
						obj.Methods = append(obj.Methods[:i], obj.Methods[i+1:]...)
						break
					}
				}
			case "duplicate declaration":
				obj.Methods = append(obj.Methods, root)
			case "missing code":
				root.Attributes = nil
			case "duplicate code":
				root.Attributes = append(root.Attributes, root.Attributes...)
			case "static constructor":
				root.AccessFlags |= StaticFlag
			case "bad descriptor":
				root.DescriptorIndex = root.NameIndex
			case "wrong allocation PC":
				child.invokePC++
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			initializing, closed := d.nativeAnonymousInitializerDelegation(map[string]*nativeAnonymousClass{"DelegationOwner$1": child})
			want := variant == "root" || variant == "first delegate" || variant == "wide delegate"
			if closed != want || initializing != (variant == "root") {
				t.Fatalf("initializing=%v closed=%v", initializing, closed)
			}
		})
	}
}
