package javaclassparser

import (
	"bytes"
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
	"testing"
)

const receiverDispatchFixture = `class DispatchBase{int word;public int change(int n){word=n;return word;}}
class DispatchMiddle extends DispatchBase{long change(long n){return n;}}
final class DispatchLeaf extends DispatchMiddle{}`

func TestAdversarialConstructorReceiverDispatchNeedsExactFinalClassAndUnshadowedAncestry(t *testing.T) {
	files := nativeCompileClasses(t, receiverDispatchFixture)
	for _, variant := range []string{"original", "own final receiver", "private entry", "final entry", "matching bridge", "matching middle method", "matching private method", "matching static method", "nonfinal receiver", "interface receiver", "abstract receiver", "missing receiver", "missing ancestor", "malformed ancestor", "wrong ancestor identity", "ancestry cycle", "unrelated owner", "different root object", "nil declaration", "duplicate unrelated declaration", "invalid physical parameter width", "wrong name encoding", "wrong descriptor encoding", "malformed descriptor", "wrong opcode", "budget", "work", "memory", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(bytes.Clone(files["DispatchLeaf.class"]))
			if err != nil {
				t.Fatal(err)
			}
			base, err := Parse(bytes.Clone(files["DispatchBase.class"]))
			if err != nil {
				t.Fatal(err)
			}
			middle, err := Parse(bytes.Clone(files["DispatchMiddle.class"]))
			if err != nil {
				t.Fatal(err)
			}
			var target *MemberInfo
			for _, m := range base.Methods {
				if n, _ := base.getUtf8(m.NameIndex); n == "change" {
					target = m
				}
			}
			if target == nil {
				t.Fatal("missing exact original method")
			}
			resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
			remaining, opcode := 512, core.OP_INVOKEVIRTUAL
			d := &ClassObjectDumper{obj: root, foldSiblingResolver: resolve, constructorReceiverFinalizerSilent: true}
			member := &values.JavaClassMember{Name: "DispatchBase", Member: "change", Description: "(I)I"}
			switch variant {
			case "own final receiver":
				base.AccessFlags |= 0x10
				d.obj = base
			case "private entry":
				target.AccessFlags = 2
				root.AccessFlags &^= 0x10
			case "final entry":
				target.AccessFlags = 0x11
				root.AccessFlags &^= 0x10
			case "matching bridge", "matching private method", "matching static method":
				name := sourceBridgePoolString(t, root, "change")
				desc := sourceBridgePoolString(t, root, "(I)I")
				flags := uint16(0x1041)
				if variant == "matching private method" {
					flags = 2
				}
				if variant == "matching static method" {
					flags = 9
				}
				root.Methods = append(root.Methods, &MemberInfo{NameIndex: name, DescriptorIndex: desc, AccessFlags: flags})
			case "matching middle method":
				middle.Methods = append(middle.Methods, &MemberInfo{NameIndex: sourceBridgePoolString(t, middle, "change"), DescriptorIndex: sourceBridgePoolString(t, middle, "(I)I"), AccessFlags: 1})
			case "nonfinal receiver":
				root.AccessFlags &^= 0x10
			case "interface receiver":
				root.AccessFlags |= 0x200
			case "abstract receiver":
				root.AccessFlags |= 0x400
			case "missing receiver":
				d.obj = nil
			case "missing ancestor":
				d.foldSiblingResolver = func(string) ([]byte, bool) { return nil, false }
			case "malformed ancestor":
				d.foldSiblingResolver = func(string) ([]byte, bool) { return []byte{0}, true }
			case "wrong ancestor identity":
				d.foldSiblingResolver = func(string) ([]byte, bool) { return bytes.Clone(files["DispatchLeaf.class"]), true }
			case "ancestry cycle":
				root.SuperClass = root.ThisClass
			case "unrelated owner":
				base.ThisClass = uint16(NewConstantPoolWithConstant(&base.ConstantPool).AddNewClassInfo("UnrelatedDispatchBase"))
				member.Name = "UnrelatedDispatchBase"
			case "different root object":
				copy := *base
				copy.AccessFlags |= 0x10
				d.obj = &copy
			case "nil declaration":
				root.Methods = append(root.Methods, nil)
			case "duplicate unrelated declaration":
				root.Methods = append(root.Methods, root.Methods[0])
			case "invalid physical parameter width":
				root.Methods[0].DescriptorIndex = sourceBridgePoolString(t, root, "("+strings.Repeat("J", 128)+")V")
			case "wrong name encoding":
				root.Methods[0].NameIndex = 0
			case "wrong descriptor encoding":
				root.Methods[0].DescriptorIndex = 0
			case "malformed descriptor":
				root.Methods[0].DescriptorIndex = sourceBridgePoolString(t, root, "not a descriptor")
			case "wrong opcode":
				opcode = core.OP_INVOKESPECIAL
			case "budget":
				remaining = 1
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if variant == "matching middle method" {
				d.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == middle.GetClassName() {
						return middle.Bytes(), true
					}
					return resolve(name)
				}
			}
			want := variant == "original" || variant == "own final receiver" || variant == "private entry" || variant == "final entry"
			accepted := d.constructorReceiverMethodDispatchClosed(base, target, member, opcode, &remaining)
			if accepted != want {
				t.Fatalf("closed dispatch=%v remaining=%d want=%v", accepted, remaining, want)
			}
		})
	}
}

