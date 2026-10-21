package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Only an explicit source artifact owns this capability. A declaration resolver
// cannot enroll bytes, and each physical release owns a separate ledger.
type sourceLambdaMetadataAuthority struct {
	jar          *JarFS
	mu           sync.Mutex
	plans        map[string]*sourceLambdaMetadataPlan
	active       map[string]*sourceLambdaMetadataPlan
	publications map[string]*sourceLambdaPublication
	bytes        int64
}

func (a *sourceLambdaMetadataAuthority) original(object *ClassObject, work *workbudget.Budget) ([]byte, bool) {
	if a == nil || a.jar == nil || object == nil || !nativeSourceBinaryName(object.GetClassName()) {
		return nil, false
	}
	raw, err := a.jar.readJarEntry(object.GetClassName() + ".class")
	if err != nil || len(raw) > 16<<20 || !nativeProofWork(work, int64(len(raw))*2+1) || work != nil && work.CheckAlloc(int64(len(raw))*4) != nil {
		return nil, false
	}
	original, err := ParseWithBudget(raw, work)
	return raw, err == nil && bytes.Equal(original.Bytes(), object.Bytes())
}

func (a *sourceLambdaMetadataAuthority) plan(owner string) *sourceLambdaMetadataPlan {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active[owner]
}

// DumpClass can run inside an unpublished family transaction. Its local plan
// becomes usable only when this exact complete source is returned through the
// artifact's public source view. Failed/provisional layouts confer no edits.
func (a *sourceLambdaMetadataAuthority) sourceReturned(owner string, source []byte) error {
	hash := sha256.Sum256(source)
	a.mu.Lock()
	defer a.mu.Unlock()
	publication := a.publications[owner]
	if publication == nil || publication.sourceHash != hash {
		return nil
	}
	for _, plan := range publication.plans {
		if prior := a.active[plan.owner]; prior != nil && (prior.originalHash != plan.originalHash || !slices.Equal(prior.methods, plan.methods)) {
			return fmt.Errorf("lambda metadata: conflicting completed source variants require a fresh artifact")
		}
	}
	if a.active == nil {
		a.active = map[string]*sourceLambdaMetadataPlan{}
	}
	for _, plan := range publication.plans {
		a.active[plan.owner] = plan
	}
	return nil
}

func (c *ClassObjectDumper) artifactLambdaMetadataAllowed() bool {
	if c == nil || c.obj == nil || c.obj.MajorVersion != 52 || c.options.SourceCompiler.effective() != ModernJavac || c.options.TargetSourceVersion != 8 || c.options.sourceLambdaMetadata == nil {
		return false
	}
	_, known := c.options.sourceLambdaMetadata.original(c.obj, c.Work)
	return known
}

// Original and generated spellings are separate constraints. The alternate
// component is accepted only with an explicit obligation; the final source
// still proves actual factory ancestry, cardinality and registration order.
func (c *ClassObjectDumper) originalLambdaFactoryNameComponent(factory *lambdaSourceFactory, method *MemberInfo) (string, bool) {
	component, known := c.lambdaSourceFactoryNameComponent(factory)
	if !known || method == nil {
		return "", false
	}
	name, named := sourceBridgeUTF8(c.obj, method.NameIndex)
	if named && strings.HasPrefix(name, "lambda$"+component+"$") {
		return component, true
	}
	if !c.artifactLambdaMetadataAllowed() || !sourceLambdaImplementationFlags(method.AccessFlags) {
		return component, known
	}
	native, valid := lambdaFactoryNameComponent(factory)
	return native, valid && named && strings.HasPrefix(name, "lambda$"+native+"$")
}

