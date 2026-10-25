package javaclassparser

import (
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/yaklang/javajive/internal/memfile"
)

// JarReconstructionArtifact is an explicit source-plus-lowering transaction.
// Unlike ordinary source, its Java files can owe a postcompile executable
// lowering. Compile them with the selected compiler, then call FinishClass for
// EVERY output class before publishing or executing the result. In particular,
// a flat constructor's capture assignments can require restoration before
// SUPER. Running the intermediate compiler output is not equivalent.
//
// Matched instructions in the freshly compiled class can be reordered. A
// separately proved qualified-SUPER projection can also neutralize only the
// compiler-added enclosing null check, preserving an original implicit nullable
// port and all instruction offsets. Original explicit null checks are excluded.
// No original executable bytes are copied. All existing field, method, bridge and
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
// A separately proved Java5-8 enum initializer can inline a fresh
// compiler's private constant-array helper, restoring the original method
// universe. Its only consumer, array element identities, empty-stack boundary
// and supported debug PCs are checked before any edit; all other metadata
// contracts still apply. The original initializer's Code is never copied.
// A separately published flat synthetic class with only static declarations
// can also owe removal of javac's added default Object constructor. Only an
// unused exact default constructor is removed, alongside restoration of the
// original synthetic class flag; every surviving fresh method stays untouched.
// A returned physical source can also owe a temporary private static checked-
// escape adapter. Its exact fresh identity-throw body is removed only after
// proving that every use directly rethrows a non-null caught operand with
// identical handler coverage and no alternate entry into its continuation.
// All retained instruction PCs and the original method universe stay fixed.
// A returned pre-default-method abstract interface can owe removal of fresh
// default bridges. Each removed declaration must be an exact typed forwarder
// to an original abstract override, and its erased contract must still resolve
// through the complete original interface hierarchy. Retained fresh methods
// and annotations are unchanged, preserving original initialization semantics.
// For a closed modern nest, read every original member's source before finishing
// its compiled classes. An exact final access forwarder can then be removed and
// its callers rebound to the original private declaration, retaining fresh Code
// except for constant-pool operands. The original reciprocal nest is restored
// only with the complete returned-source graph; handles, new callers, changed
// helper effects or private flags prevent completion. This preserves private
// dispatch, operand/null-check order and the original declaration universe.
type JarReconstructionArtifact struct {
	source     *JarSourceArtifact
	resourceMu sync.Mutex
	resources  map[string]classResourcePublication
}

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
	if z != nil && strings.HasSuffix(logical, ".class") && packageDescriptorName(strings.TrimSuffix(logical, ".class")) {
		return a.source.restoreClassMetadata(name, compiled, sourceCaptureMetadataTarget{}, nil)
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
	lowered, err = z.sourceConstructorLowering.lowerPrivateNestAdapters(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.lowerFinalInheritedBridges(strings.TrimSuffix(logical, ".class"), original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.lowerInheritedBridges(strings.TrimSuffix(logical, ".class"), original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.lowerLegacyInterfaceBridges(strings.TrimSuffix(logical, ".class"), original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.lowerEnumConstructorPort(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.restoreEnumLexicalOwner(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.restoreEnumConstantModifiers(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.restoreEnumAssertionOwner(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.lowerEnumArrayHelper(strings.TrimSuffix(logical, ".class"), original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.lowerCheckedEscapeHelper(strings.TrimSuffix(logical, ".class"), original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.lowerStaticClassDefaultConstructor(strings.TrimSuffix(logical, ".class"), original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	// Lambda spelling and ordinary synthetic flags are independent compiler
	// obligations. Apply their published identity projection before checking
	// the whole declaration bijection; no method-name pattern is authority.
	var declarationLambda *sourceLambdaMetadataPlan
	if authority := z.sourceLambdaMetadata; authority != nil {
		declarationLambda = authority.plan(strings.TrimSuffix(logical, ".class"))
	}
	// A checked lambda input changes the physical descriptor as well as Code.
	// Complete and prove that obligation before applying a declaration-only
	// projection. Keep its normalized plan local to this completion transaction.
	lowered, declarationLambda, err = lowerLambdaCheckedInputs(original, lowered, declarationLambda, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = restoreReconstructionSyntheticFlagsWithLambda(original, lowered, declarationLambda, reader.Work)
	if err != nil {
		return nil, err
	}
	// Lexical scope completion validates the entire original member universe.
	// Discharge a published storage alpha-conversion first: its independent
	// original/source receipt and typed Fieldref proof restore symbolic identity
	// without touching fresh Code. Matching an alias by spelling would weaken
	// the scope proof; skipping unrelated fields would conceal missing members.
	// The final source metadata transaction rechecks this idempotent obligation.
	if z.sourceFieldMetadata != nil && z.sourceFieldMetadata.hasAssertionStorage(strings.TrimSuffix(logical, ".class")) {
		lowered, err = z.sourceFieldMetadata.restoreAssertionStorage(original, lowered, reader.Work)
		if err != nil {
			return nil, err
		}
	}
	lowered, err = z.sourceConstructorLowering.restoreAnonymousCapturedConstructorSignature(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.restoreConstructorBindingSignatures(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.restoreMemberReferenceSignatures(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	if z.sourceConstructorLowering.memberGenericSourceReturned(strings.TrimSuffix(logical, ".class"), original, reader.Work) {
		// Scope publication checks the complete method contract. Discharge
		// compiler parameter-name/mandated-bit differences with their existing
		// independent proof first, rather than rejecting an obligation that
		// the artifact already knows how to complete. The final metadata gate
		// below still checks the entire finished result.
		lowered, err = restoreMethodParameterMetadata(original, lowered, declarationLambda, reader.Work)
		if err != nil {
			return nil, err
		}
	}
	lowered, err = z.sourceConstructorLowering.restoreMemberGenericScope(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	lowered, err = z.sourceConstructorLowering.restoreNamedMemberRows(original, lowered, reader.Work)
	if err != nil {
		return nil, err
	}
	return a.source.restoreClassMetadata(name, lowered, reconstructionCaptureMetadataTarget(z), declarationLambda)
}
