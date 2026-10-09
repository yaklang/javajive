package javaclassparser

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorClosedLeafContextIndependenceHasBoundedEntryGuards(t *testing.T) {
	files := nativeCompileClasses(t, closedMethodEvidenceFixture+"class ClosedBodyMemoChild extends ClosedMethodEvidence{}")
	c, object := closedBodyMemoDomain(t, files)
	member := &values.JavaClassMember{Name: object.GetClassName(), Member: "change", Description: "(I)I"}
	args := []constructorEffectValue{{kind: 'I', knownInt: true, intWord: 9}}
	invoke := func(depth int, active map[string]bool) (bool, int) {
		remaining := 512
		_, accepted := c.constructorReceiverClosedMethod(object, member, core.OP_INVOKESPECIAL, map[string]bool{}, active, &remaining, depth, &constructorSelfStorageProof{}, args...)
		return accepted, 512 - remaining
	}
	accepted, initial := invoke(0, map[string]bool{})
	if !accepted {
		t.Fatal("original leaf proof")
	}
	for depth := 0; depth <= 16; depth++ {
		for path := 0; path < 64; path++ {
			active := map[string]bool{fmt.Sprintf("other%d\x00(I)I", path): true}
			accepted, work := invoke(depth, active)
			if !accepted || work >= initial {
				t.Fatalf("body without recursion inputs was rewalked at depth=%d path=%d: %v work=%d initial=%d", depth, path, accepted, work, initial)
			}
		}
	}
	if accepted, _ := invoke(17, map[string]bool{}); accepted {
		t.Fatal("leaf hit bypassed the unchanged entry depth bound")
	}
	if accepted, _ := invoke(0, map[string]bool{member.Name + "\x00" + member.Member + "\x00" + member.Description: true}); accepted {
		t.Fatal("leaf hit bypassed the active entry cycle guard")
	}
}

func TestAdversarialConstructorNestedClosedBodiesRetainActiveAndDepthDependencies(t *testing.T) {
	fixture := strings.Replace(closedMethodEvidenceFixture, "class ClosedMethodEvidence {", "class ClosedMethodEvidence {private int dependent(int n){return change(n);}", 1)
	files := nativeCompileClasses(t, fixture+"class ClosedBodyMemoChild extends ClosedMethodEvidence{}")
	for _, variant := range []string{"active callee", "maximum depth"} {
		t.Run(variant, func(t *testing.T) {
			c, object := closedBodyMemoDomain(t, files)
			e := c.constructorProfileEvidence
			member := &values.JavaClassMember{Name: object.GetClassName(), Member: "dependent", Description: "(I)I"}
			invoke := func(depth int, active map[string]bool) bool {
				remaining := 512
				_, accepted := c.constructorReceiverClosedMethod(object, member, core.OP_INVOKESPECIAL, map[string]bool{}, active, &remaining, depth, &constructorSelfStorageProof{}, constructorEffectValue{kind: 'I'})
				return accepted
			}
			if !invoke(0, map[string]bool{}) {
				t.Fatal("original nested proof")
			}
			leaf, dependent := 0, 0
			for key := range e.closedBodies {
				name, _ := object.getUtf8(key.target.NameIndex)
				if name == "change" && key.leaf {
					leaf++
				}
				if name == "dependent" && !key.leaf {
					dependent++
				}
			}
			if leaf != 1 || dependent != 1 {
				t.Fatal("leaf and nested context certificates were conflated", leaf, dependent)
			}
			if variant == "active callee" {
				if invoke(0, map[string]bool{object.GetClassName() + "\x00change\x00(I)I": true}) {
					t.Fatal("outer certificate skipped a newly active nested call")
				}
			} else if invoke(16, map[string]bool{}) {
				t.Fatal("outer certificate hid nested entry depth 17")
			}
		})
	}
}

func TestAdversarialConstructorClosedLeafEntryEpochWrapInvalidatesCertificates(t *testing.T) {
	files := nativeCompileClasses(t, closedMethodEvidenceFixture+"class ClosedBodyMemoChild extends ClosedMethodEvidence{}")
	c, object := closedBodyMemoDomain(t, files)
	e := c.constructorProfileEvidence
	member := &values.JavaClassMember{Name: object.GetClassName(), Member: "change", Description: "(I)I"}
	invoke := func() int {
		remaining := 512
		_, _ = c.constructorReceiverClosedMethod(object, member, core.OP_INVOKESPECIAL, map[string]bool{}, map[string]bool{}, &remaining, 0, &constructorSelfStorageProof{}, constructorEffectValue{kind: 'I'})
		return 512 - remaining
	}
	invoke()
	if len(e.closedBodies) != 1 {
		t.Fatal("original leaf certificate")
	}
	cheap := invoke()
	e.closedCallEpoch = ^uint64(0)
	if work := invoke(); e.eligible || !e.inconsistent || work <= cheap {
		t.Fatal("wrapped entry epoch revived a leaf certificate", work, cheap)
	}
}

