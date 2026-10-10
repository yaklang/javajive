package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeConstructorBridgeAbsentJointScopeKeepsWritableEmptySet(t *testing.T) {
	allowed := nativeMemberJointBridgeNameTypes(nil, nil, nil)
	if allowed == nil || len(allowed) != 0 {
		t.Fatal("absent joint scope changed its empty accumulator contract")
	}
	// Standalone anonymous proof can compose its own independently proved edges.
	allowed[7] = true
	if !allowed[7] {
		t.Fatal("empty scope cannot compose a standalone bridge certificate")
	}
}

func TestNativeConstructorBridgeAliasesRequireEveryOriginalOwnerEdge(t *testing.T) {
	files := nativeCompileClasses(t, `class TupleBridgeOwner {static class First{private First(long n){}}static class Second{private Second(long n){}}Object make(boolean second,long n){return second?new Second(n):new First(n);}}`)
	for _, variant := range []string{"original", "unknown owner alias", "missing second owner", "wrong second descriptor", "field alias", "interface alias", "handle first alias", "handle second alias", "dynamic tuple", "invokedynamic tuple", "orphan tuple", "missing child", "budget", "allocation budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["TupleBridgeOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original lexical family unavailable")
			}
			refs := []int{}
			index := 0
			for i, cp := range root.ConstantPool {
				ref, ok := cp.(*ConstantMethodrefInfo)
				if !ok || ref == nil {
					continue
				}
				owner, known := sourceBridgeClassName(root, ref.ClassIndex)
				if !known || owner != "TupleBridgeOwner$First" && owner != "TupleBridgeOwner$Second" {
					continue
				}
				if index != 0 && index != int(ref.NameAndTypeIndex) {
					t.Fatal("fixture did not share a NameAndType")
				}
				index = int(ref.NameAndTypeIndex)
				refs = append(refs, i)
			}
			if len(refs) != 2 || index == 0 {
				t.Fatal("original shared two-owner tuple unavailable")
			}
			second := root.ConstantPool[refs[1]].(*ConstantMethodrefInfo)
			secondOwner, _ := sourceBridgeClassName(root, second.ClassIndex)
			firstOwner, _ := sourceBridgeClassName(root, root.ConstantPool[refs[0]].(*ConstantMethodrefInfo).ClassIndex)
			// The single-owner profile must not borrow another family's permission.
			single := &nativeAnonymousFamily{owner: firstOwner, bridges: p.children[firstOwner].accessBridges}
			if single.accessBridgeNameTypes(root, nil)[index] {
				t.Fatal("standalone scope admitted the foreign alias")
			}
			var work *workbudget.Budget
			checkIndex := index
			switch variant {
			case "unknown owner alias":
				copy := *second
				copy.ClassIndex = root.ThisClass
				root.ConstantPool = append(root.ConstantPool, &copy)
			case "missing second owner":
				delete(p.children, secondOwner)
			case "wrong second descriptor":
				p.children[secondOwner].accessBridges = nil
			case "field alias":
				root.ConstantPool[refs[1]] = &ConstantFieldrefInfo{ConstantMemberrefInfo: second.ConstantMemberrefInfo}
			case "interface alias":
				root.ConstantPool[refs[1]] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: second.ConstantMemberrefInfo}
			case "handle first alias", "handle second alias":
				refIndex := refs[0]
				if variant == "handle second alias" {
					refIndex = refs[1]
				}
				root.ConstantPool = append(root.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: uint16(refIndex + 1)})
			case "dynamic tuple":
				root.ConstantPool = append(root.ConstantPool, &ConstantDynamicInfo{NameAndTypeIndex: uint16(index)})
			case "invokedynamic tuple":
				root.ConstantPool = append(root.ConstantPool, &ConstantInvokeDynamicInfo{NameAndTypeIndex: uint16(index)})
			case "orphan tuple":
				copy := *root.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
				root.ConstantPool = append(root.ConstantPool, &copy)
				checkIndex = len(root.ConstantPool)
			case "missing child":
				p.children[secondOwner] = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberJointBridgeNameTypes(p, root, work)[checkIndex]; got != (variant == "original") {
				t.Fatalf("complete tuple accepted=%t", got)
			}
		})
	}
}
