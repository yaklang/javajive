package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// The pre-nestmate compiler emits this class only to distinguish a package-visible access bridge
// from its private target. It has no executable body and is never allocated.
// Its original EnclosingMethod/self row (not dollar depth) fixes the owner;
// the exact ordinal is a compiler regeneration obligation of the whole nest.
func nativeMemberEmptyAccessMarker(obj *ClassObject, owner string, work *workbudget.Budget) bool {
	if obj == nil || (obj.MajorVersion < 51 || obj.MajorVersion > 52) || obj.MinorVersion != 0 || obj.AccessFlags != 0x1020 || obj.GetSupperClassName() != "java/lang/Object" || len(obj.Interfaces) != 0 || len(obj.Fields) != 0 || len(obj.Methods) != 0 || obj.GetClassName() != owner+"$1" || len(obj.Attributes) > 3 || len(obj.ConstantPool) > 64 {
		return false
	}
	if !nativeProofWork(work, int64(len(obj.Attributes)+len(obj.ConstantPool))) {
		return false
	}
	original, method, known := originalAnonymousOwner(obj)
	if !known || original != owner || method != "" {
		return false
	}
	enclosing, inner, source := 0, 0, 0
	for _, a := range obj.Attributes {
		switch a := a.(type) {
		case *UnparsedAttribute:
			if a == nil || a.Name != "EnclosingMethod" {
				return false
			}
			enclosing++
		case *InnerClassesAttribute:
			if a == nil || len(a.Classes) != 1 {
				return false
			}
			row := a.Classes[0]
			if row == nil || row.OuterClassInfoIndex != 0 || row.InnerNameIndex != 0 || row.InnerClassAccessFlags != 0x1008 {
				return false
			}
			name, ok := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
			if !ok || name != obj.GetClassName() {
				return false
			}
			inner++
		case *SourceFileAttribute:
			if a == nil {
				return false
			}
			source++
		default:
			return false
		}
	}
	for _, c := range obj.ConstantPool {
		switch c.(type) {
		case *ConstantUtf8Info, *ConstantClassInfo:
		default:
			return false
		}
	}
	return enclosing == 1 && inner == 1 && source <= 1
}
