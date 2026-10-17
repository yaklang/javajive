package javaclassparser

import (
	"crypto/sha256"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// A method-local is an independent source role. Its original enclosing-field
// path may supply a named member's hidden receiver without granting that local
// a member or anonymous declaration role. This is an origin certificate, never
// a nonnull fact: an unqualified allocation must retain the original nullable
// hidden word and each earlier enclosing dereference.
type nativeMethodLocalMemberEnclosing struct {
	caller                   *ClassObject
	local                    *nativeMethodLocalClass
	child                    *nativeMemberClass
	methodKey, descriptor    string
	newPC, invokePC, checkPC int
	read                     *nativeMemberLexicalRead
	inputs                   map[*ClassObject][32]byte
}

func nativeMethodLocalMemberEnclosingProof(p *nativeMemberFamily, caller *ClassObject, method string, newPC int, read *nativeMemberLexicalRead, child *nativeMemberClass, work *workbudget.Budget) *nativeMethodLocalMemberEnclosing {
	if p == nil || caller == nil || child == nil || child.object == nil || child.static || read == nil || read.descriptor != "L"+child.owner+";" || !nativeProofWork(work, 1) {
		return nil
	}
	owner, valid := nativeMemberJointMethodLocalOwner(p, caller, work)
	if !valid {
		return nil
	}
	local := p.methodLocals[caller.GetClassName()]
	declaring, name, flags, valid := originalMemberOwner(child.object)
	if !valid || declaring != child.owner || name != child.name || flags != child.flags || !nativeMemberSuperCaptureDeclaration(child, work) {
		return nil
	}
	if local.constructor.enclosingField == "" {
		return nil
	}
	r := &nativeMethodLocalMemberEnclosing{caller: caller, local: local, child: child, methodKey: method, newPC: newPC, read: read, checkPC: -1, inputs: map[*ClassObject][32]byte{}}
	total := 0
	seal := func(obj *ClassObject) bool {
		if obj == nil {
			return false
		}
		if _, present := r.inputs[obj]; present {
			return true
		}
		if len(r.inputs) >= 66 || !nativeProofWork(work, 1) {
			return false
		}
		raw := obj.Bytes()
		total += len(raw)
		if len(raw) > 1<<20 || !nativeProofWork(work, int64(len(raw))) || work != nil && work.CheckAlloc(int64(total+len(r.inputs)*128)) != nil {
			return false
		}
		r.inputs[obj] = sha256.Sum256(raw)
		return true
	}
	if !seal(caller) || !seal(child.object) {
		return nil
	}
	// Close every original declaration that places the local, even when the
	// requested value path stops at a nearer owner than the compilation root.
	current := owner.owner
	for depth := 0; ; depth++ {
		if depth >= 64 || !seal(p.lexicalObjects[current]) {
			return nil
		}
		if current == p.owner {
			break
		}
		named := p.children[current]
		if named == nil || named.object != p.lexicalObjects[current] {
			return nil
		}
		current = named.owner
	}
	seen := map[*nativeMemberLexicalRead]bool{}
	for node := read; node != nil; node = node.prior {
		if len(seen) >= 64 || seen[node] || node.parameterOwner != "" || node.parameterDescriptor != "" || !nativeProofWork(work, 1) {
			return nil
		}
		seen[node] = true
		if node.prior == nil {
			if node.owner != caller.GetClassName() || node.field != local.constructor.enclosingField || node.descriptor != "L"+owner.owner+";" {
				return nil
			}
		} else {
			named := p.children[node.owner]
			if named == nil || named.object != p.lexicalObjects[node.owner] || node.field != named.field || node.descriptor != "L"+named.owner+";" || !nativeMemberSuperCaptureDeclaration(named, work) {
				return nil
			}
		}
	}
	return r
}

func nativeMethodLocalMemberEnclosingCurrent(p *nativeMemberFamily, current string, plan *nativeMemberAllocation, work *workbudget.Budget) bool {
	if p == nil || p.failed || plan == nil || plan.methodLocalEnclosing == nil || !nativeProofWork(work, 1) {
		return false
	}
	r := plan.methodLocalEnclosing
	if r.caller == nil || r.local == nil || r.child == nil || r.child.object == nil || r.caller.GetClassName() != current || p.methodLocals[current] != r.local || r.local.object != r.caller || r.child != plan.child || p.allocationClass(r.child.object.GetClassName()) != r.child || r.newPC != plan.newPC || r.invokePC != plan.invokePC || r.descriptor != plan.descriptor || r.checkPC != plan.checkPC || r.read == nil || r.read.pc != plan.enclosingReadPC {
		return false
	}
	if plan.slot != 0 || plan.implicitEnclosing != (r.checkPC < 0) || plan.anonymousEnclosingRead != nil || plan.anonymousMemberEnclosing != nil || plan.enclosingParameter != nil || plan.freshEnclosing != nil {
		return false
	}
	for obj, hash := range r.inputs {
		if obj == nil || obj != r.child.object && p.lexicalObjects[obj.GetClassName()] != obj {
			return false
		}
		raw := obj.Bytes()
		if !nativeProofWork(work, int64(len(raw))) || work != nil && work.CheckAlloc(int64(len(raw))) != nil || sha256.Sum256(raw) != hash {
			return false
		}
	}
	reads, valid := nativeMemberLexicalReads(r.caller, p, work)
	if !valid {
		return false
	}
	fresh := reads[r.methodKey][r.read.pc]
	reopened := nativeMethodLocalMemberEnclosingProof(p, r.caller, r.methodKey, r.newPC, fresh, r.child, work)
	if reopened == nil || len(r.inputs) != len(reopened.inputs) {
		return false
	}
	for obj, hash := range reopened.inputs {
		if original, present := r.inputs[obj]; !present || original != hash {
			return false
		}
	}
	left, right := r.read, fresh
	for depth := 0; left != nil || right != nil; depth++ {
		if depth >= 64 || left == nil || right == nil || !nativeProofWork(work, 1) {
			return false
		}
		if left.owner != right.owner || left.field != right.field || left.descriptor != right.descriptor || left.pc != right.pc || left.basePC != right.basePC || left.parameterOwner != right.parameterOwner || left.parameterDescriptor != right.parameterDescriptor {
			return false
		}
		left, right = left.prior, right.prior
	}
	return true
}

// Source selection and receiver origin are separate obligations. A local's
// formal or inherited member type can hide the allocated class even when its
// enclosing receiver has exactly the right descriptor. Prefer a proved simple
// binding; otherwise anchor the type path at the actual enclosing root class.
// Type qualification preserves implicit enclosing evaluation: it does not
// introduce the null check of an explicit value-qualified outer.new call.
func (c *ClassObjectDumper) nativeMethodLocalMemberAllocationNames(plan *nativeMemberAllocation, p *nativeMemberFamily, ctx *class_context.ClassContext) (string, string, bool) {
	if c == nil || c.obj == nil || ctx == nil || p == nil || p.failed || plan == nil || plan.methodLocalEnclosing == nil || plan.child == nil || plan.child.object == nil || p.children[plan.child.object.GetClassName()] != plan.child || c.nativeMethodLocalCurrent != plan.methodLocalEnclosing.local || c.CurrentMethod == nil || !nativeProofWork(c.Work, 1) {
		return "", "", false
	}
	r := plan.methodLocalEnclosing
	owned := 0
	for _, original := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return "", "", false
		}
		if original == c.CurrentMethod {
			owned++
		}
	}
	if owned != 1 {
		return "", "", false
	}
	method, mk := sourceBridgeUTF8(c.obj, c.CurrentMethod.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, c.CurrentMethod.DescriptorIndex)
	if !mk || !dk || method+desc != r.methodKey || ctx.FunctionName != method || ctx.CurrentMethodDesc != desc {
		return "", "", false
	}
	scope := c.nativeMethodLocalTypeScope(p, r.local, ctx)
	if scope == nil {
		return "", "", false
	}
	binds := scope.binds
	if !plan.implicitEnclosing {
		parts := []string{}
		owner := plan.child.owner
		for depth := 0; depth < 64; depth++ {
			object := p.lexicalObjects[owner]
			if object == nil || !nativeProofWork(c.Work, 1) {
				return "", "", false
			}
			name, parent := "", ""
			if owner == p.owner {
				name = owner[strings.LastIndexByte(owner, '/')+1:]
			} else if named := p.children[owner]; named != nil && named.object == object {
				name, parent = named.name, named.owner
			} else {
				return "", "", false
			}
			parts = append([]string{name}, parts...)
			if binds(name, owner) {
				return plan.child.name, strings.Join(parts, ".") + ".this", true
			}
			if parent == "" {
				return "", "", false
			}
			owner = parent
		}
		return "", "", false
	}
	if binds(plan.child.name, plan.child.object.GetClassName()) {
		return plan.child.name, "", true
	}
	parts := []string{plan.child.name}
	owner := plan.child.owner
	for depth := 0; depth < 64; depth++ {
		object := p.lexicalObjects[owner]
		if object == nil || !nativeProofWork(c.Work, 1) {
			return "", "", false
		}
		// A raw qualified owner would change an enclosing generic view. Keep
		// that case closed until its original type arguments are denotable.
		signature, known := nativeMethodLocalOriginalSignature(object, object.Attributes, c.Work)
		if !known {
			return "", "", false
		}
		if signature != "" {
			formals, _, known := types.SignatureTypeVariableReferences(signature)
			if !known || len(formals) != 0 {
				return "", "", false
			}
		}
		if owner == p.owner {
			name := owner[strings.LastIndexByte(owner, '/')+1:]
			if !binds(name, owner) {
				return "", "", false
			}
			parts = append(parts, name)
			for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
				parts[i], parts[j] = parts[j], parts[i]
			}
			return strings.Join(parts, "."), "", true
		}
		named := p.children[owner]
		if named == nil || named.object != object || !class_context.PreservesJava8Identifier(named.name) {
			return "", "", false
		}
		parts = append(parts, named.name)
		owner = named.owner
	}
	return "", "", false
}

