package javaclassparser

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

type classResourcePublication struct {
	owner            string
	original, source [32]byte
}

// Class resources are explicit physical byte suppliers for a defining loader.
// They do not belong to the JVM class path, even if their bytes contain a class.
// Neither a resource extension nor a same-named .class grants output authority.
// Keep this API separate from ReadFile/FinishClass: its caller must publish the
// finished bytes under the SAME resource path, never the returned binary name.
// There is no resource-to-class alias in dependency or native ownership lookup.
func (a *JarReconstructionArtifact) classResourceOriginal(name string) ([]byte, *ClassObject, *workbudget.Budget, error) {
	if a == nil || a.source == nil || a.source.jar == nil || !fs.ValidPath(name) || name == "." || strings.Contains(name, "\\") || strings.HasSuffix(strings.ToLower(name), ".class") || strings.HasPrefix(name, "META-INF/") {
		return nil, nil, nil, fmt.Errorf("class resource: noncanonical or reserved physical path %q", name)
	}
	z := a.source.jar
	selection, err := z.boundJarReleaseSelection()
	if err != nil {
		return nil, nil, nil, err
	}
	raw, err := selection.readPhysical(name, 32<<20)
	if err != nil {
		return nil, nil, nil, err
	}
	work := z.nativeMemberReader(nil).Work
	if work == nil {
		work = selection.legacyWork
	}
	obj, err := ParseWithBudget(raw, work)
	if err != nil {
		return nil, nil, work, err
	}
	if !nativeSourceBinaryName(obj.GetClassName()) || packageDescriptorName(obj.GetClassName()) {
		return nil, nil, work, fmt.Errorf("class resource: unsupported binary identity")
	}
	return raw, obj, work, nil
}

// ReadClassResource publishes one independently compiled resource class. The
// binary name supplies the Java source file name only. Original resource paths
// and their loader role remain explicit. This initial domain has no executable
// lowering or borrowed source capability: an extra nested class, lambda adapter
// or changed declaration universe must still fail completion.
func (a *JarReconstructionArtifact) ReadClassResource(name string) ([]byte, string, error) {
	raw, obj, work, err := a.classResourceOriginal(name)
	if err != nil {
		return nil, "", err
	}
	z := a.source.jar
	d := NewClassObjectDumper(obj)
	d.Work = work
	d.options.TargetSourceVersion = z.targetSourceVersion
	d.options.SourceCompiler = z.sourceCompiler
	d.declarationResolver = z.declarationResolver
	// Real archive declarations may inform binding, but cannot turn this
	// resource into an owned class or confer reconstruction permissions.
	d.foldSiblingResolver = z.enumSiblingResolver()
	source, err := d.DumpClass()
	if err != nil {
		return nil, "", err
	}
	if source == "" || legacySourceHasFailureMarker(source) || d.sourceDegraded || len(source) > 32<<20 || !nativeProofWork(work, int64(len(source))+1) {
		return nil, "", sourceLambdaMetadataFailure(work, "class resource source was not completely published")
	}
	origin := classResourcePublication{owner: obj.GetClassName(), original: sha256.Sum256(raw), source: sha256.Sum256([]byte(source))}
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	if previous, found := a.resources[name]; found && previous != origin {
		return nil, "", fmt.Errorf("class resource: original or source publication changed")
	}
	if len(a.resources) >= 4096 {
		if _, found := a.resources[name]; !found {
			return nil, "", fmt.Errorf("class resource: publication budget")
		}
	}
	if a.resources == nil {
		a.resources = map[string]classResourcePublication{}
	}
	a.resources[name] = origin
	return []byte(source), origin.owner, nil
}

// FinishClassResource restores only independently checkable metadata on FRESH
// compiler output. It never copies original Code, substitutes a dependency,
// invents a .class path or ignores an extra declaration. It is not a proof of
// algorithmic equivalence: the caller still owes a loader/runtime oracle.
func (a *JarReconstructionArtifact) FinishClassResource(name string, compiled []byte) ([]byte, error) {
	raw, obj, work, err := a.classResourceOriginal(name)
	if err != nil {
		return nil, err
	}
	a.resourceMu.Lock()
	origin, found := a.resources[name]
	a.resourceMu.Unlock()
	if !found || origin.owner != obj.GetClassName() || origin.original != sha256.Sum256(raw) || origin.source == ([32]byte{}) {
		return nil, fmt.Errorf("class resource: missing or stale physical source publication")
	}
	if len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(compiled))+1) {
		return nil, sourceLambdaMetadataFailure(work, "class resource compiler output budget")
	}
	// No lambda/constructor/source alias plan can be borrowed from a .class
	// with the same internal name. Each complete metadata guard uses this
	// physical resource's original declaration, without executable edits.
	result, err := restoreMethodParameterMetadata(raw, compiled, nil, work)
	if err != nil {
		return nil, err
	}
	result, err = RestoreSyntheticFieldMetadata(raw, result, work)
	if err != nil {
		return nil, err
	}
	if err = sourceMethodMetadataClosed(obj, result, work); err != nil {
		return nil, err
	}
	if err = sourceArtifactBridgeProgramsClosed(obj, result, work); err != nil {
		return nil, err
	}
	return result, nil
}

// FinishOutput routes a canonical compiler output to its original physical
// publication role. Callers rebuilding a mixed archive must honor the returned
// path. Ambiguous resource/class suppliers or two resources with one binary
// identity cannot be resolved by choosing whichever happened to render last.
// FinishClass itself remains strict and never borrows resource permissions.
func (a *JarReconstructionArtifact) FinishOutput(name string, compiled []byte) (string, []byte, error) {
	if a == nil || a.source == nil || a.source.jar == nil || !fs.ValidPath(name) || !strings.HasSuffix(name, ".class") {
		return "", nil, fmt.Errorf("reconstruction output: noncanonical class path")
	}
	owner := strings.TrimSuffix(name, ".class")
	a.resourceMu.Lock()
	resource, count := "", 0
	for path, origin := range a.resources {
		if origin.owner == owner {
			resource, count = path, count+1
		}
	}
	a.resourceMu.Unlock()
	if count == 0 {
		out, err := a.FinishClass(name, compiled)
		if err != nil {
			return "", nil, err
		}
		return name, out, nil
	}
	selection, err := a.source.jar.boundJarReleaseSelection()
	if err != nil {
		return "", nil, err
	}
	if _, err = selection.catalog(); err != nil {
		return "", nil, err
	}
	_, ordinary := selection.exact[name]
	if count != 1 || ordinary {
		return "", nil, fmt.Errorf("reconstruction output: ambiguous physical suppliers for %q", name)
	}
	out, err := a.FinishClassResource(resource, compiled)
	if err != nil {
		return "", nil, err
	}
	return resource, out, nil
}
