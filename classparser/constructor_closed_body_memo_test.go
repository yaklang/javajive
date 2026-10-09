package javaclassparser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func closedBodyMemoDomain(t *testing.T, files map[string][]byte, work ...*workbudget.Budget) (*ClassObjectDumper, *ClassObject) {
	t.Helper()
	root, err := Parse(bytes.Clone(files["ClosedBodyMemoChild.class"]))
	if err != nil {
		t.Fatal(err)
	}
	e := &constructorProfileEvidence{originals: map[string][32]byte{}, eligible: true, inBody: true}
	c := &ClassObjectDumper{obj: root, constructorReceiverFinalizerSilent: true, constructorProfileEvidence: e}
	if len(work) > 0 {
		c.Work = work[0]
	}
	c.foldSiblingResolver = func(name string) ([]byte, bool) {
		e.providerObserved()
		raw, known := files[name+".class"]
		if known {
			e.original(c, name, raw)
		}
		return bytes.Clone(raw), known
	}
	obj, known := c.constructorMotionClass("ClosedMethodEvidence")
	if !known || !e.parsedOriginals[obj] {
		t.Fatal("fresh original domain")
	}
	return c, obj
}

func TestAdversarialConstructorClosedBodyMemoRechecksBindingObservationsAndStorage(t *testing.T) {
	files := nativeCompileClasses(t, closedMethodEvidenceFixture+"class ClosedBodyMemoChild extends ClosedMethodEvidence{}")
	for _, variant := range []string{"same", "root query", "provider miss", "metadata miss", "moved storage", "actual integer", "active set", "active cycle", "depth", "caller object", "unregistered object", "canceled", "work", "memory", "epoch wrap"} {
		t.Run(variant, func(t *testing.T) {
			c, obj := closedBodyMemoDomain(t, files)
			e := c.constructorProfileEvidence
			member := &values.JavaClassMember{Name: obj.GetClassName(), Member: "change", Description: "(I)I"}
			args := []constructorEffectValue{{kind: 'I', knownInt: true, intWord: 9}}
			writes, active := map[string]bool{}, map[string]bool{}
			depth := 0
			if variant == "actual integer" {
				member.Member = "branchPublish"
				member.Description = "(Z)I"
				args[0].intWord = 1
			}
			invoke := func() (bool, int) {
				remaining := 512
				_, accepted := c.constructorReceiverClosedMethod(obj, member, core.OP_INVOKESPECIAL, writes, active, &remaining, depth, &constructorSelfStorageProof{}, args...)
				return accepted, 512 - remaining
			}
			accepted, first := invoke()
			if !accepted || len(e.closedBodies) != 1 {
				t.Fatalf("first original proof accepted=%v memo=%d", accepted, len(e.closedBodies))
			}
			cached, reuseCost := invoke()
			if !cached || reuseCost >= first {
				t.Fatalf("unchanged original proof not reused: %v %d >= %d", cached, reuseCost, first)
			}
			beforeProvider := e.providerEpoch
			switch variant {
			case "root query":
				if !e.record(c, constructorProfileObservation{absentRootMethod: "change", methodDescriptor: "(I)I"}) {
					t.Fatal("root query")
				}
				if e.providerEpoch != beforeProvider {
					t.Fatal("pure root query became a callback")
				}
			case "provider miss":
				if _, known := c.foldSiblingResolver("MissingOriginal"); known {
					t.Fatal("unexpected provider")
				}
			case "metadata miss":
				_, _ = e.metadata(func(string) (callbinding.Class, bool) { return callbinding.Class{}, false })("java/lang/Object")
			case "moved storage":
				writes[obj.GetClassName()+"\x00word\x00I"] = true
			case "actual integer":
				args[0].intWord = 0
			case "active cycle":
				active[member.Name+"\x00"+member.Member+"\x00"+member.Description] = true
			case "active set":
				active["other\x00call\x00()V"] = true
			case "depth":
				depth = 1
			case "caller object":
				c.obj = obj
			case "unregistered object":
				delete(e.parsedOriginals, obj)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(ctx, workbudget.Limits{})
			case "work":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "epoch wrap":
				e.providerEpoch = ^uint64(0)
				e.providerObserved()
				if e.eligible || !e.inconsistent {
					t.Fatal("wrapped provider revived certificate")
				}
			}
			accepted, second := invoke()
			want := variant != "active cycle" && variant != "moved storage" && variant != "actual integer" && variant != "canceled" && variant != "work" && variant != "memory"
			if accepted != want {
				t.Fatalf("accepted=%v want=%v first=%d second=%d", accepted, want, first, second)
			}
			if variant == "same" || variant == "root query" {
				if second >= first {
					t.Fatalf("same no-callback body was not reused: %d >= %d", second, first)
				}
			} else if accepted && second <= reuseCost {
				t.Fatalf("changed domain reused proof: %d <= %d", second, reuseCost)
			}
		})
	}
}