func TestAdversarialConstructorMethodFallbackSharesOnlyUninterruptedOwnedBinding(t *testing.T) {
	files := nativeCompileClasses(t, closedMethodEvidenceFixture+"class ClosedBodyMemoChild extends ClosedMethodEvidence{}")
	var costs [2]int
	for variant := 0; variant < 2; variant++ {
		c, object := closedBodyMemoDomain(t, files)
		member := &values.JavaClassMember{Name: object.GetClassName(), Member: "change", Description: "(I)I"}
		args := []constructorEffectValue{{kind: 'I', knownInt: true, intWord: 9}}
		remaining := 512
		aliases := &constructorSelfStorageProof{}
		var value constructorEffectValue
		var accepted bool
		if variant == 0 {
			value, accepted = c.constructorReceiverReadOnlyMethodWithStorage(object, member, core.OP_INVOKESPECIAL, map[string]bool{}, &remaining, aliases, args...)
			if accepted {
				t.Fatal("mutating body unexpectedly readonly")
			}
			value, accepted = c.constructorReceiverClosedMethod(object, member, core.OP_INVOKESPECIAL, map[string]bool{}, map[string]bool{}, &remaining, 0, aliases, args...)
		} else {
			value, accepted = c.constructorReceiverMethodWithStorage(object, member, core.OP_INVOKESPECIAL, map[string]bool{}, map[string]bool{}, &remaining, 0, aliases, args...)
		}
		if !accepted || value.kind != 'I' || aliases.selfStored || aliases.referenceRead {
			t.Fatal("fallback changed body evidence", variant, value, aliases)
		}
		costs[variant] = 512 - remaining
	}
	if costs[1] >= costs[0] {
		t.Fatal("same uninterrupted declaration was bound twice", costs)
	}
}

func TestAdversarialConstructorMethodFallbackRepeatsDispatchAfterReadonlyCallback(t *testing.T) {
	files := nativeCompileClasses(t, `class ReadBindingStorage{int word;}
 class ReadBindingBase extends ReadBindingStorage{public int probe(){return word;}}
 final class ReadBindingLeaf extends ReadBindingBase{}`)
	root, err := Parse(bytes.Clone(files["ReadBindingLeaf.class"]))
	if err != nil {
		t.Fatal(err)
	}
	e := &constructorProfileEvidence{originals: map[string][32]byte{}, eligible: true, inBody: true}
	c := &ClassObjectDumper{obj: root, constructorProfileEvidence: e, constructorReceiverFinalizerSilent: true}
	callbacks := 0
	c.foldSiblingResolver = func(name string) ([]byte, bool) {
		e.providerObserved()
		if name == "ReadBindingStorage" {
			callbacks++
			root.Methods = append(root.Methods, &MemberInfo{NameIndex: sourceBridgePoolString(t, root, "probe"), DescriptorIndex: sourceBridgePoolString(t, root, "()I"), AccessFlags: 0x1041})
		}
		raw, known := files[name+".class"]
		if known {
			e.original(c, name, raw)
		}
		return bytes.Clone(raw), known
	}
	object, known := c.constructorMotionClass("ReadBindingBase")
	if !known {
		t.Fatal("original base")
	}
	remaining := 512
	member := &values.JavaClassMember{Name: "ReadBindingBase", Member: "probe", Description: "()I"}
	// The readonly inherited GETFIELD is refused because it reads moved storage.
	// Its resolver callback also adds a caller-owned shadow before fallback.
	writes := map[string]bool{"ReadBindingStorage\x00word\x00I": true}
	if _, accepted := c.constructorReceiverMethodWithStorage(object, member, core.OP_INVOKEVIRTUAL, writes, map[string]bool{}, &remaining, 0, &constructorSelfStorageProof{}); accepted {
		t.Fatal("stale first dispatch survived a readonly provider callback")
	}
	if callbacks != 1 {
		t.Fatal("callback boundary was not exercised", callbacks)
	}
}
