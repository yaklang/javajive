package javaclassparser

import (
	"fmt"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// This is planning evidence only. Rendering must consume the proved original
// target successfully, and archive closure must exclude other physical users.
func nativeMemberLambdaImplementation(child *nativeMemberClass, method *MemberInfo, work *workbudget.Budget) bool {
	if child == nil || child.object == nil || method == nil || !nativeProofWork(work, 1) {
		return false
	}
	if len(child.object.Methods) > 4096 || !nativeProofWork(work, int64(len(child.object.Methods))) {
		return false
	}
	found := 0
	for _, original := range child.object.Methods {
		if original == method {
			found++
		}
	}
	if found != 1 {
		return false
	}
	for _, attr := range method.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		switch attr.(type) {
		case *CodeAttribute, *ExceptionsAttribute:
		default:
			return false
		}
	}
	if child.lambdaImplementations[method] {
		return true
	}
	if method.AccessFlags & ^uint16(0x100a) != 0 || method.AccessFlags&(0x1002) != 0x1002 || len(child.lambdaImplementations) >= 64 {
		return false
	}
	name, nok := sourceBridgeUTF8(child.object, method.NameIndex)
	desc, dok := sourceBridgeUTF8(child.object, method.DescriptorIndex)
	if child.lambdaContext.localCaptures == nil {
		child.lambdaContext.localCaptures = map[string]*nativeLambdaLocalCaptureSite{}
	}
	if !nok || !dok || !nativeLambdaImplementationScope(child.object, name, desc, "", false, work, child.lambdaContext) {
		return false
	}
	if child.lambdaImplementations == nil {
		child.lambdaImplementations = map[*MemberInfo]bool{}
	}
	child.lambdaImplementations[method] = true
	return true
}