// Caller-owned method tables need their own ordered certificate observation.
// Name/super/flags equality cannot replace the absence query after a shadow is
// introduced into the same original root object.
func TestAdversarialConstructorReceiverDispatchRechecksMutableRootMethodTables(t *testing.T) {
	files := nativeCompileClasses(t, receiverDispatchFixture)
	root, _ := Parse(bytes.Clone(files["DispatchLeaf.class"]))
	base, _ := Parse(bytes.Clone(files["DispatchBase.class"]))
	d := NewClassObjectDumper(root)
	d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
	evidence := &constructorProfileEvidence{eligible: true, inBody: true}
	d.constructorProfileEvidence = evidence
	var target *MemberInfo
	for _, m := range base.Methods {
		if n, _ := base.getUtf8(m.NameIndex); n == "change" {
			target = m
		}
	}
	member := &values.JavaClassMember{Name: "DispatchBase", Member: "change", Description: "(I)I"}
	remaining := 512
	if !d.constructorReceiverMethodDispatchClosed(base, target, member, core.OP_INVOKEVIRTUAL, &remaining) || !evidence.eligible || len(evidence.transcript) != 1 || evidence.transcript[0].absentRootMethod != "change" || evidence.transcript[0].methodDescriptor != "(I)I" {
		t.Fatal("mutable original root requires its own ordered absence observation")
	}
	root.Methods = append(root.Methods, &MemberInfo{NameIndex: sourceBridgePoolString(t, root, "change"), DescriptorIndex: sourceBridgePoolString(t, root, "(I)I"), AccessFlags: 1})
	remaining = 512
	if d.constructorReceiverMethodDispatchClosed(base, target, member, core.OP_INVOKEVIRTUAL, &remaining) {
		t.Fatal("same name/super/flags cannot hide a changed dispatch method table")
	}
}

// Inject a changed declaration at an original provider observation, without
// changing the caller name, superclass or flags. Replaying just byte hashes or
// an unordered initial root digest would miss the last runtime's shadow.
func TestAdversarialConstructorFinalReceiverProfileRechecksRootDispatchInObservationOrder(t *testing.T) {
	files := nativeCompileClasses(t, `class DispatchProfileBase{int word;DispatchProfileBase(int n){word=adjust(n);}public int adjust(int n){return n+1;}}final class DispatchProfileLeaf extends DispatchProfileBase{DispatchProfileLeaf(int n){super(n);}}`)
	for _, at := range []int{0, 3, 4, 11, 12} {
		t.Run(strconv.Itoa(at), func(t *testing.T) {
			root, err := Parse(bytes.Clone(files["DispatchProfileLeaf.class"]))
			if err != nil {
				t.Fatal(err)
			}
			reads := 0
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 8
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == "DispatchProfileBase" {
					reads++
					if reads == at {
						root.Methods = append(root.Methods, &MemberInfo{NameIndex: sourceBridgePoolString(t, root, "adjust"), DescriptorIndex: sourceBridgePoolString(t, root, "(I)I"), AccessFlags: 1})
					}
				}
				raw, known := files[name+".class"]
				return bytes.Clone(raw), known
			}
			safe := d.constructorCaptureChainDoesNotObserve("DispatchProfileBase", "(I)V", map[string]bool{})
			if safe != (at == 0) {
				t.Fatalf("safe=%v reads=%d shadow_at=%d", safe, reads, at)
			}
			if at == 0 && reads != 12 || at != 0 && reads < at {
				t.Fatalf("did not reach original provider observation: reads=%d shadow_at=%d", reads, at)
			}
		})
	}
}
