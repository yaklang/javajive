package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberAccessorInitializationRequiresInertOwnedAncestry(t *testing.T) {
	files := nativeCompileClasses(t, `class InitOwner {static class Base{} class Child extends Base {private Object value;Object read(){return value;}}}`)
	for _, variant := range []string{"original", "owner initializer", "ancestor initializer", "unknown ancestor", "interface", "cycle", "missing method", "invalid method name", "failed family", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			p := &nativeMemberFamily{lexicalObjects: map[string]*ClassObject{}}
			for _, raw := range files {
				obj, err := Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				p.lexicalObjects[obj.GetClassName()] = obj
			}
			child, base := p.lexicalObjects["InitOwner$Child"], p.lexicalObjects["InitOwner$Base"]
			var work *workbudget.Budget
			switch variant {
			case "owner initializer", "ancestor initializer":
				obj := child
				if variant == "ancestor initializer" {
					obj = base
				}
				obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "<clinit>"})
				obj.Methods = append(obj.Methods, &MemberInfo{NameIndex: uint16(len(obj.ConstantPool))})
			case "unknown ancestor":
				delete(p.lexicalObjects, "InitOwner$Base")
			case "interface":
				base.Interfaces = []uint16{base.SuperClass}
			case "cycle":
				base.SuperClass = base.ThisClass
			case "missing method":
				base.Methods = append(base.Methods, nil)
			case "invalid method name":
				base.Methods[0].NameIndex = 0
			case "failed family":
				p.failed = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberAccessorInitializationInert(p, "InitOwner$Child", work); got != (variant == "original") {
				t.Fatalf("inert initialization=%v", got)
			}
		})
	}
}

func TestNativeMemberNestmateProjectionRequiresSourcePacketCoverage(t *testing.T) {
	first := &nativeMemberPrivateGetter{owner: "Owner", field: "a", ordinal: 0}
	second := &nativeMemberPrivateGetter{owner: "Owner$Layer", field: "b", ordinal: 100}
	for _, variant := range []string{"original", "reversed", "missing", "quoted only", "foreign", "cloned duplicate"} {
		t.Run(variant, func(t *testing.T) {
			p := &nativeMemberFamily{nestmateAccessors: true, getters: map[string]*nativeMemberPrivateGetter{"a": first, "b": second}}
			a, b := `x/*jdec-owned-getter:0:Owner:a*/.a;`, `y/*jdec-owned-getter:100:Owner$Layer:b*/.b;`
			source := a + b
			switch variant {
			case "reversed":
				source = b + a
			case "missing":
				source = a
			case "quoted only":
				source = a + `"` + b + `";`
			case "foreign":
				source += `/*jdec-owned-getter:200:Foreign:c*/`
			case "cloned duplicate":
				clone := *second
				clone.inheritedField = true
				p.getters["b"] = &clone
				source += b
			}
			if got := nativeMemberPrivateGetterSourceClosed(p, source, nil); got != (variant == "original" || variant == "reversed") {
				t.Fatalf("source coverage=%v", got)
			}
		})
	}
}
