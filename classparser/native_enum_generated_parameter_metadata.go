package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

type nativeEnumParameterCompiler struct {
	profile SourceCompilerProfile
	source  int
}

// These names are fixed by the selected compiler lowering; source cannot
// declare the generated valueOf parameter or enum's hidden name/ordinal.
// This proof supplements, never replaces, the existing executable factory,
// backing-array and constructor-flow proofs. No original metadata is changed.
func nativeEnumGeneratedParameterNames(obj *ClassObject, compiler nativeEnumParameterCompiler, work *workbudget.Budget) bool {
	if obj == nil || obj.MajorVersion != 52 || obj.MinorVersion != 0 || obj.AccessFlags&0x4000 == 0 || compiler.source != 8 || compiler.profile != NativeJavac8 && compiler.profile != ModernJavac || len(obj.Methods) > 4096 || !nativeProofWork(work, 1) {
		return false
	}
	constructors, factories, helpers := 0, 0, 0
	for _, method := range obj.Methods {
		if method == nil || !nativeProofWork(work, int64(len(method.Attributes))+1) {
			return false
		}
		name, nk := sourceBridgeUTF8(obj, method.NameIndex)
		descriptor, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
		if !nk || !dk || len(name) > 65535 || len(descriptor) > 65535 || !nativeProofWork(work, int64(len(name)+len(descriptor))) {
			return false
		}
		if method.AccessFlags == 0x100a && descriptor == "()[L"+obj.GetClassName()+";" {
			helpers++
		}
		var names []string
		flags := uint16(0)
		switch name {
		case "valueOf":
			if descriptor != "(Ljava/lang/String;)L"+obj.GetClassName()+";" {
				continue // An ordinary overload keeps its own source declaration.
			}
			factories++
			names, flags = []string{"name"}, 0x8000
		case "<init>":
			parameters, result, err := callbinding.Descriptor(descriptor)
			// Source constructor parameters need their own header/name binding
			// certificate. Here only the compiler-owned hidden prefix is used.
			if err != nil || result != "V" || len(parameters) != 2 || parameters[0] != "Ljava/lang/String;" || parameters[1] != "I" {
				return false
			}
			constructors++
			names, flags = []string{"$enum$name", "$enum$ordinal"}, 0x1000
		default:
			continue
		}
		var table *UnparsedAttribute
		for _, attribute := range method.Attributes {
			if raw, ok := attribute.(*UnparsedAttribute); ok && raw != nil && raw.Name == "MethodParameters" {
				if table != nil {
					return false
				}
				table = raw
			}
		}
		if table == nil || table.Length != uint32(len(table.Info)) || len(table.Info) != 1+4*len(names) || int(table.Info[0]) != len(names) || !nativeProofWork(work, int64(len(table.Info))) {
			return false
		}
		for i, expected := range names {
			row := table.Info[1+4*i : 5+4*i]
			actual, known := sourceBridgeUTF8(obj, binary.BigEndian.Uint16(row))
			if !known || actual != expected || binary.BigEndian.Uint16(row[2:]) != flags {
				return false
			}
		}
	}
	// Native8 emits the values array inline; the selected modern compiler
	// emits its private synthetic helper. They differ observably in methods.
	return constructors == 1 && factories == 1 && (compiler.profile == NativeJavac8 && helpers == 0 || compiler.profile == ModernJavac && helpers == 1) && native8LambdaParameterCompilerFlagClosed(obj, work)
}

// -parameters applies to the complete original owning source unit. A named
// generated enum member cannot authorize inventing tables on an unmarked
// outer/sibling method. The completed forest supplies ownership, not a guessed
// binary-name prefix or classes available only as external declarations.
func nativeMemberEnumParameterUnitClosed(family *nativeMemberFamily, object *ClassObject, work *workbudget.Budget) bool {
	if object == nil || !native8LambdaParameterCompilerFlagClosed(object, work) {
		return false
	}
	if family == nil {
		// A separately dumped member is not its original owning source unit.
		_, _, _, nested := originalMemberOwner(object)
		_, _, anonymous := originalAnonymousOwner(object)
		return !nested && !anonymous
	}
	if family.owner == "" || family.lexicalObjects[family.owner] == nil || len(family.lexicalObjects) == 0 || len(family.lexicalObjects) > nativeMemberLayoutNodeLimit+1 || family.lexicalObjects[object.GetClassName()] != object {
		return false
	}
	for name, original := range family.lexicalObjects {
		if original == nil || original.GetClassName() != name || !native8LambdaParameterCompilerFlagClosed(original, work) {
			return false
		}
		if owner, _, _, nested := originalMemberOwner(original); nested && family.lexicalObjects[owner] == nil {
			return false
		}
	}
	return true
}
