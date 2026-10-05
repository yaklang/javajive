package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeMemberEnclosingWideningNeedsClosedOriginalClasses(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberEnclosingWideningFixture)
	for _, kind := range []string{"original", "identity", "two ancestors", "missing source", "missing target", "missing intermediate", "wrong source name", "wrong target name", "interface source", "interface target", "cycle", "foreign unrelated", "too deep", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			p := &nativeMemberFamily{lexicalObjects: map[string]*ClassObject{}}
			for name, raw := range files {
				obj, e := Parse(raw)
				if e != nil {
					t.Fatal(e)
				}
				p.lexicalObjects[strings.TrimSuffix(name, ".class")] = obj
			}
			from, to := "WideningOwner$SubScope", "WideningOwner$BaseScope"
			source, target := p.lexicalObjects[from], p.lexicalObjects[to]
			var work *workbudget.Budget
			switch kind {
			case "identity":
				to = from
			case "two ancestors", "missing intermediate":
				copy := *source
				copy.ConstantPool = append([]ConstantInfo(nil), source.ConstantPool...)
				name := "WideningOwner$Intermediate"
				copy.ThisClass = nativeWideningTestPoolClass(t, &copy, name)
				copy.SuperClass = source.SuperClass
				p.lexicalObjects[name] = &copy
				source.SuperClass = nativeWideningTestPoolClass(t, source, name)
				if kind == "missing intermediate" {
					delete(p.lexicalObjects, name)
				}
			case "missing source":
				delete(p.lexicalObjects, from)
			case "missing target":
				delete(p.lexicalObjects, to)
			case "wrong source name":
				source.ThisClass = nativeWideningTestPoolClass(t, source, "Foreign")
			case "wrong target name":
				target.ThisClass = nativeWideningTestPoolClass(t, target, "Foreign")
			case "interface source":
				source.AccessFlags |= 0x0200
			case "interface target":
				target.AccessFlags |= 0x0200
			case "cycle":
				source.SuperClass = source.ThisClass
			case "foreign unrelated":
				to = "WideningOwner"
			case "too deep":
				previous := source
				for i := 0; i < 65; i++ {
					copy := *target
					copy.ConstantPool = append([]ConstantInfo(nil), target.ConstantPool...)
					name := "WideningOwner$Long" + strings.Repeat("x", i)
					copy.ThisClass = nativeWideningTestPoolClass(t, &copy, name)
					copy.SuperClass = target.ThisClass
					p.lexicalObjects[name] = &copy
					previous.SuperClass = nativeWideningTestPoolClass(t, previous, name)
					previous = &copy
				}
				previous.SuperClass = nativeWideningTestPoolClass(t, previous, to)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := kind == "original" || kind == "identity" || kind == "two ancestors"
			if got := nativeMemberEnclosingClassWidening(from, to, p, work); got != want {
				t.Fatalf("admitted=%v want=%v", got, want)
			}
		})
	}
}

func nativeWideningTestPoolClass(t *testing.T, obj *ClassObject, name string) uint16 {
	t.Helper()
	idx := sourceBridgePoolString(t, obj, name)
	obj.ConstantPool = append(obj.ConstantPool, &ConstantClassInfo{NameIndex: idx})
	return uint16(len(obj.ConstantPool))
}
