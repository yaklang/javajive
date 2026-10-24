package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/internal/jdecenv"
	"os"
	"path/filepath"
	"strings"
)

// Planning and rendering are separate phases so a dependency transaction can
// establish source names without publishing any private scope or incomplete
// source. Each participant still performs the same original ownership, bridge,
// allocation and final source certificates as a single family.
type nativeMemberPrepared struct {
	root     *ClassObject
	reader   *ClassObjectDumper
	family   *nativeMemberFamily
	objects  map[string]*ClassObject
	snapshot map[string]string
}

func (z *JarFS) prepareNativeMemberFamily(root *ClassObject, snap map[string]string) *nativeMemberPrepared {
	prepared := z.prepareNativeMemberFamilyUnpublished(root, snap)
	if prepared == nil || !z.nativeMemberAccessRepresentable(prepared.family, z.originalMemberIndex(), prepared.reader.Work) {
		return nil
	}
	return prepared
}

// A local plan is not a source accessibility certificate. Keep every original
// ownership/allocation proof, but let an atomic dependency transaction establish
// its lexical peers before checking the original protected type users.
func (z *JarFS) prepareNativeMemberFamilyUnpublished(root *ClassObject, snap map[string]string) *nativeMemberPrepared {
	return z.prepareNativeMemberFamilyFromRoot(root, snap, nil)
}

