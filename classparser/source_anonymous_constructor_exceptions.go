package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Native javac8 omits Exceptions on a static anonymous constructor. Modern
// javac infers its checked throws from the parent declaration. This projection
// belongs to the completed source artifact, never to an ordinary constructor.
// Reopen the physical capture/SUPER/initializer proof and require the original
// lexical factory to declare every checked exception. Generic substitution,
// caught-only factories and throwing initializers need separate proofs.
func (c *ClassObjectDumper) sourceAnonymousCheckedSuperProjection(child *nativeAnonymousClass, metadata callbinding.Provider) (string, string, bool) {
	if c == nil || c.obj == nil || child == nil || child.object == nil || child.enclosingField != "" || child.unusedEnclosing || child.sharedParameterRoles || child.method == "" || metadata == nil {
		return "", "", false
	}
	exceptions, closed := sourceAnonymousStaticConstructorCalls(child, metadata, c.Work)
	if !closed || len(exceptions) == 0 || len(exceptions) > 128 || c.Work.CheckAlloc(int64(len(exceptions))*96+16384+int64(len(child.descriptor))) != nil {
		return "", "", false
	}
	parentName := child.object.GetSupperClassName()
	parent, known := metadata(parentName)
	if !known || parent.Name != parentName || !parent.MembersComplete || parent.Signature != "" || !nativeProofWork(c.Work, int64(len(parent.Methods))+1) {
		return "", "", false
	}
	matches := 0
	for _, method := range parent.Methods {
		if method.Name == "<init>" && method.Desc == child.superDescriptor {
			if method.Generic || method.Signature != "" || !method.ExceptionsKnown {
				return "", "", false
			}
			matches++
		}
	}
	if matches != 1 {
		return "", "", false
	}
	var factory *MemberInfo
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return "", "", false
		}
		name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
		descriptor, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
		if !nk || !dk {
			return "", "", false
		}
		if name+descriptor == child.method {
			if factory != nil || method.AccessFlags&StaticFlag == 0 {
				return "", "", false
			}
			factory = method
		}
	}
	if factory == nil {
		return "", "", false
	}
	if _, present, valid := sourceMethodSignatureKey(c.obj, factory); !valid || present {
		return "", "", false
	}
	declared, _, valid := sourceAnonymousExceptionNames(c.obj, factory, c.Work)
	if !valid {
		return "", "", false
	}
	covered := strings.Split(declared, "\x00")
	text := int64(0)
	for _, exception := range exceptions {
		cost := int64(len(exception)) + 1
		if !nativeSourceBinaryName(exception) || cost > (8192-text) || !nativeProofWork(c.Work, cost) || !sourceAnonymousExceptionHierarchyClosed(exception, metadata, c.Work) {
			return "", "", false
		}
		text += cost
		permitted := originalExceptionUnchecked(exception, metadata)
		for _, declaration := range covered {
			if !nativeProofWork(c.Work, 1) {
				return "", "", false
			}
			if declaration != "" && originalExceptionCovered(exception, declaration, metadata) {
				permitted = true
			}
		}
		if !permitted {
			return "", "", false
		}
	}
	inferred, known := sourceAnonymousInferredSuperExceptions(exceptions, metadata, c.Work)
	if !known {
		return "", "", false
	}
	if inferred == "" {
		return "", "", true // Only unchecked declarations generate no attribute.
	}
	return child.descriptor, inferred, true
}

// javac Flow.markThrown ignores unchecked exceptions. Check.incl maintains
// an ordered maximal-type antichain: an already covered subtype is ignored;
// a new supertype removes covered entries and is prepended. The result is not
// the parent's Exceptions table, even when no initializer can throw.
func sourceAnonymousInferredSuperExceptions(exceptions []string, metadata callbinding.Provider, work *workbudget.Budget) (string, bool) {
	if len(exceptions) > 128 || work.CheckAlloc(int64(len(exceptions))*64+16384) != nil {
		return "", false
	}
	var inferred []string
	text := int64(0)
	for _, exception := range exceptions {
		cost := int64(len(exception)) + 1
		if !nativeSourceBinaryName(exception) || cost > 8192-text || !nativeProofWork(work, cost) || !sourceAnonymousExceptionHierarchyClosed(exception, metadata, work) {
			return "", false
		}
		text += cost
		if originalExceptionUnchecked(exception, metadata) {
			continue
		}
		covered := false
		for _, existing := range inferred {
			if !nativeProofWork(work, 1) {
				return "", false
			}
			covered = covered || originalExceptionCovered(exception, existing, metadata)
		}
		if covered {
			continue
		}
		next := []string{exception}
		for _, existing := range inferred {
			if !nativeProofWork(work, 1) {
				return "", false
			}
			if !originalExceptionCovered(existing, exception, metadata) {
				next = append(next, existing)
			}
		}
		inferred = next
	}
	return strings.Join(inferred, "\x00"), true
}

