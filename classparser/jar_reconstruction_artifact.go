package javaclassparser

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/yaklang/javajive/internal/memfile"
)

// JarReconstructionArtifact is an explicit source-plus-lowering transaction.
// Unlike ordinary source, its Java files can owe a postcompile executable
// lowering. Compile them with the selected compiler, then call FinishClass for
// EVERY output class before publishing or executing the result. In particular,
// a flat constructor's capture assignments can require restoration before
// SUPER. Running the intermediate compiler output is not equivalent.
//
// Only matched instructions in the freshly compiled class are reordered. No
// original executable bytes are copied. All existing field, method, bridge and
// metadata checks remain required. This operation does not certify the rest of
// a method, JVM verification or whole-program equivalence; callers still owe
// independent validation. Plain JarFS and JarSourceArtifact are unchanged.
// Ordinary declared methods can additionally recover a missing ACC_SYNTHETIC
// marker under an unchanged complete method/class declaration universe. This
// grants no constructor, initializer, bridge, visibility or method-name edits.
// A separate pre-nest constructor proof can restore only ACC_SYNTHETIC on a
// forwarding constructor whose original and fresh packets load identical typed
// arguments, call the same private constructor, ignore the same trailing port,
// and return without additional effects or handlers. Neither packet is copied
// or edited; its descriptor, exception contract and all other flags must match.
// A separately proved straight-line Java8 enum initializer can inline a fresh
// compiler's private constant-array helper, restoring the original method
// universe. Its only consumer, array element identities, empty-stack boundary
// and supported debug PCs are checked before any edit; all other metadata
// contracts still apply. The original initializer's Code is never copied.
// A separately published flat synthetic class with only static declarations
// can also owe removal of javac's added default Object constructor. Only an
// unused exact default constructor is removed, alongside restoration of the
// original synthetic class flag; every surviving fresh method stays untouched.
type JarReconstructionArtifact struct{ source *JarSourceArtifact }

var _ fs.ReadFileFS = (*JarReconstructionArtifact)(nil)
var _ fs.ReadDirFS = (*JarReconstructionArtifact)(nil)

func NewJarReconstructionArtifactFromLocal(path string, target int, compiler SourceCompilerProfile, resolve func(string) ([]byte, bool)) (*JarReconstructionArtifact, error) {
	a, err := NewJarSourceArtifactFromLocal(path, target, compiler, resolve)
	if err != nil {
		return nil, err
	}
	a.jar.sourceConstructorLowering = &sourceConstructorLoweringAuthority{jar: a.jar}
	return &JarReconstructionArtifact{source: a}, nil
}

func (a *JarReconstructionArtifact) ReadFile(name string) ([]byte, error) {
	if a == nil || a.source == nil {
		return nil, fs.ErrInvalid
	}
	source, err := a.source.ReadFile(name)
	if err != nil {
		return nil, err
	}
	z, logical := a.source.jar, name
	if release, candidate, physical := physicalClassNamespace(name); physical {
		z, logical = z.sourceReleaseView(release), candidate
	}
	if z != nil && z.sourceConstructorLowering != nil && strings.HasSuffix(logical, ".class") {
		if err := z.sourceConstructorLowering.sourceReturned(strings.TrimSuffix(logical, ".class"), source); err != nil {
			return nil, err
		}
	}
	return source, nil
}

func (a *JarReconstructionArtifact) ReadDir(name string) ([]fs.DirEntry, error) {
	if a == nil || a.source == nil {
		return nil, fs.ErrInvalid
	}
	return a.source.ReadDir(name)
}

func (a *JarReconstructionArtifact) Open(name string) (fs.File, error) {
	source, err := a.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return memfile.New(source), nil
}

func (a *JarReconstructionArtifact) Close() error {
	if a == nil || a.source == nil {
		return fs.ErrInvalid
	}
	return a.source.Close()
}

// FinishClass discharges executable lowering first, then the independent
// metadata contract. Failure returns no partially lowered class. The inputs
// are immutable; all other fresh compiled bytes retain their exact values.
func (a *JarReconstructionArtifact) FinishClass(name string, compiled []byte) ([]byte, error) {
	if a == nil || a.source == nil || a.source.jar == nil {
		return nil, fs.ErrInvalid
	}
	z, logical := a.source.jar, name
	if release, candidate, physical := physicalClassNamespace(name); physical {
		z, logical = z.sourceReleaseView(release), candidate
	}
	if z == nil || !strings.HasSuffix(logical, ".class") || !nativeSourceBinaryName(strings.TrimSuffix(logical, ".class")) {
		return nil, fmt.Errorf("constructor lowering: noncanonical class path %q", name)
	}
	original, err := z.readJarEntry(logical)
	if err != nil {
		return nil, err
	}
	reader := z.nativeMemberReader(nil)
	lowered, err := z.sourceConstructorLowering.lower(original, compiled, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.lowerEnumArrayHelper(strings.TrimSuffix(logical, ".class"), original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.lowerStaticClassDefaultConstructor(strings.TrimSuffix(logical, ".class"), original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = restoreReconstructionSyntheticFlags(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	return a.source.restoreClassMetadata(name, lowered, reconstructionCaptureMetadataTarget(z))
}