func TestAdversarialConstructorClosedBodyMemoFiniteContextKeysRemainDistinct(t *testing.T) {
	obj := &ClassObject{}
	target := &MemberInfo{}
	code := &CodeAttribute{}
	c := &ClassObjectDumper{constructorProfileEvidence: &constructorProfileEvidence{eligible: true, inBody: true, parsedOriginals: map[*ClassObject]bool{obj: true}}}
	arguments := []constructorEffectValue{{kind: 'I'}, {kind: 'I', knownInt: true, intWord: 0}, {kind: 'I', knownInt: true, intWord: 1}, {kind: 'I', knownInt: true, intWord: -1}, {kind: 'L'}, {kind: 'J'}, {kind: 'D'}, {kind: 'F'}}
	sets := []map[string]bool{{}, {"a\x00b": true}, {"a\x00b": false}, {"a": true}}
	seen := map[constructorClosedBodyKey]bool{}
	for depth := 0; depth < 16; depth++ {
		for _, arg := range arguments {
			for _, writes := range sets {
				for _, active := range sets {
					remaining := 512
					key, known := c.constructorClosedBodyKey(obj, target, code, []constructorEffectValue{arg}, writes, active, &remaining, depth)
					if !known || seen[key] {
						t.Fatalf("missing or aliased key depth=%d arg=%+v writes=%v active=%v", depth, arg, writes, active)
					}
					seen[key] = true
				}
			}
		}
	}
	if len(seen) != 2048 {
		t.Fatal("incomplete finite context grammar", len(seen))
	}
}

// A body that reads inherited storage must repeat its original provider reads;
// a failed or changed provider is observable even when the field type agrees.
func TestAdversarialConstructorClosedBodyCallbacksCannotBecomeMemoCertificates(t *testing.T) {
	fixture := strings.Replace(closedMethodEvidenceFixture, "class ClosedMethodEvidence {", "class ClosedMethodEvidence extends MemoStorageAncestor {", 1)
	fixture = strings.Replace(fixture, "int word;Object self;static Object saved;", "int word;Object self;static Object saved;private int borrowed(){borrowedWord=7;return borrowedWord;}", 1)
	files := nativeCompileClasses(t, fixture+"class MemoStorageAncestor{int borrowedWord;}class ClosedBodyMemoChild extends ClosedMethodEvidence{}")
	c, obj := closedBodyMemoDomain(t, files)
	e := c.constructorProfileEvidence
	member := &values.JavaClassMember{Name: obj.GetClassName(), Member: "borrowed", Description: "()I"}
	for i := 0; i < 2; i++ {
		before := e.providerEpoch
		remaining := 512
		_, accepted := c.constructorReceiverClosedMethod(obj, member, core.OP_INVOKESPECIAL, map[string]bool{}, map[string]bool{}, &remaining, 0, &constructorSelfStorageProof{})
		if !accepted || e.providerEpoch == before || len(e.closedBodies) != 0 {
			t.Fatalf("provider body accepted=%v epoch %d->%d memo=%d", accepted, before, e.providerEpoch, len(e.closedBodies))
		}
	}
}