// The archive's original type-use index includes CP method/handle owners. Any
// foreign symbolic reference to a consumed private implementation is refused,
// including unused CP entries: it cannot inherit this family's lookup scope.
func (z *JarFS) nativeMemberLambdaArchiveClosed(child *nativeMemberClass, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if child == nil || child.object == nil || index == nil || !index.valid {
		return false
	}
	if len(child.lambdaImplementations) == 0 {
		return true
	}
	owner := child.object.GetClassName()
	targets := map[[2]string]bool{}
	for method := range child.lambdaImplementations {
		if !nativeProofWork(work, 1) {
			return false
		}
		n, nok := sourceBridgeUTF8(child.object, method.NameIndex)
		d, dok := sourceBridgeUTF8(child.object, method.DescriptorIndex)
		if !nok || !dok {
			return false
		}
		targets[[2]string{n, d}] = true
	}
	users := index.typeUsers[owner]
	if len(users) > 4096 || !nativeProofWork(work, int64(len(users))) {
		return false
	}
	bytes := 0
	for user := range users {
		if user == owner {
			continue
		}
		raw, found := z.enumSiblingResolver()(user)
		if !found || len(raw) > (16<<20)-bytes || !nativeProofWork(work, int64(len(raw))) {
			return false
		}
		bytes += len(raw)
		reader := z.nativeMemberReader(nil)
		object, err := reader.parseResolved(raw)
		if err != nil || object.GetClassName() != user {
			return false
		}
		for _, cp := range object.ConstantPool {
			if !nativeProofWork(work, 1) {
				return false
			}
			member := nativeConstantMember(cp)
			if member == nil {
				continue
			}
			name, known := sourceBridgeClassName(object, member.ClassIndex)
			if !known {
				return false
			}
			if name != owner {
				continue
			}
			if member.NameAndTypeIndex == 0 || int(member.NameAndTypeIndex) > len(object.ConstantPool) {
				return false
			}
			nt, ok := object.ConstantPool[member.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			if !ok || nt == nil {
				return false
			}
			n, nok := sourceBridgeUTF8(object, nt.NameIndex)
			d, dok := sourceBridgeUTF8(object, nt.DescriptorIndex)
			if !nok || !dok || targets[[2]string{n, d}] {
				return false
			}
		}
	}
	return true
}

func nativeMemberLambdaSourceClosed(child *nativeMemberClass, dumper *ClassObjectDumper, work *workbudget.Budget) bool {
	if child == nil || dumper == nil || dumper.obj != child.object {
		return false
	}
	for method := range child.lambdaImplementations {
		if !nativeProofWork(work, 1) {
			return false
		}
		name, nok := sourceBridgeUTF8(child.object, method.NameIndex)
		desc, dok := sourceBridgeUTF8(child.object, method.DescriptorIndex)
		body := dumper.dumpedMethodsSet[fmt.Sprintf("name:%s,desc:%s", name, desc)]
		if !nok || !dok || !slices.Contains(dumper.lambdaMethods[name], desc) || body == nil || body.member != method || body.bodyCode == "stub" || body.checkedEscape || strings.Contains(body.code, DecompileStubMarker) {
			return false
		}
		if site := child.lambdaContext.localCaptures[name+desc]; site != nil && !nativeLambdaLocalCaptureSourceClosed(site, dumper.nativeLambdaLocalSources[name+desc], work) {
			return false
		}
	}
	return true
}

// Source lambdas need the original interface declaration, not merely a
// reference-shaped factory return. Keep a bounded, unambiguous erased SAM;
// intersection/covariant multi-descriptor families require their own proof.
type nativeLambdaImplementationContext struct {
	resolve       func(string) (*ClassObject, bool)
	metadata      callbinding.Provider
	localCaptures map[string]*nativeLambdaLocalCaptureSite
	// Only a completed original anonymous constructor packet may replace a
	// hidden captured field by its enclosing effectively-final declaration.
	anonymousCaptures *nativeAnonymousClass
}

func nativeLambdaFunctionalTarget(descriptor, name, sam string, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if resolve == nil || len(descriptor)+len(name)+len(sam) > 4096 || !nativeProofWork(work, int64(len(descriptor)+len(name)+len(sam))) || len(descriptor) < 3 || descriptor[0] != 'L' || descriptor[len(descriptor)-1] != ';' || name == "" {
		return false
	}
	object, known := resolve("java/lang/Object")
	if !known || object == nil || object.GetClassName() != "java/lang/Object" || len(object.Methods) > 4096 {
		return false
	}
	excluded := map[[2]string]bool{}
	for _, m := range object.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		if m.AccessFlags&1 == 0 || m.AccessFlags&8 != 0 {
			continue
		}
		n, nok := sourceBridgeUTF8(object, m.NameIndex)
		d, dok := sourceBridgeUTF8(object, m.DescriptorIndex)
		if !nok || !dok {
			return false
		}
		excluded[[2]string{n, d}] = true
	}
	type methodDeclaration struct {
		owner    string
		abstract bool
	}
	queue := []string{descriptor[1 : len(descriptor)-1]}
	objects := map[string]*ClassObject{}
	parents := map[string][]string{}
	declarations := map[[2]string][]methodDeclaration{}
	methods := 0
	for len(queue) > 0 {
		if !nativeProofWork(work, 1) || len(queue) > 256 {
			return false
		}
		current := queue[0]
		queue = queue[1:]
		if objects[current] != nil {
			continue
		}
		if len(objects) >= 64 {
			return false
		}
		o, ok := resolve(current)
		if !ok || o == nil || o.GetClassName() != current || o.AccessFlags&0x2200 != 0x200 || len(o.Interfaces) > 64 || len(o.Methods) > 4096-methods {
			return false
		}
		objects[current] = o
		methods += len(o.Methods)
		if work != nil && work.CheckAlloc(int64(methods+len(objects)+len(queue))*160) != nil {
			return false
		}
		for _, m := range o.Methods {
			if m == nil || !nativeProofWork(work, 1) {
				return false
			}
			if m.AccessFlags&8 != 0 || m.AccessFlags&2 != 0 {
				continue
			}
			n, nok := sourceBridgeUTF8(o, m.NameIndex)
			d, dok := sourceBridgeUTF8(o, m.DescriptorIndex)
			if !nok || !dok || m.AccessFlags&7 != 1 {
				return false
			}
			if _, _, e := callbinding.Descriptor(d); e != nil {
				return false
			}
			key := [2]string{n, d}
			if excluded[key] {
				continue
			}
			for _, old := range declarations[key] {
				if old.owner == current {
					return false
				}
			}
			declarations[key] = append(declarations[key], methodDeclaration{current, m.AccessFlags&0x0400 != 0})
		}
		for _, parent := range o.Interfaces {
			if !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(queue)+1)*32) != nil {
				return false
			}
			n, ok := sourceBridgeClassName(o, parent)
			if !ok {
				return false
			}
			parents[current] = append(parents[current], n)
			queue = append(queue, n)
		}
	}
	// Resolve maximally specific declarations in the original interface DAG.
	// A default removes an inherited abstract method; a subinterface can
	// reabstract that default. Neither traversal order nor method names decide.
	colors := map[string]uint8{}
	var acyclic func(string) bool
	acyclic = func(n string) bool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if colors[n] == 1 {
			return false
		}
		if colors[n] == 2 {
			return true
		}
		colors[n] = 1
		for _, parent := range parents[n] {
			if !acyclic(parent) {
				return false
			}
		}
		colors[n] = 2
		return true
	}
	if !acyclic(descriptor[1 : len(descriptor)-1]) {
		return false
	}
	remaining := 8192
	reaches := func(from, to string) (bool, bool) {
		seen := map[string]bool{}
		pending := []string{from}
		for len(pending) > 0 {
			remaining--
			if remaining < 0 || !nativeProofWork(work, 1) {
				return false, false
			}
			n := pending[0]
			pending = pending[1:]
			if n == to {
				return true, true
			}
			if seen[n] {
				continue
			}
			seen[n] = true
			pending = append(pending, parents[n]...)
		}
		return false, true
	}
	abstract := map[[2]string]bool{}
	for key, decls := range declarations {
		var maximal []methodDeclaration
		for i, decl := range decls {
			shadowed := false
			for j, other := range decls {
				if i == j {
					continue
				}
				yes, known := reaches(other.owner, decl.owner)
				if !known {
					return false
				}
				if yes {
					shadowed = true
					break
				}
			}
			if !shadowed {
				maximal = append(maximal, decl)
			}
		}
		defaults := 0
		for _, decl := range maximal {
			if !decl.abstract {
				defaults++
			}
		}
		if defaults != 0 {
			if len(maximal) != 1 {
				return false
			}
			continue
		}
		if len(maximal) == 0 {
			return false
		}
		abstract[key] = true
	}
	return len(abstract) == 1 && abstract[[2]string{name, sam}]
}
