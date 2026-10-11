package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMethodLocalTypeConsumersRequireOriginalScope(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"MethodTypeOwner.java": methodLocalTypeConsumerFixture}, "none", "8")
	for _, scenario := range []string{"original", "no owner", "no method", "wrong scope", "wrong method", "wrong descriptor", "wrong declaration", "wrong ordinal", "instance scope", "explicit packet", "invented capture", "no resolver", "no code", "bad code", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			root, e := Parse(append([]byte(nil), files["MethodTypeOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			local, e := Parse(append([]byte(nil), files["MethodTypeOwner$1Token.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			d := z.nativeMemberReader(root)
			owner, k := originalMethodLocalOwner(local, root, nil)
			if !k {
				t.Fatal("original scope")
			}
			ctor, k := originalMethodLocalSourceConstructor(local, root, nil)
			if !k {
				t.Fatal("original constructor")
			}
			switch scenario {
			case "no owner":
				owner = nil
			case "no method":
				owner.declaration = nil
			case "wrong scope":
				owner.owner = "Foreign"
			case "wrong method":
				owner.method = "unknown"
			case "wrong descriptor":
				owner.descriptor = "()V"
			case "wrong declaration":
				copy := *owner.declaration
				owner.declaration = &copy
			case "wrong ordinal":
				owner.ordinal++
			case "instance scope":
				owner.declaration.AccessFlags &^= 8
			case "explicit packet":
				ctor.descriptor = "(I)V"
			case "invented capture":
				ctor.captures = map[string]int{"val$payload": 0}
			case "no resolver":
				d.foldSiblingResolver = nil
			case "no code":
				owner.declaration.Attributes = nil
			case "bad code":
				for _, a := range owner.declaration.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.Code = []byte{0xbb}
					}
				}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			facts, known := d.nativeMethodLocalTypeConsumerFacts(local, owner, ctor)
			if known != (scenario == "original") {
				t.Fatalf("known=%v facts=%v", known, facts)
			}
			if known && len(facts) != 2 {
				t.Fatalf("actual anonymous NEW and local class LDC: %v", facts)
			}
		})
	}
}
func TestNativeMethodLocalAnonymousSubtypeRequiresCommittedPacket(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"MethodTypeOwner.java": methodLocalTypeConsumerFixture}, "none", "8")
	for _, scenario := range []string{"original", "missing group", "failed group", "failed family", "wrong group owner", "wrong method", "wrong constructor", "wrong source super", "wrong physical super", "wrong super pc", "extra super argument", "missing consumer", "missing lexical owner", "changed constructor body", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			root, e := Parse(append([]byte(nil), files["MethodTypeOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("complete original local/anonymous plans")
			}
			local := p.methodLocals["MethodTypeOwner$1Token"]
			name := "MethodTypeOwner$1"
			group := p.anonymousUnits[name]
			child := group.children[name]
			var work *workbudget.Budget
			switch scenario {
			case "missing group":
				delete(p.anonymousUnits, name)
			case "failed group":
				group.failed = true
			case "failed family":
				p.failed = true
			case "wrong group owner":
				group.owner = "Foreign"
			case "wrong method":
				child.method = "unknown()V"
			case "wrong constructor":
				child.descriptor = "(I)V"
			case "wrong source super":
				child.sourceSuperDescriptor = "(I)V"
			case "wrong physical super":
				child.superDescriptor = "(I)V"
			case "wrong super pc":
				child.superPC++
			case "extra super argument":
				child.superParams = []int{0}
			case "missing consumer":
				local.typeConsumers = nil
			case "missing lexical owner":
				delete(p.lexicalObjects, local.owner.owner)
			case "changed constructor body":
				for _, m := range child.object.Methods {
					n, _ := sourceBridgeUTF8(child.object, m.NameIndex)
					if n == "<init>" {
						for _, a := range m.Attributes {
							if code, ok := a.(*CodeAttribute); ok {
								code.Code = []byte{0xb1}
							}
						}
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			got := nativeMethodLocalAnonymousSubtype(local, p, name, work)
			if got != (scenario == "original") {
				t.Fatalf("committed subtype=%v", got)
			}
		})
	}
}
