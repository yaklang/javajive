package javaclassparser

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"

	"github.com/yaklang/javajive/internal/memfile"
	"github.com/yaklang/javajive/internal/workbudget"
)

// JarSourceArtifact is a source view with explicit postcompile compiler
// metadata obligation. ReadFile returns Java source under the original class
// paths, as JarFS does. Recompile with the selected compiler profile, then call
// RestoreClassMetadata for each rebuilt class before publishing the artifact.
// Plain JarFS source admission remains unchanged.
//
// This contract can express ordinary synthetic storage fields and proved
// nonserializable private lambda helper names/parameter tables without dropping
// their reflective metadata or importing original executable code. Lambda
// obligations require a completed ReadFile source/factory proof in this view.
// An independently proved, fully rendered all-hidden anonymous constructor can
// also restore original MethodParameters/empty-Signature absence. This grants
// no ordinary-constructor, explicit-SUPER-argument or executable-code edit.
// It does not
// certify compilation, JVM verification, other ABI metadata or algorithmic
// equivalence; those still require independent validation.
type JarSourceArtifact struct {
	jar *JarFS
}

func NewJarSourceArtifactFromLocal(path string, target int, compiler SourceCompilerProfile, resolve func(string) ([]byte, bool)) (*JarSourceArtifact, error) {
	z, err := NewJarFSFromLocalWithCompilerProfile(path, target, compiler, resolve)
	if err != nil {
		return nil, err
	}
	z.sourceFieldMetadata = &sourceFieldMetadataAuthority{jar: z}
	z.sourceLambdaMetadata = &sourceLambdaMetadataAuthority{jar: z}
	return &JarSourceArtifact{jar: z}, nil
}

func (a *JarSourceArtifact) ReadFile(name string) ([]byte, error) {
	source, err := a.jar.ReadFile(name)
	if err != nil {
		return nil, err
	}
	z, logical := a.jar, name
	if release, candidate, physical := physicalClassNamespace(name); physical {
		z, logical = z.sourceReleaseView(release), candidate
	}
	if z != nil && z.sourceLambdaMetadata != nil && strings.HasSuffix(logical, ".class") {
		if err := z.sourceLambdaMetadata.sourceReturned(strings.TrimSuffix(logical, ".class"), source); err != nil {
			return nil, err
		}
	}
	return source, nil
}
func (a *JarSourceArtifact) Open(name string) (fs.File, error) {
	source, err := a.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return memfile.New(source), nil
}
func (a *JarSourceArtifact) ReadDir(name string) ([]fs.DirEntry, error) {
	return a.jar.ReadDir(name)
}
func (a *JarSourceArtifact) Close() error { return a.jar.Close() }

// RestoreClassMetadata binds restoration to this archive's original bytes and
// the exact physical source namespace. Declaration-resolver classes cannot
// authorize edits. Only proved field/lambda/implicit-constructor metadata is restored; executable
// Code and all other compiled attribute payloads remain unchanged.
func (a *JarSourceArtifact) RestoreClassMetadata(name string, compiled []byte) ([]byte, error) {
	if a == nil || a.jar == nil {
		return nil, fs.ErrInvalid
	}
	z, logical := a.jar, name
	if release, candidate, physical := physicalClassNamespace(name); physical {
		z, logical = z.sourceReleaseView(release), candidate
	}
	if z == nil || !strings.HasSuffix(logical, ".class") || !nativeSourceBinaryName(strings.TrimSuffix(logical, ".class")) {
		return nil, fmt.Errorf("field metadata: noncanonical class path %q", name)
	}
	original, err := z.readJarEntry(logical)
	if err != nil {
		return nil, err
	}
	reader := z.nativeMemberReader(nil)
	object, err := ParseWithBudget(original, reader.Work)
	if err != nil {
		return nil, err
	}
	if object.GetClassName()+".class" != logical {
		return nil, fmt.Errorf("field metadata: archive path and class identity disagree")
	}
	if authority := z.sourceLambdaMetadata; authority != nil {
		if plan := authority.constructorPlan(object.GetClassName()); plan != nil {
			compiled, err = restoreImplicitConstructorParameterMetadata(original, compiled, plan, reader.Work)
			if err != nil {
				return nil, err
			}
		}
		if plan := authority.plan(object.GetClassName()); plan != nil {
			compiled, err = restoreLambdaMethodMetadata(original, compiled, plan, reader.Work)
			if err != nil {
				return nil, err
			}
		}
	}
	result, err := RestoreSyntheticFieldMetadata(original, compiled, reader.Work)
	if err != nil {
		return nil, err
	}
	if err := sourceMethodMetadataClosed(object, result, reader.Work); err != nil {
		return nil, err
	}
	return result, nil
}

// This capability exists only in a fresh artifact source view. The original
// class objects remain authoritative for types, captures and bytecode proofs.
// A declaration-only resolver never enrolls a foreign class in the artifact.
type sourceFieldMetadataAuthority struct{ jar *JarFS }

func (a *sourceFieldMetadataAuthority) fields(object *ClassObject, work *workbudget.Budget) map[*MemberInfo]bool {
	if a == nil || a.jar == nil || object == nil {
		return nil
	}
	if !nativeProofWork(work, int64(len(object.Fields))+1) {
		return nil
	}
	wanted := false
	for _, field := range object.Fields {
		if flags, _, known := nativeMemberEffectiveFieldFlags(field, work); known && flags&syntheticFieldFlag != 0 {
			wanted = true
		}
	}
	if !wanted || !nativeSourceBinaryName(object.GetClassName()) {
		return nil
	}
	raw, err := a.jar.readJarEntry(object.GetClassName() + ".class")
	if err != nil || !nativeProofWork(work, int64(len(raw))*2+1) {
		return nil
	}
	if work != nil && work.CheckAlloc(int64(len(raw))*4) != nil {
		return nil
	}
	original, err := ParseWithBudget(raw, work)
	if err != nil {
		return nil
	}
	originalEncoding, objectEncoding := original.Bytes(), object.Bytes()
	if len(originalEncoding) < 10 || len(objectEncoding) < 10 || !bytes.Equal(originalEncoding, objectEncoding) {
		return nil
	}
	fields := make(map[*MemberInfo]bool)
	for _, field := range object.Fields {
		flags, _, known := nativeMemberEffectiveFieldFlags(field, work)
		if !known || flags & ^uint16(0x10df) != 0 || flags&syntheticFieldFlag == 0 || flags&0x50 == 0x50 {
			continue
		}
		visibility := flags & 7
		if visibility != 0 && visibility&(visibility-1) != 0 {
			continue
		}
		name, named := sourceBridgeUTF8(object, field.NameIndex)
		if named && name != nativeAssertionField {
			fields[field] = true
		}
	}
	return fields
}
