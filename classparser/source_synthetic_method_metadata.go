package javaclassparser

import (
	"encoding/binary"
	"slices"

	"github.com/yaklang/javajive/internal/workbudget"
)

const syntheticMethodFlag uint16 = 0x1000

// Ordinary Java declarations cannot spell ACC_SYNTHETIC. This optional
// reconstruction step restores only that missing bit, under a bijection of
// every original/fresh method and equality of every other method/class flag.
// It does not rename a method, grant bridge/constructor recovery, edit Code or
// make a method-universe/visibility mismatch acceptable. If this narrow client
// does not apply, the existing metadata restorer still owns the final refusal.
func restoreDeclaredSyntheticMethodFlags(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	return restoreSyntheticDeclarationFlags(original, compiled, false, nil, work)
}

func restoreReconstructionSyntheticFlags(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	return restoreSyntheticDeclarationFlags(original, compiled, true, nil, work)
}

func restoreReconstructionSyntheticFlagsWithLambda(original, compiled []byte, plan *sourceLambdaMetadataPlan, work *workbudget.Budget) ([]byte, error) {
	return restoreSyntheticDeclarationFlags(original, compiled, true, plan, work)
}

func restoreSyntheticDeclarationFlags(original, compiled []byte, constructorPackets bool, plan *sourceLambdaMetadataPlan, work *workbudget.Budget) ([]byte, error) {
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return nil, sourceLambdaMetadataFailure(work, "synthetic method input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	reader, err := NewClassReaderWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
	got, err := parseWithMetadataLayout(reader, nil, layout)
	if err != nil {
		return nil, err
	}
	if !sourceParameterClassIdentityClosed(want, got) || len(want.Methods) != len(got.Methods) || len(got.Methods) > 4096 {
		return compiled, nil
	}
	if err := work.CheckAlloc(int64(len(got.Methods)) * 256); err != nil {
		return nil, err
	}
	identity := func(object *ClassObject, method *MemberInfo) (sourceLambdaMethodIdentity, bool) {
		if method == nil {
			return sourceLambdaMethodIdentity{}, false
		}
		name, nk := sourceParameterUTF8Key(object, method.NameIndex)
		desc, dk := sourceParameterUTF8Key(object, method.DescriptorIndex)
		return sourceLambdaMethodIdentity{name, desc}, nk && dk && name != "" && desc != ""
	}
	flags := map[sourceLambdaMethodIdentity]uint16{}
	for _, method := range want.Methods {
		key, known := identity(want, method)
		if !known || !nativeProofWork(work, int64(len(key.name)+len(key.descriptor))+1) {
			return nil, sourceLambdaMetadataFailure(work, "invalid original synthetic method identity")
		}
		if _, duplicate := flags[key]; duplicate {
			return compiled, nil
		}
		flags[key] = method.AccessFlags
	}
	projection, err := sourceLambdaDeclarationProjection(want, original, plan, work, func(key sourceLambdaMethodIdentity) (uint16, bool) { flags, known := flags[key]; return flags, known })
	if err != nil {
		return nil, err
	}
	seen := map[sourceLambdaMethodIdentity]bool{}
	var edits []compiledMetadataEdit
	var constructorProof func(*MemberInfo) bool
	constructorProofReady := false
	for _, method := range got.Methods {
		key, known := identity(got, method)
		if !known || !nativeProofWork(work, int64(len(key.name)+len(key.descriptor))+1) {
			return nil, sourceLambdaMetadataFailure(work, "invalid compiled synthetic method identity")
		}
		if mapped, found := projection[key]; found {
			key = mapped
		}
		expected, found := flags[key]
		if !found || seen[key] || expected&^syntheticMethodFlag != method.AccessFlags&^syntheticMethodFlag || method.AccessFlags&syntheticMethodFlag != 0 && expected&syntheticMethodFlag == 0 {
			return compiled, nil
		}
		seen[key] = true
		if expected == method.AccessFlags {
			continue
		}
		// A constructor is eligible only under its own original/fresh typed
		// forwarding proof. The ordinary metadata client keeps its old domain.
		if key.name == "<init>" {
			if !constructorPackets {
				return compiled, nil
			}
			if !constructorProofReady {
				constructorProof = syntheticConstructorMarkerProof(want, got, work)
				constructorProofReady = true
			}
			if constructorProof == nil || !constructorProof(method) {
				if err := work.Check(); err != nil {
					return nil, err
				}
				return compiled, nil
			}
		}
		if key.name == "<clinit>" || expected&0x0040 != 0 {
			return compiled, nil
		}
		span, recorded := layout.methods[method]
		if !recorded || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 || binary.BigEndian.Uint16(compiled[span.start:]) != method.AccessFlags {
			return nil, sourceLambdaMetadataFailure(work, "invalid synthetic method declaration span")
		}
		data := make([]byte, 2)
		binary.BigEndian.PutUint16(data, expected)
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{span.start, span.start + 2}, data})
	}
	if len(edits) == 0 {
		return compiled, work.Check()
	}
	return applyCompiledMetadataEdits(compiled, edits, work)
}