func TestAdversarialConstructorClosedBodyMemoRepeatsMutableRootDispatchQueries(t *testing.T) {
	files := nativeCompileClasses(t, receiverDispatchFixture)
	root, err := Parse(bytes.Clone(files["DispatchLeaf.class"]))
	if err != nil {
		t.Fatal(err)
	}
	e := &constructorProfileEvidence{originals: map[string][32]byte{}, eligible: true, inBody: true}
	c := &ClassObjectDumper{obj: root, constructorProfileEvidence: e, constructorReceiverFinalizerSilent: true}
	c.foldSiblingResolver = func(name string) ([]byte, bool) {
		e.providerObserved()
		raw, known := files[name+".class"]
		if known {
			e.original(c, name, raw)
		}
		return bytes.Clone(raw), known
	}
	// Use the direct original ancestor edge so dispatch itself makes only a pure
	// root-table read. Intervening ancestor providers otherwise disable reuse.
	root.SuperClass = uint16(NewConstantPoolWithConstant(&root.ConstantPool).AddNewClassInfo("DispatchBase"))
	obj, known := c.constructorMotionClass("DispatchBase")
	if !known {
		t.Fatal("original parent")
	}
	member := &values.JavaClassMember{Name: "DispatchBase", Member: "change", Description: "(I)I"}
	for i := 0; i < 2; i++ {
		remaining := 512
		before := e.observationEpoch
		_, accepted := c.constructorReceiverClosedMethod(obj, member, core.OP_INVOKEVIRTUAL, map[string]bool{}, map[string]bool{}, &remaining, 0, &constructorSelfStorageProof{}, constructorEffectValue{kind: 'I'})
		if !accepted || e.observationEpoch == before || len(e.closedBodies) != 1 {
			t.Fatalf("dispatch query skipped or body lost: %v %d", accepted, len(e.closedBodies))
		}
	}
	root.Methods = append(root.Methods, &MemberInfo{NameIndex: sourceBridgePoolString(t, root, "change"), DescriptorIndex: sourceBridgePoolString(t, root, "(I)I"), AccessFlags: 0x1041})
	remaining := 512
	if _, accepted := c.constructorReceiverClosedMethod(obj, member, core.OP_INVOKEVIRTUAL, map[string]bool{}, map[string]bool{}, &remaining, 0, &constructorSelfStorageProof{}, constructorEffectValue{kind: 'I'}); accepted {
		t.Fatal("cached parent body hid a newly shadowing original root method")
	}
}

func TestAdversarialConstructorClosedBodyOriginalRetentionIsOwnedAndCumulative(t *testing.T) {
	files := nativeCompileClasses(t, closedMethodEvidenceFixture+"class ClosedBodyMemoChild extends ClosedMethodEvidence{}")
	c, obj := closedBodyMemoDomain(t, files, workbudget.New(nil, workbudget.Limits{}))
	retained := c.constructorProfileEvidence.parsedRetention
	if retained <= int64(len(files["ClosedMethodEvidence.class"]))*4 {
		t.Fatal("retained parser items were omitted")
	}
	original := bytes.Clone(files["ClosedMethodEvidence.class"])
	// The parser owns a disjoint provider snapshot, including opaque Code bytes.
	for i := range files["ClosedMethodEvidence.class"] {
		files["ClosedMethodEvidence.class"][i] ^= 0xff
	}
	if got := obj.Bytes(); !bytes.Equal(got, original) {
		t.Fatal("provider buffer mutation changed an already owned parsed original")
	}
	files["ClosedMethodEvidence.class"] = original
	for _, variant := range []string{"parsed retention", "body retention"} {
		t.Run(variant, func(t *testing.T) {
			c, _ := closedBodyMemoDomain(t, files)
			if variant == "parsed retention" {
				c.constructorProfileEvidence.parsedRetention = int64(^uint64(0) >> 1)
			} else {
				c.constructorProfileEvidence.bodyRetention = int64(^uint64(0) >> 1)
			}
			if _, known := c.constructorMotionClass("ClosedMethodEvidence"); known {
				t.Fatal("overflow retained a certificate")
			}
		})
	}
	bounded, _ := closedBodyMemoDomain(t, files, workbudget.New(nil, workbudget.Limits{MaxOutputBytes: (retained + 7) / 8}))
	if _, known := bounded.constructorMotionClass("ClosedMethodEvidence"); known || bounded.Work.Check() == nil {
		t.Fatal("second retained original bypassed cumulative allocation bound")
	}
}
