package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
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
	jar      *JarFS
	mu       sync.Mutex
	plans    map[string]*sourceLambdaMetadataPlan
	returned map[string][32]byte
	bytes    int64
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
	plan := a.plans[owner]
	if plan == nil || a.returned[owner] != plan.sourceHash {
		return nil
	}
	return plan
}

// DumpClass can run inside an unpublished family transaction. Its local plan
// becomes usable only when this exact complete source is returned through the
// artifact's public source view. Failed/provisional layouts confer no edits.
func (a *sourceLambdaMetadataAuthority) sourceReturned(owner string, source []byte) {
	hash := sha256.Sum256(source)
	a.mu.Lock()
	defer a.mu.Unlock()
	if plan := a.plans[owner]; plan != nil && plan.sourceHash == hash {
		if a.returned == nil {
			a.returned = map[string][32]byte{}
		}
		a.returned[owner] = hash
	}
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
	if !c.artifactLambdaMetadataAllowed() || method.AccessFlags != 0x100a {
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
	if !known || !nativeLambdaSourceIdentityOrderClosed(events) {
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
		return prior.originalHash == plan.originalHash && prior.sourceHash == plan.sourceHash && slices.Equal(prior.methods, plan.methods)
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
