package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// A NameAndType has no declaring owner. javac can share one tuple between a
// private-constructor dummy parameter and a nested anonymous enclosing parameter.
// Their individual all-edges predicates cannot prove that mixed tuple: each
// rejects the other's independently valid owner. Compose the certificates per
// original member edge, then require complete coverage of the tuple's uses.
// This grants no operand/private-access permission; original allocation and
// delegation packets still validate the actual instructions before publication.
func nativeAnonymousForestSharedConstructorNameType(object *ClassObject, index int, forest *nativeAnonymousForest, work *workbudget.Budget) bool {
	if forest == nil || forest.members == nil || forest.members.failed || forest.members.owner != forest.root || object == nil || forest.objects[object.GetClassName()] != object || index < 1 || index > len(object.ConstantPool) {
		return false
	}
	nt, ok := object.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return false
	}
	name, nk := sourceBridgeUTF8(object, nt.NameIndex)
	desc, dk := sourceBridgeUTF8(object, nt.DescriptorIndex)
	if !nk || !dk || name != "<init>" {
		return false
	}
	// Ordinary anonymous tuples keep their existing proof. Only a descriptor
	// with an independently discovered private bridge can need this composition.
	p := forest.members
	bridgeCandidate := p.rootAccessBridges[desc] != nil
	for _, child := range p.children {
		if !nativeProofWork(work, 1) || child == nil {
			return false
		}
		bridgeCandidate = bridgeCandidate || child.accessBridges[desc] != nil
	}
	if !bridgeCandidate {
		return false
	}
	anonymous, bridge := false, false
	for _, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if member := nativeConstantMember(constant); member != nil && int(member.NameAndTypeIndex) == index {
			ref, ordinary := constant.(*ConstantMethodrefInfo)
			if !ordinary || ref == nil {
				return false
			}
			owner, known := sourceBridgeClassName(object, ref.ClassIndex)
			if !known {
				return false
			}
			if child := forest.units[owner]; child != nil {
				group := forest.groups[object.GetClassName()]
				if child.object == nil {
					return false
				}
				actual, _, owned := originalAnonymousOwner(child.object)
				if forest.objects[owner] != child.object || child.object.GetClassName() != owner || child.descriptor != desc || group == nil || group.failed || group.owner != object.GetClassName() || group.forest != forest || group.children[owner] != child || !owned || actual != object.GetClassName() {
					return false
				}
				anonymous = true
			} else {
				b := p.constructorBridges(owner)[desc]
				target := forest.objects[owner]
				if b == nil || target == nil || p.lexicalObjects[owner] != target {
					return false
				}
				marker := forest.units[b.marker]
				if marker == nil || marker.object == nil || forest.objects[b.marker] != marker.object || !nativeAnonymousForestBridgeMarker(forest, b.marker) || !nativeMemberJointBridgeDeclaration(p, target, b.method, b.marker, work) {
					return false
				}
				bridge = true
			}
		}
		switch dynamic := constant.(type) {
		case *ConstantDynamicInfo:
			if dynamic == nil || int(dynamic.NameAndTypeIndex) == index {
				return false
			}
		case *ConstantInvokeDynamicInfo:
			if dynamic == nil || int(dynamic.NameAndTypeIndex) == index {
				return false
			}
		case *ConstantMethodHandleInfo:
			if dynamic == nil || dynamic.ReferenceIndex < 1 || int(dynamic.ReferenceIndex) > len(object.ConstantPool) {
				return false
			}
			if ref := nativeConstantMember(object.ConstantPool[dynamic.ReferenceIndex-1]); ref != nil && int(ref.NameAndTypeIndex) == index {
				return false
			}
		}
	}
	return anonymous && bridge
}