type nativeMethodLocalTypeScope struct {
	family *nativeMemberFamily
	work   *workbudget.Budget
	binds  func(string, string) bool
}

// Original lexical type identity is distinct from a same-spelled formal,
// imported name, or inherited member. Formatting a live receiver, a generated
// temporary's declaration and an allocation must use this same source scope.
func (c *ClassObjectDumper) nativeMethodLocalTypeScope(p *nativeMemberFamily, local *nativeMethodLocalClass, ctx *class_context.ClassContext) *nativeMethodLocalTypeScope {
	if c == nil || c.obj == nil || ctx == nil || local == nil || local.object != c.obj || p == nil || p.failed || !nativeProofWork(c.Work, 1) {
		return nil
	}
	if _, known := nativeMemberJointMethodLocalOwner(p, c.obj, c.Work); !known {
		return nil
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	// Resolve an original member-type declaration through the entire hierarchy.
	// An own declaration hides inherited declarations. Multiple inherited
	// identities remain ambiguous; a missing ancestor never proves absence.
	memberTypes := func(object *ClassObject, name string) (map[string]bool, bool) {
		active := map[string]bool{}
		memo := map[string]map[string]bool{}
		var visit func(*ClassObject) (map[string]bool, bool)
		visit = func(node *ClassObject) (map[string]bool, bool) {
			if node == nil || !nativeProofWork(c.Work, 1) {
				return nil, false
			}
			owner := node.GetClassName()
			if active[owner] {
				return nil, false
			}
			if result, present := memo[owner]; present {
				return result, true
			}
			if len(memo)+len(active) >= 64 || c.Work != nil && c.Work.CheckAlloc(int64(len(memo)+len(active)+1)*512) != nil {
				return nil, false
			}
			active[owner] = true
			defer delete(active, owner)
			result := map[string]bool{}
			for _, attribute := range node.Attributes {
				if !nativeProofWork(c.Work, 1) {
					return nil, false
				}
				rows, ok := attribute.(*InnerClassesAttribute)
				if !ok {
					continue
				}
				if rows == nil {
					return nil, false
				}
				for _, row := range rows.Classes {
					if row == nil || !nativeProofWork(c.Work, 1) {
						return nil, false
					}
					if row.InnerNameIndex == 0 || row.OuterClassInfoIndex == 0 {
						continue
					}
					simple, known := sourceBridgeUTF8(node, row.InnerNameIndex)
					if !known {
						return nil, false
					}
					if simple != name {
						continue
					}
					declaring, known := sourceBridgeClassName(node, row.OuterClassInfoIndex)
					if !known {
						return nil, false
					}
					if declaring != owner {
						continue
					}
					binary, known := sourceBridgeClassName(node, row.InnerClassInfoIndex)
					if !known || result[binary] {
						return nil, false
					}
					actual := p.lexicalObjects[binary]
					if actual == nil {
						actual, known = resolve(binary)
					}
					if actual == nil || actual.GetClassName() != binary || !known && p.lexicalObjects[binary] == nil {
						return nil, false
					}
					ao, an, af, valid := originalMemberOwner(actual)
					if !valid || ao != declaring || an != simple || af != row.InnerClassAccessFlags {
						return nil, false
					}
					result[binary] = true
				}
			}
			if len(result) > 0 {
				memo[owner] = result
				return result, true
			}
			parents := []string{}
			if super := node.GetSupperClassName(); super != "" {
				parents = append(parents, super)
			}
			for _, index := range node.Interfaces {
				parent, known := sourceBridgeClassName(node, index)
				if !known {
					return nil, false
				}
				parents = append(parents, parent)
			}
			for _, parent := range parents {
				if !nativeProofWork(c.Work, 1) {
					return nil, false
				}
				next := p.lexicalObjects[parent]
				if next == nil {
					var known bool
					next, known = resolve(parent)
					if !known {
						return nil, false
					}
				}
				if next == nil || next.GetClassName() != parent {
					return nil, false
				}
				inherited, known := visit(next)
				if !known {
					return nil, false
				}
				for binary := range inherited {
					result[binary] = true
				}
			}
			memo[owner] = result
			return result, true
		}
		return visit(object)
	}
	binds := func(name, target string) bool {
		if !class_context.PreservesJava8Identifier(name) || ctx.IsTypeParam(name) || ctx.LexicalTypeNames[name] {
			return false
		}
		// Keep other declarations in the same original method conservative:
		// the placement pass knows their blocks, this type-name query does not.
		for _, other := range p.methodLocals {
			if other == nil || other.owner == nil || !nativeProofWork(c.Work, 1) {
				return false
			}
			if other != local && other.owner.owner == local.owner.owner && other.owner.method == local.owner.method && other.owner.descriptor == local.owner.descriptor && other.owner.name == name {
				return false
			}
		}
		current := c.obj
		for depth := 0; depth < 64; depth++ {
			if current == nil || !nativeProofWork(c.Work, 1) {
				return false
			}
			binary := current.GetClassName()
			simple, parent := "", ""
			if current == local.object {
				simple, parent = local.owner.name, local.owner.owner
			} else if binary == p.owner {
				simple = binary[strings.LastIndexByte(binary, '/')+1:]
			} else if named := p.children[binary]; named != nil && named.object == current {
				simple, parent = named.name, named.owner
			} else {
				return false
			}
			if simple == name {
				return binary == target
			}
			candidates, known := memberTypes(current, name)
			if !known {
				return false
			}
			if len(candidates) > 0 {
				return len(candidates) == 1 && candidates[target]
			}
			if parent == "" {
				return false
			}
			current = p.lexicalObjects[parent]
		}
		return false
	}

	return &nativeMethodLocalTypeScope{p, c.Work, binds}
}

func (scope *nativeMethodLocalTypeScope) sourceName(binary string) (string, bool) {
	if scope == nil || scope.family == nil || scope.binds == nil {
		return "", false
	}
	p := scope.family
	parts := []string{}
	current := binary
	for depth := 0; depth < 64; depth++ {
		object := p.lexicalObjects[current]
		if object == nil || !nativeProofWork(scope.work, 1) {
			return "", false
		}
		name, parent := "", ""
		if current == p.owner {
			name = current[strings.LastIndexByte(current, '/')+1:]
		} else if child := p.children[current]; child != nil && child.object == object {
			name, parent = child.name, child.owner
		} else {
			return "", false
		}
		parts = append([]string{name}, parts...)
		if scope.binds(name, current) {
			return strings.Join(parts, "."), true
		}
		if parent == "" {
			return "", false
		}
		current = parent
	}
	return "", false
}