func (z *JarFS) prepareNativeMemberFamilyFromRoot(root *ClassObject, snap map[string]string, independent *nativeMemberIndependentRoot) *nativeMemberPrepared {
	if root == nil {
		return nil
	}
	owner := root.GetClassName()
	d := z.nativeMemberReader(root)
	d.options.EnvSnapshot = snap
	p := d.planNativeMemberFamilyFromRoot(independent)
	if p == nil {
		fmt.Printf("FAMILY_DIAG %s:45\n", owner)
		return nil
	}
	if independent != nil && !independent.familyClosed(p, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:48\n", owner)
		return nil
	}
	if !d.planNativeMemberAnonymousScopes(p) {
		fmt.Printf("FAMILY_DIAG %s:51\n", owner)
		return nil
	}
	if !nativeMemberCompilerMetadataEligible(root, p, d.options.SourceCompiler, d.options.TargetSourceVersion, d.Work, d.options.sourceLambdaMetadata) {
		fmt.Printf("FAMILY_DIAG %s:54\n", owner)
		return nil
	}
	// Ordinal completeness is a post-forest invariant. It grants no source
	// access by itself; the following archive and permission proofs still run.
	if !nativeMemberAccessorRegistrationsClosed(p, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:59\n", owner)
		return nil
	}
	if !nativeEnumSwitchOrdinalClosed(p, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:62\n", owner)
		return nil
	}
	if !nativeMemberJointBridgeMarkersClosed(p, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:65\n", owner)
		return nil
	}
	index := z.originalMemberIndex()
	if !z.nativeEnumSwitchSourceUsersClosed(p, root, index, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:69\n", owner)
		return nil
	}
	if p.anonymousForest != nil && !nativeAnonymousForestArchiveClosed(p.anonymousForest, index, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:72\n", owner)
		return nil
	}
	if !z.nativeMethodLocalArchiveClosed(p, index) {
		fmt.Printf("FAMILY_DIAG %s:75\n", owner)
		return nil
	}
	if !index.valid || !z.nativeMemberStaticConstantsReferencesClosed(p, index, d.Work) || !nativeMemberPrivateGetterReferencesClosed(p, index, d.Work) || !z.nativeMemberJointBridgeReferencesClosed(p, index, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:78\n", owner)
		return nil
	}
	if len(p.rootAccessBridges) > 0 && index.handles[owner] {
		// Constructor bridges change a physical constructor protocol. Other
		// handles can survive only with unchanged original method bindings.
		// A lambda additionally needs exclusive bootstrap use, archive closure,
		// and a later receipt from this exact root's rendered implementation.
		p.rootHandleSource = &nativeMemberClass{object: root, lambdaContext: nativeLambdaImplementationContext{resolve: d.nativeAnnotationDeclarationResolver(), metadata: d.buildInvocationMetadata()}}
		if !nativeRootBridgeMethodHandlesClosed(root, index, p.rootHandleSource, d.Work) || !z.nativeMemberLambdaArchiveClosed(p.rootHandleSource, index, d.Work) {
			fmt.Printf("FAMILY_DIAG %s:87\n", owner)
			return nil
		}
	}
	objects := map[string]*ClassObject{owner: root}
	// Use the exact original objects whose constructor packets were proved.
	// A fresh parse with the same binary name is not an ownership witness.
	for name, group := range p.anonymousUnits {
		unit := group.children[name]
		if unit == nil || unit.object == nil || unit.object.GetClassName() != name || !nativeProofWork(d.Work, 1) {
			fmt.Printf("FAMILY_DIAG %s:96\n", owner)
			return nil
		}
		if !z.nativeAnonymousLambdaChildArchiveClosed(unit, index, d.Work) {
			fmt.Printf("FAMILY_DIAG %s:99\n", owner)
			return nil
		}
		objects[name] = unit.object
	}
	for binary, local := range p.methodLocals {
		objects[binary] = local.object
	}
	for name, body := range p.enumConstants {
		if body == nil || !nativeMemberOrdinaryHandlesClosed(body.object, index, d.Work) {
			fmt.Printf("FAMILY_DIAG %s:108\n", owner)
			return nil
		}
		objects[name] = body.object
	}
	if !z.nativeEnumConstantsArchiveClosed(p, index, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:113\n", owner)
		return nil
	}
	rootPrivateConstructor := false
	for _, method := range root.Methods {
		if !nativeProofWork(d.Work, 1) || method == nil {
			fmt.Printf("FAMILY_DIAG %s:118\n", owner)
			return nil
		}
		name, known := sourceBridgeUTF8(root, method.NameIndex)
		if !known {
			fmt.Printf("FAMILY_DIAG %s:122\n", owner)
			return nil
		}
		rootPrivateConstructor = rootPrivateConstructor || name == "<init>" && method.AccessFlags&2 != 0
	}
	if len(p.rootAccessBridges) > 0 || rootPrivateConstructor {
		for user := range index.constructors[owner] {
			if objects[user] != nil {
				continue
			}
			raw, known := z.enumSiblingResolver()(user)
			if !known {
				fmt.Printf("FAMILY_DIAG %s:133\n", owner)
				return nil
			}
			other, err := d.parseResolved(raw)
			if err != nil || other.GetClassName() != user {
				fmt.Printf("FAMILY_DIAG %s:137\n", owner)
				return nil
			}
			objects[user] = other
		}
	}
	for n, child := range p.children {
		objects[n] = child.object
		if !nativeMemberOrdinaryHandlesClosed(child.object, index, d.Work, child) {
			fmt.Printf("FAMILY_DIAG %s:145\n", owner)
			return nil
		}
		if !z.nativeMemberLambdaArchiveClosed(child, index, d.Work) {
			fmt.Printf("FAMILY_DIAG %s:148\n", owner)
			return nil
		}
		for user := range index.captureUsers[nativeMemberCaptureIndexKey(n, child.field)] {
			if user != n {
				if named := p.children[user]; named != nil {
					if _, valid := nativeMemberLexicalReads(named.object, p, d.Work); !valid {
						fmt.Printf("FAMILY_DIAG %s:154\n", owner)
						return nil
					}
				} else if local := p.methodLocals[user]; local != nil {
					if _, valid := nativeMemberLexicalReads(local.object, p, d.Work); !valid {
						fmt.Printf("FAMILY_DIAG %s:158\n", owner)
						return nil
					}
				} else {
					group := p.anonymousUnits[user]
					if group == nil || !nativeMemberProjectedAnonymousCaptureRead(p, group.children[user], n, d.Work) {
						fmt.Printf("FAMILY_DIAG %s:163\n", owner)
						return nil
					}
				}
			}
		}
		for user := range index.constructors[n] {
			if objects[user] != nil {
				continue
			}
			raw, known := z.enumSiblingResolver()(user)
			if !known {
				fmt.Printf("FAMILY_DIAG %s:174\n", owner)
				return nil
			}
			other, e := d.parseResolved(raw)
			if e != nil || other.GetClassName() != user {
				fmt.Printf("FAMILY_DIAG %s:178\n", owner)
				return nil
			}
			objects[user] = other
		}
	}
	for _, object := range objects {
		reader := z.nativeMemberReader(object)
		allocations, known := reader.nativeMemberAllocations(p)
		if !known || !nativeMemberJointBridgeCallersClosed(p, object, allocations, d.Work) {
			fmt.Printf("FAMILY_DIAG %s:187\n", owner)
			return nil
		}
	}
	for name, bridges := range p.bridgeOwners() {
		for descriptor := range bridges {
			if p.bridgeCalls[name+descriptor] == 0 {
				fmt.Printf("FAMILY_DIAG %s:193\n", owner)
				return nil
			}
		}
	}

	if !nativeModernNestSourceScopeClosed(p.modernNestObjects, objects, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:199\n", owner)
		return nil
	}
	if !nativeMemberPrivatePermissionClosed(p, d.nativeAnnotationDeclarationResolver(), d.Work, d.nativePrivatePermissionPlatformParents()) {
		fmt.Printf("FAMILY_DIAG %s:202\n", owner)
		return nil
	}

	return &nativeMemberPrepared{root: root, reader: d, family: p, objects: objects, snapshot: snap}
}

