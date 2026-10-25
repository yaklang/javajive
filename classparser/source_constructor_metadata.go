package javaclassparser

import (
	"crypto/sha256"
	"fmt"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// This obligation is independent of lambda metadata. The selected modern
// compiler emits unnamed implicit-constructor rows and an empty declared-type
// Signature that native javac8 can omit.
// Only an actually rendered, all-hidden anonymous constructor can enroll it.
// Explicit SUPER arguments, shared ports and ordinary constructors grant none.
type sourceConstructorMetadataPlan struct {
	owner, descriptor string
	sourceMajor       uint16
	originalHash      [32]byte
	flags             []uint16
	omitParameters    bool
	omitSignature     bool
}

type sourceConstructorMemberPublication struct {
	group       *nativeAnonymousFamily
	child       *nativeAnonymousClass
	plan        *sourceConstructorMetadataPlan
	anonymous   *sourceAnonymousClassMetadataPlan
	publication *sourceLambdaPublication
}

func sourceConstructorMetadataPlansEqual(left, right *sourceConstructorMetadataPlan) bool {
	return left != nil && right != nil && left.owner == right.owner && left.descriptor == right.descriptor && left.originalHash == right.originalHash && left.sourceMajor == right.sourceMajor && left.omitParameters == right.omitParameters && left.omitSignature == right.omitSignature && slices.Equal(left.flags, right.flags)
}

func (a *sourceLambdaMetadataAuthority) constructorPlan(owner string) *sourceConstructorMetadataPlan {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.constructors[owner]
}

// All-hidden constructor metadata has no dependency on the Java8 lambda
// naming protocol. It grants only a completed anonymous allocation's attribute
// obligation; lambda names, static-anonymous flags and executable lowering keep
// their separate permissions. The actual original bytes bind every plan.
func (c *ClassObjectDumper) artifactConstructorMetadataAllowed() bool {
	if c == nil || c.obj == nil || c.obj.MajorVersion < 49 || c.obj.MajorVersion > 52 || c.obj.MinorVersion != 0 || c.options.SourceCompiler.effective() != ModernJavac || c.options.TargetSourceVersion < 8 || c.options.TargetSourceVersion > 21 || c.options.sourceLambdaMetadata == nil || hasOriginalNestAttribute(c.obj, "NestHost") || hasOriginalNestAttribute(c.obj, "NestMembers") {
		return false
	}
	_, bound := c.options.sourceLambdaMetadata.original(c.obj, c.Work)
	return bound
}

func (plan *sourceConstructorMetadataPlan) targetMatches(original, fresh *ClassObject, target sourceCaptureMetadataTarget) bool {
	if plan == nil || original == nil || fresh == nil {
		return false
	}
	major := plan.sourceMajor
	if major == 0 {
		// Existing standalone restoration retains its Java8-only permission.
		return target.matches(original, fresh)
	}
	if target.major != 0 {
		// The physical MR9/10 compiler can recompile an emitted base8 unit.
		// This permission does not authorize arbitrary target substitution.
		if major != 52 || target.major < 53 || target.major > 54 {
			return false
		}
		major = target.major
	}
	return original.MajorVersion >= 49 && original.MajorVersion <= 52 && original.MinorVersion == 0 && !hasOriginalNestAttribute(original, "NestHost") && !hasOriginalNestAttribute(original, "NestMembers") && major >= 52 && major <= 65 && fresh.MajorVersion == major && fresh.MinorVersion == 0 && (major >= 55 || !hasOriginalNestAttribute(fresh, "NestHost") && !hasOriginalNestAttribute(fresh, "NestMembers"))
}

func (c *ClassObjectDumper) artifactConstructorPlan(child *nativeAnonymousClass) *sourceConstructorMetadataPlan {
	if child == nil || child.object == nil || !c.artifactConstructorMetadataAllowed() || child.object.MajorVersion < 49 || child.object.MajorVersion > 52 || child.object.MinorVersion != 0 || hasOriginalNestAttribute(child.object, "NestHost") || hasOriginalNestAttribute(child.object, "NestMembers") || child.sharedParameterRoles || len(child.superParams) != 0 {
		return nil
	}
	params, result, err := callbinding.Descriptor(child.descriptor)
	if err != nil || result != "V" || len(params) == 0 || len(params) > 255 {
		return nil
	}
	plan := &sourceConstructorMetadataPlan{sourceMajor: uint16(c.options.TargetSourceVersion + 44), owner: child.object.GetClassName(), descriptor: child.descriptor, flags: make([]uint16, len(params))}
	for field, index := range child.fields {
		if index < 0 || index >= len(params) || plan.flags[index] != 0 || !nativeProofWork(c.Work, 1) {
			return nil
		}
		plan.flags[index] = 0x1010 // final captured local
		if field == child.enclosingField {
			plan.flags[index] = 0x8010 // final mandated enclosing instance
		}
	}
	if child.unusedEnclosing && plan.flags[0] == 0 {
		plan.flags[0] = 0x8010
	}
	for _, flags := range plan.flags {
		if flags == 0 {
			return nil // A source/SUPER parameter has no hidden-capture role.
		}
	}
	constructors := 0
	for _, method := range child.object.Methods {
		name, known := sourceBridgeUTF8(child.object, method.NameIndex)
		if !known || !nativeProofWork(c.Work, 1) {
			return nil
		}
		if name != "<init>" {
			continue
		}
		constructors++
		descriptor, typed := sourceBridgeUTF8(child.object, method.DescriptorIndex)
		_, _, present, valid := sourceMethodParameterTable(child.object, method)
		signature, signaturePresent, signatureValid := sourceConstructorSignature(child.object, method)
		if !typed || descriptor != child.descriptor || !valid || !signatureValid || signaturePresent && signature != "()V" {
			return nil
		}
		plan.omitParameters = !present
		// All ordinary captured ports are absent from the implicit source
		// constructor's declared type. Modern javac encodes that empty type
		// as ()V; native javac8 can omit this Signature attribute entirely.
		for _, flags := range plan.flags {
			plan.omitSignature = plan.omitSignature || !signaturePresent && flags == 0x1010
		}
	}
	if constructors != 1 || !plan.omitParameters && !plan.omitSignature {
		return nil
	}
	raw, bound := c.options.sourceLambdaMetadata.original(child.object, c.Work)
	if !bound {
		return nil
	}
	plan.originalHash = sha256.Sum256(raw)
	return plan
}

// Called at the successful typed allocation renderer, after capture, SUPER,
// lexical access and body proofs. A provisional body alone cannot activate it.
func (c *ClassObjectDumper) recordArtifactConstructorPublication(group *nativeAnonymousFamily, child *nativeAnonymousClass, sub *ClassObjectDumper, source string) bool {
	metadataAllowed := c.artifactConstructorMetadataAllowed()
	// A modern anonymous declaration can own a member of a reciprocal nest
	// without owing legacy implicit-constructor metadata. Transport only its
	// completed source origin through the same typed allocation/body proof.
	// This grants no additional constructor, lambda or class-flag rewriting.
	nestAllowed := c != nil && c.obj != nil && c.obj.MajorVersion >= 55 && c.obj.MajorVersion <= 65 && c.obj.MinorVersion == 0 && c.options.SourceCompiler.effective() == ModernJavac && c.options.TargetSourceVersion >= 11 && c.options.TargetSourceVersion <= 21 && c.options.sourceLambdaMetadata != nil && c.privateNestCompletionAuthority() != nil
	if !metadataAllowed && !nestAllowed {
		return true
	}
	if group == nil || group != c.nativeAnonymousRoot || group.owner != c.obj.GetClassName() || child == nil || child.object == nil || group.children[child.object.GetClassName()] != child || sub == nil || sub.obj != child.object {
		return false
	}
	var plan *sourceConstructorMetadataPlan
	var anonymous *sourceAnonymousClassMetadataPlan
	if metadataAllowed {
		plan = c.artifactConstructorPlan(child)
		anonymous = c.artifactAnonymousClassPlan(child)
	}
	if child.artifactStaticMetadata && anonymous == nil {
		return false
	}
	publication := c.options.sourceLambdaMetadata.publication(child.object.GetClassName(), source)
	if !metadataAllowed {
		if publication == nil || len(publication.nests) == 0 {
			return c.Work.Check() == nil
		}
		if len(publication.plans)+len(publication.constructors)+len(publication.anonymous)+len(publication.enums) != 0 {
			return false
		}
	}
	if plan == nil && anonymous == nil && publication == nil {
		return c.Work.Check() == nil
	}
	for _, prior := range c.sourceConstructorMembers {
		if prior.group == group && prior.child == child {
			return (plan == nil && prior.plan == nil || sourceConstructorMetadataPlansEqual(prior.plan, plan)) && sourceAnonymousMetadataPlansEqual(prior.anonymous, anonymous) && prior.publication == publication
		}
	}
	if len(c.sourceConstructorMembers) >= 4096 || !nativeProofWork(c.Work, 1) || c.Work.CheckAlloc(int64(len(c.sourceConstructorMembers)+1)*256) != nil {
		return false
	}
	c.sourceConstructorMembers = append(c.sourceConstructorMembers, sourceConstructorMemberPublication{group: group, child: child, plan: plan, anonymous: anonymous, publication: publication})
	return true
}

func (c *ClassObjectDumper) artifactConstructorPublicationClosed(member sourceConstructorMemberPublication, source string) bool {
	group, child := member.group, member.child
	if group == nil || group != c.nativeAnonymousRoot || group.owner != c.obj.GetClassName() || group.failed || child == nil || child.object == nil || group.children[child.object.GetClassName()] != child || !nativeProofWork(c.Work, int64(len(source))+1) || !group.completeOwnSource(source) {
		return false
	}
	// The existing lexical marker scan proves that every actual anonymous
	// allocation occurs once in this completed owning source, in ordinal order.
	// Text never invents an allocation role: the exact typed child must survive.
	if member.plan != nil && !sourceConstructorMetadataPlansEqual(member.plan, c.artifactConstructorPlan(child)) {
		return false
	}
	if !sourceAnonymousMetadataPlansEqual(member.anonymous, c.artifactAnonymousClassPlan(child)) {
		return false
	}
	if nested := member.publication; nested != nil {
		raw, bound := c.options.sourceLambdaMetadata.original(child.object, c.Work)
		if !bound || nested.owner != child.object.GetClassName() || sha256.Sum256(raw) != nested.originalHash {
			return false
		}
	}
	return member.plan != nil || member.anonymous != nil || member.publication != nil
}

// Restore the absence of compiler-generated all-hidden metadata, with its
// complete descriptor/role schema checked first. Attribute absence is a
// reflective contract too. No instruction, CP index or other attribute byte
// is rewritten; a Signature can be removed only for the empty source type ()V.
func restoreImplicitConstructorParameterMetadata(original, compiled []byte, plan *sourceConstructorMetadataPlan, work *workbudget.Budget) ([]byte, error) {
	return restoreImplicitConstructorParameterMetadataAtTarget(original, compiled, plan, sourceCaptureMetadataTarget{}, work)
}

func restoreImplicitConstructorParameterMetadataAtTarget(original, compiled []byte, plan *sourceConstructorMetadataPlan, target sourceCaptureMetadataTarget, work *workbudget.Budget) ([]byte, error) {
	fail := func(reason string) ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("constructor metadata: %s", reason)
	}
	if plan == nil || len(plan.flags) == 0 || len(plan.flags) > 255 || len(compiled) > 32<<20 || sha256.Sum256(original) != plan.originalHash || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return fail("missing or stale implicit-port proof")
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
	if want.GetClassName() != plan.owner || got.GetClassName() != plan.owner || !plan.targetMatches(want, got, target) || want.AccessFlags != got.AccessFlags || want.GetSupperClassName() != got.GetSupperClassName() || !slices.Equal(want.GetInterfacesName(), got.GetInterfacesName()) || len(want.Methods) != len(got.Methods) || len(got.Methods) > 4096 {
		return fail("class or constructor universe changed")
	}
	find := func(object *ClassObject) (*MemberInfo, bool) {
		var ctor *MemberInfo
		for _, method := range object.Methods {
			name, known := sourceBridgeUTF8(object, method.NameIndex)
			if !known || !nativeProofWork(work, 1) {
				return nil, false
			}
			if name == "<init>" {
				descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
				if ctor != nil || !typed || descriptor != plan.descriptor {
					return nil, false
				}
				ctor = method
			}
		}
		return ctor, ctor != nil
	}
	before, bk := find(want)
	after, ak := find(got)
	if !bk || !ak || before.AccessFlags != after.AccessFlags {
		return fail("constructor identity or flags changed")
	}
	originalNames, originalFlags, originalTable, valid := sourceMethodParameterTable(want, before)
	names, flags, generatedTable, generatedValid := sourceMethodParameterTable(got, after)
	textWork := int64(0)
	params, result, descriptorError := callbinding.Descriptor(plan.descriptor)
	if !valid || plan.omitParameters == originalTable || !generatedValid || !sourceMethodMetadataTextWork(work, &textWork, originalNames, names) || descriptorError != nil || result != "V" || len(params) != len(plan.flags) {
		return fail("invalid original table-absence contract")
	}
	for _, flag := range plan.flags {
		if flag != 0x1010 && flag != 0x8010 {
			return fail("unknown implicit parameter role")
		}
	}
	removeParameters := plan.omitParameters && generatedTable
	if removeParameters {
		if !slices.Equal(flags, plan.flags) || len(names) != len(plan.flags) {
			return fail("generated implicit parameter flags changed")
		}
		for _, name := range names {
			if name != "" {
				return fail("a named parameter is outside the unnamed implicit contract")
			}
		}
	} else if originalTable != generatedTable || !slices.Equal(originalNames, names) || !slices.Equal(originalFlags, flags) {
		return fail("unrelated original parameter table changed")
	}
	wantSignature, wantSignaturePresent, wantSignatureValid := sourceConstructorSignature(want, before)
	gotSignature, gotSignaturePresent, gotSignatureValid := sourceConstructorSignature(got, after)
	if !wantSignatureValid || !gotSignatureValid || plan.omitSignature && wantSignaturePresent {
		return fail("invalid original constructor signature contract")
	}
	removeSignature := plan.omitSignature && gotSignaturePresent
	if removeSignature {
		if gotSignature != "()V" {
			return fail("generated signature is not the proved empty source constructor")
		}
	} else if wantSignaturePresent != gotSignaturePresent || wantSignature != gotSignature {
		return fail("unrelated constructor signature changed")
	}
	if !removeSignature && !removeParameters {
		return applyCompiledMetadataEdits(compiled, nil, work)
	}
	namesToRemove := map[string]bool{}
	if removeParameters {
		namesToRemove["MethodParameters"] = true
	}
	if removeSignature {
		namesToRemove["Signature"] = true
	}
	edit, err := compiledMethodWithoutAttributes(compiled, got, after, layout, namesToRemove, work)
	if err != nil {
		return nil, err
	}
	return applyCompiledMetadataEdits(compiled, []compiledMetadataEdit{edit}, work)
}

func sourceConstructorSignature(object *ClassObject, method *MemberInfo) (string, bool, bool) {
	text, present := "", false
	for _, attribute := range method.Attributes {
		if signature, ok := attribute.(*SignatureAttribute); ok {
			if signature == nil || present {
				return "", false, false
			}
			var known bool
			text, known = sourceBridgeUTF8(object, signature.SignatureIndex)
			if !known || text == "" {
				return "", false, false
			}
			present = true
		}
	}
	return text, present, true
}