// A negative subtype query is meaningful only after all original ancestors
// are available. An unknown custom exception cannot masquerade as a checked,
// unrelated type merely because the subtype lookup returned false.
func sourceAnonymousExceptionHierarchyClosed(exception string, metadata callbinding.Provider, work *workbudget.Budget) bool {
	queue, seen := []string{exception}, map[string]bool{}
	text := int64(0)
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		cost := int64(len(current)) + 1
		if len(seen) >= 128 || cost > (16384-text) || !nativeProofWork(work, cost) || work.CheckAlloc(text+cost+int64(len(seen)+len(queue)+1)*128) != nil {
			return false
		}
		text += cost
		seen[current] = true
		declaration, known := callbinding.Class{}, false
		if metadata != nil {
			declaration, known = metadata(current)
		}
		parents := declaration.Parents
		if !known || declaration.Name != current || !declaration.ParentsComplete {
			platform, known := types.KnownPlatformSuperTypes(strings.ReplaceAll(current, "/", "."))
			if !known {
				return false
			}
			parents = make([]string, len(platform))
			for i, name := range platform {
				parents[i] = strings.ReplaceAll(name, ".", "/")
			}
		}
		if len(parents)+len(queue) > 128 {
			return false
		}
		queue = append(queue, parents...)
	}
	return seen["java/lang/Throwable"]
}

// Presence is separate from an empty table. The bounded logical tuple owns
// declaration order and rejects duplicate/malformed attributes and CP aliases
// that do not identify a real exception class.
func sourceAnonymousExceptionNames(object *ClassObject, method *MemberInfo, work *workbudget.Budget) (string, bool, bool) {
	if object == nil || method == nil || !nativeProofWork(work, int64(len(method.Attributes))+1) {
		return "", false, false
	}
	var names []string
	present, text := false, int64(0)
	for _, attribute := range method.Attributes {
		if declaration, ok := attribute.(*ExceptionsAttribute); ok {
			if present || declaration == nil || len(declaration.ExceptionIndexTable) > 128 || !nativeProofWork(work, int64(len(declaration.ExceptionIndexTable))+1) {
				return "", false, false
			}
			present = true
			for _, index := range declaration.ExceptionIndexTable {
				name, known := sourceBridgeClassName(object, index)
				cost := int64(len(name)) + 1
				if !known || !nativeSourceBinaryName(name) || cost > 8192-text || !nativeProofWork(work, cost) || work.CheckAlloc(text+cost+int64(len(names)+1)*32) != nil {
					return "", false, false
				}
				text += cost
				names = append(names, name)
			}
		}
	}
	return strings.Join(names, "\x00"), present, true
}

func sourceAnonymousConstructorMethod(object *ClassObject, descriptor string, work *workbudget.Budget) (*MemberInfo, bool) {
	var result *MemberInfo
	for _, method := range object.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		name, nk := sourceBridgeUTF8(object, method.NameIndex)
		desc, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !nk || !dk {
			return nil, false
		}
		if name == "<init>" {
			if result != nil || desc != descriptor {
				return nil, false
			}
			result = method
		}
	}
	return result, result != nil
}

func sourceAnonymousConstructorExceptionEdit(original, rebuilt *ClassObject, compiled []byte, plan *sourceAnonymousClassMetadataPlan, layout *compiledMetadataLayout, work *workbudget.Budget) (*compiledMetadataEdit, error) {
	fail := func() (*compiledMetadataEdit, error) {
		return nil, sourceLambdaMetadataFailure(work, "static anonymous constructor exception declaration changed")
	}
	if plan.constructorDescriptor == "" {
		if plan.inferredExceptions != "" {
			return fail()
		}
		return nil, nil
	}
	if plan.inferredExceptions == "" || len(plan.inferredExceptions) > 8192 {
		return fail()
	}
	want, wk := sourceAnonymousConstructorMethod(original, plan.constructorDescriptor, work)
	got, gk := sourceAnonymousConstructorMethod(rebuilt, plan.constructorDescriptor, work)
	if !wk || !gk || want.AccessFlags != got.AccessFlags {
		return fail()
	}
	_, declared, valid := sourceAnonymousExceptionNames(original, want, work)
	actual, present, actualValid := sourceAnonymousExceptionNames(rebuilt, got, work)
	if !valid || declared || !actualValid {
		return fail()
	}
	if !present {
		return nil, nil // The already restored class is idempotent.
	}
	if actual != plan.inferredExceptions {
		return fail()
	}
	edit, err := compiledMethodWithoutAttributes(compiled, rebuilt, got, layout, map[string]bool{"Exceptions": true}, work)
	return &edit, err
}
