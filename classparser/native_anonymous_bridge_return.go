package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// The erased interface result and the source result need not be identical.
// JLS 8.4.5 permits a reference subtype result. The preceding same-erasure
// certificate is preserved. Distinct erasures use only nominal raw reference
// and array widening; parameterized results need a separate substitution proof.
func nativeAnonymousBridgeReturnSubstitutable(instantiated types.JavaType, implementation string, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if instantiated == nil || resolve == nil || len(implementation) > 4096 || !nativeProofWork(work, int64(len(implementation))+1) {
		return false
	}
	formal := values.ReferenceTypeDescriptor(instantiated, nil)
	if len(formal) > 4096 {
		return false
	}
	if _, _, e := callbinding.Descriptor("()" + formal); e != nil {
		return false
	}
	if _, _, e := callbinding.Descriptor("()" + implementation); e != nil {
		return false
	}
	if !callbinding.Reference(implementation) || !callbinding.Reference(formal) {
		return false
	}
	if formal == implementation {
		return true
	}
	// Parameterized source results need a separate invariant-argument proof.
	view := instantiated
	for depth := 0; ; depth++ {
		if view == nil || depth > 255 || !nativeProofWork(work, 1) {
			return false
		}
		if _, p := types.AsParameterizedType(view); p || types.IsWildcardType(view) {
			return false
		}
		if !view.IsArray() {
			break
		}
		view = view.ElementType()
	}
	if _, _, e := callbinding.Descriptor("()" + implementation); e != nil {
		return false
	}
	if _, _, e := callbinding.Descriptor("()" + formal); e != nil {
		return false
	}
	if !callbinding.Reference(implementation) || !callbinding.Reference(formal) {
		return false
	}
	originals := map[string]*ClassObject{}
	known := map[string]bool{}
	stable := func(name string) (*ClassObject, bool) {
		if seen, ok := known[name]; ok {
			return originals[name], seen
		}
		if len(known) >= 64 || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(known)+1)*256) != nil {
			return nil, false
		}
		o, ok := resolve(name)
		if ok && o != nil {
			n, nk := sourceBridgeClassName(o, o.ThisClass)
			ok = nk && n == name
		} else {
			ok = false
		}
		known[name] = ok
		if ok {
			originals[name] = o
		}
		return o, ok
	}
	query := newConstructorWideningQuery(func(name string) (callbinding.Class, bool) {
		if !nativeSourceBinaryName(name) || !nativeProofWork(work, 1) {
			return callbinding.Class{}, false
		}
		o, ok := stable(name)
		if !ok || o == nil || len(o.Interfaces) > 63 {
			return callbinding.Class{}, false
		}
		// The three array-lattice roots are language intrinsics. Their
		// original declarations must corroborate the kind used by Assignable.
		if (name == "java/lang/Cloneable" || name == "java/io/Serializable") && o.AccessFlags&0x200 == 0 {
			return callbinding.Class{}, false
		}
		if name == "java/lang/Object" && (o.SuperClass != 0 || len(o.Interfaces) != 0 || o.AccessFlags&(0x200|0x10) != 0) {
			return callbinding.Class{}, false
		}
		parents := []string{}
		physical := []string{}
		if o.SuperClass != 0 {
			p, pk := sourceBridgeClassName(o, o.SuperClass)
			if !pk || !nativeSourceBinaryName(p) {
				return callbinding.Class{}, false
			}
			physical = append(physical, p)
			if o.AccessFlags&0x200 == 0 {
				parents = append(parents, p)
			} else if p != "java/lang/Object" {
				return callbinding.Class{}, false
			}
		} else if name != "java/lang/Object" {
			return callbinding.Class{}, false
		}
		seen := map[string]bool{}
		for _, p := range physical {
			seen[p] = true
		}
		for _, i := range o.Interfaces {
			p, pk := sourceBridgeClassName(o, i)
			if !pk || !nativeSourceBinaryName(p) || seen[p] || !nativeProofWork(work, 1) {
				return callbinding.Class{}, false
			}
			seen[p] = true
			physical = append(physical, p)
			parents = append(parents, p)
		}
		// A source extends edge cannot name an interface/final class; an
		// implements/superinterface edge must name an interface. Resolve every
		// physical edge even if another path already reaches the wanted type.
		for i, parentName := range physical {
			parent, pk := stable(parentName)
			if !pk || parent == nil {
				return callbinding.Class{}, false
			}
			classEdge := i == 0 && o.SuperClass != 0
			if classEdge {
				if parent.AccessFlags&0x200 != 0 || o.AccessFlags&0x200 == 0 && parent.AccessFlags&0x10 != 0 {
					return callbinding.Class{}, false
				}
			} else if parent.AccessFlags&0x200 == 0 {
				return callbinding.Class{}, false
			}
		}
		for _, attribute := range o.Attributes {
			if sourceProofNil(attribute) || !nativeProofWork(work, 1) {
				return callbinding.Class{}, false
			}
		}
		sig, sk := nativeMethodLocalOriginalSignature(o, o.Attributes, work)
		if !sk || len(sig) > 4096 || !nativeProofWork(work, int64(len(sig))*130+1) || work != nil && work.CheckAlloc(int64(len(sig))*256+128) != nil {
			return callbinding.Class{}, false
		}
		if sig != "" {
			if _, _, valid := types.EraseLexicalMethodSignatureWithThrows(sig, "()V"); !valid {
				return callbinding.Class{}, false
			}
			own, refs, valid := types.SignatureTypeVariableReferences(sig)
			if !valid {
				return callbinding.Class{}, false
			}
			scope := map[string]bool{}
			for _, n := range own {
				scope[n] = true
			}
			for _, n := range refs {
				if !scope[n] {
					return callbinding.Class{}, false
				}
			}
			superclass, interfaces := types.ParseClassSignatureSupers(sig)
			edges := append([]types.JavaType{superclass}, interfaces...)
			if len(edges) != len(physical) {
				return callbinding.Class{}, false
			}
			for i, edge := range edges {
				n, nk := types.RawClassFQN(edge)
				if !nk || strings.ReplaceAll(n, ".", "/") != physical[i] {
					return callbinding.Class{}, false
				}
				if pt, parameterized := types.AsParameterizedType(edge); parameterized {
					if len(pt.OwnerSegments) > 1 {
						return callbinding.Class{}, false
					}
					parent, pk := stable(physical[i])
					if !pk || parent == nil {
						return callbinding.Class{}, false
					}
					ps, pk := nativeMethodLocalOriginalSignature(parent, parent.Attributes, work)
					if !pk || len(ps) > 4096 || !nativeProofWork(work, int64(len(ps))*130+1) || work != nil && work.CheckAlloc(int64(len(ps))*256+128) != nil {
						return callbinding.Class{}, false
					}
					if _, _, valid := types.EraseLexicalMethodSignatureWithThrows(ps, "()V"); !valid {
						return callbinding.Class{}, false
					}
					if len(types.ClassFormalTypeParamNames(ps)) != len(pt.TypeArgs) {
						return callbinding.Class{}, false
					}
					for _, argument := range pt.TypeArgs {
						if argument == nil || types.IsWildcardType(argument) {
							return callbinding.Class{}, false
						}
					}
				}
			}
		}
		return callbinding.Class{Name: name, Parents: parents, ParentsComplete: true}, true
	})
	// Close both original ancestry graphs before accepting any path. A second
	// unknown parent or a cycle must not disappear because BFS found the goal.
	active, visited := map[string]bool{}, map[string]bool{}
	var close func(string, int) bool
	close = func(name string, depth int) bool {
		if depth > 64 || active[name] || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(active)+len(visited)+1)*256) != nil {
			return false
		}
		if visited[name] {
			return true
		}
		active[name] = true
		defer delete(active, name)
		declaration, ok := query.class(name)
		if !ok || query.exhausted {
			return false
		}
		for _, parent := range declaration.Parents {
			if !close(parent, depth+1) {
				return false
			}
		}
		visited[name] = true
		return true
	}
	for _, descriptor := range []string{implementation, formal} {
		element := strings.TrimLeft(descriptor, "[")
		if len(descriptor)-len(element) > 255 {
			return false
		}
		if strings.HasPrefix(element, "L") && !close(callbinding.Name(element), 0) {
			return false
		}
	}
	return query.assignable(implementation, formal)
}