func (z *JarFS) finishNativeMemberFamily(prepared *nativeMemberPrepared, lookup func(string) *nativeMemberClass, dependencyGraphClosed bool, peers ...map[string]*nativeMemberPrepared) *nativeMemberCacheEntry {
	if prepared == nil || lookup == nil || len(peers) != 0 && !dependencyGraphClosed {
		return nil
	}
	root, d, p, objects, snap := prepared.root, prepared.reader, prepared.family, prepared.objects, prepared.snapshot
	owner := p.owner
	if p.independentRoot != nil && (!p.independentRoot.validFor(d) || !p.independentRoot.familyClosed(p, d.Work)) {
		fmt.Printf("FAMILY_DIAG %s:215\n", owner)
		return nil
	}
	if !z.nativeMemberIndependentInvocationsClosed(prepared) {
		fmt.Printf("FAMILY_DIAG %s:218\n", owner)
		return nil
	}
	if !z.nativeMemberAccessRepresentable(p, z.originalMemberIndex(), d.Work, peers...) {
		fmt.Printf("FAMILY_DIAG %s:221\n", owner)
		return nil
	}
	if len(p.getters) > 0 && !z.nativeMemberAccessorAncestorFlagsClosed(prepared, dependencyGraphClosed, peers...) {
		fmt.Printf("FAMILY_DIAG %s:224\n", owner)
		return nil
	}
	// Independent direct families have independent commits to ownership.
	// Complete dependencies contribute declaration and constructor binding
	// views; their private/lexical ownership stays in their own transaction.
	lexicalNames := map[string]bool{}
	for _, child := range p.children {
		lexicalNames[child.name] = true
	}
	packageRoot := strings.SplitN(owner, "/", 2)[0]
	if strings.Contains(owner, "/") && lexicalNames[packageRoot] {
		fmt.Printf("FAMILY_DIAG %s:235\n", owner)
		return nil
	}
	staticDependenciesChecked := false
	dependencyObjects, known := nativeMemberDependencyObjects(root, p, d.Work)
	if !known {
		fmt.Printf("FAMILY_DIAG %s:240\n", owner)
		return nil
	}
	for _, object := range dependencyObjects {
		references, known := nativeMemberSourceBindingNames(object, d.Work)
		if !known {
			fmt.Printf("FAMILY_DIAG %s:245\n", owner)
			return nil
		}
		for _, n := range references {
			// A default-package class has no qualified spelling to escape a
			// same-named lexical member declaration. Retain the flat scope.
			if !strings.Contains(n, "/") && lexicalNames[n] {
				fmt.Printf("FAMILY_DIAG %s:251\n", owner)
				return nil
			}
			if p.children[n] != nil || n == owner {
				continue
			}
			raw, found := z.enumSiblingResolver()(n)
			if !found {
				continue
			}
			other, e := d.parseResolved(raw)
			if e != nil {
				fmt.Printf("FAMILY_DIAG %s:262\n", owner)
				return nil
			}
			otherOwner, _, otherFlags, isMember := originalMemberOwner(other)
			if isMember && otherOwner != owner {
				// Inherited and independently accessible member declarations
				// can both contribute names. Neither contributes ownership.
				static := otherFlags&8 != 0
				if !static && !nativeMemberLexicalAncestorDeclarationDependency(prepared, object, other, d.nativeAnnotationDeclarationResolver(), d.Work) && !nativeMemberIndependentDeclarationDependency(root, other, d.nativeAnnotationDeclarationResolver(), d.Work) && !nativeAnonymousForeignSuperDeclarationUse(p, other, d.Work) {
					fmt.Printf("FAMILY_DIAG %s:270\n", owner)
					return nil
				}
				// A terminal local refusal owns no source completion to wait on.
				// Its separately proved static declaration can keep the physical
				// spelling even if unrelated original type edges form a cycle.
				if static {
					if source := z.terminalFlatStaticDeclaration(prepared, other, peers...); source != "" {
						if p.sourceDependencies == nil {
							p.sourceDependencies = map[string]string{}
						}
						p.sourceDependencies[n] = source
						continue
					}
				}
				if !staticDependenciesChecked {
					if !dependencyGraphClosed {
						_, cyclic, known := z.nativeMemberDependencyAdmission(owner, d.Work, snap)
						if !known || cyclic {
							fmt.Printf("FAMILY_DIAG %s:288\n", owner)
							return nil
						}
					}
					staticDependenciesChecked = true
				}
				dependency := lookup(n)
				if dependency == nil && static {
					if source := z.completedFlatStaticDeclaration(prepared, other, peers...); source != "" {
						if p.sourceDependencies == nil {
							p.sourceDependencies = map[string]string{}
						}
						p.sourceDependencies[n] = source
						continue
					}
				}
				if dependency == nil || dependency.static != static || dependency.owner != otherOwner || dependency.object.GetClassName() != n || dependency.sourceName == "" {
					fmt.Printf("FAMILY_DIAG %s:304\n", owner)
					return nil
				}
				if p.sourceDependencies == nil {
					p.sourceDependencies = map[string]string{}
				}
				// A completed dependency contributes its source name, never
				// membership in this family's private/constructor access scope.
				p.sourceDependencies[n] = dependency.sourceName
				if !static {
					// Constructor metadata is a binding view, not lexical
					// ownership. Allocation proof separately refuses any
					// foreign private bridge protocol.
					if p.allocationDependencies == nil {
						p.allocationDependencies = map[string]*nativeMemberClass{}
					}
					p.allocationDependencies[n] = dependency
				}
			}
			if anonOwner, _, anon := originalAnonymousOwner(other); anon && (anonOwner == owner || p.children[anonOwner] != nil || p.enumConstants[anonOwner] != nil) {
				if body := p.enumConstants[n]; body != nil && body.owner == anonOwner {
					continue
				}
				if p.enumSwitchTables[n] != nil && anonOwner == owner {
					continue
				}
				if p.emptyMarkers[n] != nil && anonOwner == owner && nativeMemberEmptyAccessMarker(other, owner, d.Work) {
					continue
				}
				if group := p.anonymousUnits[n]; group == nil || group.owner != anonOwner {
					// A mixed lexical forest can compose an independently
					// certified flat tail without granting it private scope.
					// It is deliberately absent from anonymousUnits: those
					// units alone are regenerated and suppressed by javac.
					var independent *nativeAnonymousFamily
					if p.anonymousForest != nil {
						independent = p.anonymousForest.groups[anonOwner]
					} else if anonOwner == p.owner {
						independent = p.anonymous
					} else {
						independent = p.memberAnonymous[anonOwner]
					}
					if independent == nil || independent.owner != anonOwner || independent.standalone[n] == nil {
						fmt.Printf("FAMILY_DIAG %s:346\n", owner)
						return nil
					}
				}
			}
		}
	}

	if !nativeMemberExternalSupersClosed(p, d.nativeConstructorOperandMetadata(d.buildInvocationMetadata()), d.nativeAnnotationDeclarationResolver(), d.Work) {
		fmt.Printf("FAMILY_DIAG %s:354\n", owner)
		return nil
	}
	if !nativeAnonymousForeignSupersCommitted(p, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:357\n", owner)
		return nil
	}

	// Dependencies were unavailable during the initial ownership proof.
	// Recheck actual allocations with the completed foreign constructor
	// metadata before any body can publish a projected type spelling.
	for _, object := range dependencyObjects {
		if _, known := z.nativeMemberReader(object).nativeMemberAllocations(p); !known {
			fmt.Printf("FAMILY_DIAG %s:365\n", owner)
			return nil
		}
	}
	for name, object := range objects {
		if name == owner || p.children[name] != nil || p.anonymousUnits[name] != nil || p.enumConstants[name] != nil || p.methodLocals[name] != nil {
			continue
		}
		reader := z.nativeMemberReader(object)
		reader.options.EnvSnapshot = snap
		reader.nativeMemberRoot = p
		var err error
		jdecenv.Run(snap, func() error { _, err = reader.DumpClass(); return err })
		if err != nil || p.failed {
			fmt.Printf("FAMILY_DIAG %s:378\n", owner)
			return nil
		}
	}
	d.nativeMemberRoot = p
	d.nativeAnonymousRoot = p.anonymous
	var src string
	var e error
	jdecenv.Run(snap, func() error { src, e = d.DumpClass(); return e })
	if p.rootHandleSource != nil && !nativeMemberLambdaSourceClosed(p.rootHandleSource, d, d.Work) {
		fmt.Printf("FAMILY_DIAG %s:387\n", owner)
		return nil
	}
	if p.anonymousForest != nil && !p.anonymousForest.scopeSourceComplete(src) {
		if dir := os.Getenv("JDEC_FAMILY_DIAGNOSTIC_OUT"); dir != "" {
			os.WriteFile(filepath.Join(dir, "rejected-source.java"), []byte(src), 0600)
		}
		for name, g := range p.anonymousForest.groups {
			ord, known := nativeAnonymousOrdinalsWithinOwner(src, name)
			fmt.Printf("FOREST_DIAG %s group=%s ord=%v known=%v children=%d failed=%v prefix=%d\n", owner, name, ord, known, len(g.children), g.failed, g.enumPrefix)
		}
		fmt.Printf("FAMILY_DIAG %s:390\n", owner)
		return nil
	}
	if !nativeEnumConstantsSourceClosed(p) {
		fmt.Printf("FAMILY_DIAG %s:393\n", owner)
		return nil
	}
	if !nativeMethodLocalSourceComplete(p) {
		fmt.Printf("FAMILY_DIAG %s:396\n", owner)
		return nil
	}
	if e != nil || p.failed || !nativeEnumSwitchSourceComplete(p, src, d.Work) || !nativeMemberPrivateGetterSourceClosed(p, src, d.Work) || d.sourceDegraded || p.anonymous != nil && !p.anonymous.completeSource(src) {
		fmt.Printf("FAMILY_DIAG %s:399\n", owner)
		return nil
	}

	return &nativeMemberCacheEntry{family: p, source: src}
}