func (c *ClassObjectDumper) publishArtifactLambdaMetadata(source string, members []string, methods []*dumpedMethods) bool {
	if c == nil || c.options.sourceLambdaMetadata == nil || len(c.lambdaMethodSourceBodies) == 0 {
		return true
	}
	if !c.artifactLambdaMetadataAllowed() {
		// The existing matching-compiler plain-source contract is unchanged.
		return true
	}
	events, known := c.native8LambdaSourceIdentityEvents(source, members, methods)
	if !known {
		return false
	}
	plan := &sourceLambdaMetadataPlan{owner: c.obj.GetClassName(), sourceHash: sha256.Sum256([]byte(source))}
	needed := false
	shared := false
	seen := map[*MemberInfo]bool{}
	for ordinal, event := range events {
		body := event.body
		if body == nil || body.member == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		receipt := c.lambdaMethodSourceBodies[body.member]
		if receipt == nil || receipt.body != body {
			return false
		}
		component, named := c.lambdaSourceFactoryNameComponent(receipt.request.factory)
		name, nk := sourceBridgeUTF8(c.obj, body.member.NameIndex)
		descriptor, dk := sourceBridgeUTF8(c.obj, body.member.DescriptorIndex)
		if !named || !nk || !dk {
			return false
		}
		generated := "lambda$" + component + "$" + strconv.Itoa(ordinal)
		_, _, table, valid := sourceMethodParameterTable(c.obj, body.member)
		if !valid {
			return false
		}
		needed = needed || name != generated || table
		if seen[body.member] {
			// Shared factories can compile to a different helper universe.
			// No per-event renaming can manufacture a missing implementation.
			shared = true
			continue
		}
		seen[body.member] = true
		plan.methods = append(plan.methods, sourceLambdaMetadataMethod{sourceLambdaMethodIdentity{name, descriptor}, sourceLambdaMethodIdentity{generated, descriptor}})
	}
	if !needed {
		return true
	}
	if shared || c.nativeMethodLocalCurrent != nil || c.nativeAnonymousRoot != nil && c.nativeAnonymousRoot.owner != c.obj.GetClassName() || c.nativeEnumConstantCurrent != nil || len(seen) != len(c.lambdaMethodSourceBodies) {
		return false
	}
	for method := range seen {
		metadata, known := readLambdaParameterMetadata(c.obj, method, c.Work)
		if !known || !sourceLambdaOrdinaryFactoryOnly(c.obj, method, metadata, c.Work) {
			return false
		}
	}
	authority := c.options.sourceLambdaMetadata
	if !authority.enclosingMethodReferencesClosed(c.obj, plan.methods, c.Work) {
		return false
	}
	raw, bound := authority.original(c.obj, c.Work)
	if !bound {
		return false
	}
	plan.originalHash = sha256.Sum256(raw)
	size := int64(256 + 2*len(plan.owner))
	for _, method := range plan.methods {
		size += int64(len(method.original.name)+len(method.generated.name)+2*len(method.original.descriptor)) + 64
	}
	if c.Work != nil && c.Work.CheckAlloc(size) != nil {
		return false
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if prior := authority.plans[plan.owner]; prior != nil {
		if prior.originalHash != plan.originalHash {
			return false
		}
		if prior.sourceHash == plan.sourceHash {
			return slices.Equal(prior.methods, plan.methods)
		}
	}
	if len(authority.plans) >= 4096 || size > (16<<20)-authority.bytes {
		return false
	}
	if authority.plans == nil {
		authority.plans = map[string]*sourceLambdaMetadataPlan{}
	}
	authority.plans[plan.owner] = plan
	authority.bytes += size
	return true
}

// These bindings come only from the actual successful named-member renderer.
// Complete packet equality reopens that typed ownership proof at the final
// source boundary; a matching substring by itself never enrolls a class.
type sourceLambdaMemberPublication struct {
	object      *ClassObject
	packet      string
	publication *sourceLambdaPublication
}

type sourceLambdaPublication struct {
	owner                    string
	originalHash, sourceHash [32]byte
	plans                    []*sourceLambdaMetadataPlan
}

func (a *sourceLambdaMetadataAuthority) publication(owner, source string) *sourceLambdaPublication {
	hash := sha256.Sum256([]byte(source))
	a.mu.Lock()
	defer a.mu.Unlock()
	publication := a.publications[owner]
	if publication == nil || publication.sourceHash != hash {
		return nil
	}
	return publication
}

func (c *ClassObjectDumper) sealArtifactLambdaPublication(source string, members []string) bool {
	if c == nil || c.options.sourceLambdaMetadata == nil {
		return true
	}
	a := c.options.sourceLambdaMetadata
	hash := sha256.Sum256([]byte(source))
	a.mu.Lock()
	own := a.plans[c.obj.GetClassName()]
	a.mu.Unlock()
	if own != nil && own.sourceHash != hash {
		own = nil
	}
	if own == nil && len(c.sourceLambdaMembers) == 0 {
		return true
	}
	if !c.artifactLambdaMetadataAllowed() || len(c.sourceLambdaMembers) > 4096 || len(members) != len(c.nativeRenderedMemberNames) {
		return false
	}
	publication := &sourceLambdaPublication{owner: c.obj.GetClassName(), sourceHash: hash}
	seen := map[string]bool{}
	add := func(plan *sourceLambdaMetadataPlan) bool {
		if plan == nil || seen[plan.owner] || len(publication.plans) >= 4096 || !nativeProofWork(c.Work, 1) {
			return false
		}
		seen[plan.owner] = true
		publication.plans = append(publication.plans, plan)
		return true
	}
	if own != nil && !add(own) {
		return false
	}
	bytes := int64(0)
	for _, member := range c.sourceLambdaMembers {
		if member.object == nil || member.publication == nil || c.nativeMemberRoot == nil {
			return false
		}
		name := member.object.GetClassName()
		child := c.nativeMemberRoot.children[name]
		if child == nil || child.object != member.object || child.owner != c.obj.GetClassName() || member.publication.owner != name {
			return false
		}
		matches := 0
		for i, packet := range members {
			if c.nativeRenderedMemberNames[i] == name && packet == member.packet {
				matches++
			}
		}
		cost := int64(len(source)+len(member.packet)) + 1
		if matches != 1 || cost > (128<<20)-bytes || !nativeProofWork(c.Work, cost) {
			return false
		}
		bytes += cost
		start := strings.Index(source, member.packet)
		if start < 0 || strings.Contains(source[start+len(member.packet):], member.packet) {
			return false
		}
		raw, bound := a.original(member.object, c.Work)
		if !bound || sha256.Sum256(raw) != member.publication.originalHash {
			return false
		}
		for _, plan := range member.publication.plans {
			if !add(plan) {
				return false
			}
		}
	}
	raw, bound := a.original(c.obj, c.Work)
	if !bound {
		return false
	}
	publication.originalHash = sha256.Sum256(raw)
	size := int64(128 + len(publication.owner) + len(publication.plans)*16)
	if c.Work != nil && c.Work.CheckAlloc(size) != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if prior := a.publications[publication.owner]; prior != nil && prior.originalHash == publication.originalHash && prior.sourceHash == publication.sourceHash && slices.Equal(prior.plans, publication.plans) {
		// The typed/source checks above still run on a cache hit. Reusing the
		// same immutable publication must not spend retained-memory allowance.
		return true
	}
	if size > (16<<20)-a.bytes || len(a.publications) >= 4096 {
		return false
	}
	if a.publications == nil {
		a.publications = map[string]*sourceLambdaPublication{}
	}
	a.publications[publication.owner] = publication
	a.bytes += size
	return true
}

// A child class may name its lexical parent in EnclosingMethod even though
// there is no method invocation. Renaming only this owner cannot repair that
// other class. Keep such artifacts outside this per-class metadata contract.
// Rows identify actual archive bytes; a resolver or dollar-name guess cannot
// supply cross-class edit authority.
func (a *sourceLambdaMetadataAuthority) enclosingMethodReferencesClosed(object *ClassObject, methods []sourceLambdaMetadataMethod, work *workbudget.Budget) bool {
	renamed := map[sourceLambdaMethodIdentity]bool{}
	for _, method := range methods {
		if method.original != method.generated {
			renamed[method.original] = true
		}
	}
	if len(renamed) == 0 {
		return true
	}
	if a == nil || a.jar == nil || object == nil {
		return false
	}
	total, rows := 0, 0
	for _, attribute := range object.Attributes {
		table, ok := attribute.(*InnerClassesAttribute)
		if !ok {
			continue
		}
		if table == nil || len(table.Classes) > 4096-rows {
			return false
		}
		rows += len(table.Classes)
		for _, row := range table.Classes {
			if row == nil || !nativeProofWork(work, 1) {
				return false
			}
			name, known := sourceBridgeClassName(object, row.InnerClassInfoIndex)
			if !known || !nativeSourceBinaryName(name) {
				return false
			}
			raw, err := a.jar.readJarEntry(name + ".class")
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil || len(raw) > (16<<20)-total || !nativeProofWork(work, int64(len(raw))+1) {
				return false
			}
			total += len(raw)
			child, err := ParseWithBudget(raw, work)
			if err != nil || child.GetClassName() != name {
				return false
			}
			for _, attribute := range child.Attributes {
				enclosing, ok := attribute.(*UnparsedAttribute)
				if !ok || enclosing == nil || enclosing.Name != "EnclosingMethod" {
					continue
				}
				if len(enclosing.Info) != 4 || enclosing.Length != 4 {
					return false
				}
				owner, known := sourceBridgeClassName(child, binary.BigEndian.Uint16(enclosing.Info))
				if !known {
					return false
				}
				index := binary.BigEndian.Uint16(enclosing.Info[2:])
				if owner != object.GetClassName() || index == 0 {
					continue
				}
				if int(index) > len(child.ConstantPool) {
					return false
				}
				nt, ok := child.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
				if !ok || nt == nil {
					return false
				}
				n, nk := sourceBridgeUTF8(child, nt.NameIndex)
				d, dk := sourceBridgeUTF8(child, nt.DescriptorIndex)
				if !nk || !dk || renamed[sourceLambdaMethodIdentity{n, d}] {
					return false
				}
			}
		}
	}
	return true
}