// The unused trailing port is not identified by a numeric class-name suffix.
// Both actual packets must load the same typed arguments, invoke the same
// private constructor on uninitialized-this, and return without other effects.
// Whole class/method identities and flags are checked by the caller first.
func syntheticConstructorMarkerProof(original, fresh *ClassObject, work *workbudget.Budget, completedFlags ...uint16) func(*MemberInfo) bool {
	flags := uint16(0)
	if len(completedFlags) > 1 {
		return nil
	}
	if len(completedFlags) == 1 {
		flags = completedFlags[0]
		if flags != syntheticMethodFlag {
			return nil
		}
	}
	if original == nil || original.MajorVersion < 49 || original.MajorVersion > 54 || original.MinorVersion != 0 || hasOriginalNestAttribute(original, "NestHost") || hasOriginalNestAttribute(original, "NestMembers") {
		return nil
	}
	// Modern javac uses nest access and no longer regenerates an old access
	// constructor. Its explicitly emitted forwarding declaration still needs
	// the unspellable synthetic bit. This accepts no new original ownership
	// protocol, no missing method, and no change to the freshly compiled Code.
	if fresh == nil || fresh.MajorVersion < 49 || fresh.MajorVersion > 65 || fresh.MinorVersion != 0 || fresh.MajorVersion < 55 && (hasOriginalNestAttribute(fresh, "NestHost") || hasOriginalNestAttribute(fresh, "NestMembers")) || !nativeAccessorConstantNamespace(fresh, work) {
		return nil
	}
	a, b := NewClassObjectDumper(original), NewClassObjectDumper(fresh)
	a.Work, b.Work = work, work
	// Index each method once, rather than rescanning the class per overload.
	oldPackets, freshPackets := a.originalNativeConstructorAccessBridges(), b.constructorForwardingPackets(flags)
	exceptions := func(object *ClassObject, member *MemberInfo) ([]string, bool) {
		var result []string
		seen := false
		for _, attribute := range member.Attributes {
			if !nativeProofWork(work, 1) {
				return nil, false
			}
			if table, ok := attribute.(*ExceptionsAttribute); ok {
				if table == nil || seen {
					return nil, false
				}
				seen = true
				for _, index := range table.ExceptionIndexTable {
					name, known := sourceParameterClassKey(object, index)
					if !known || !nativeProofWork(work, int64(len(name))+1) {
						return nil, false
					}
					result = append(result, name)
				}
			}
		}
		return result, true
	}
	return func(method *MemberInfo) bool {
		descriptor, known := sourceBridgeUTF8(fresh, method.DescriptorIndex)
		if !known {
			return false
		}
		left, right := oldPackets[descriptor], freshPackets[descriptor]
		if left == nil || right == nil || right.method != method || left.descriptor != right.descriptor || left.target != right.target || left.marker != right.marker {
			return false
		}
		leftThrows, leftKnown := exceptions(original, left.method)
		rightThrows, rightKnown := exceptions(fresh, right.method)
		return leftKnown && rightKnown && slices.Equal(leftThrows, rightThrows)
	}
}
