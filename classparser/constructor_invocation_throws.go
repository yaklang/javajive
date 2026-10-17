package javaclassparser

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Constructor declarations retain erased Exceptions, while a Java invocation
// checks their instantiated throws types. Bind only this original THIS/SUPER
// call; this proof changes neither its operands nor either constructor's ABI.
func (c *ClassObjectDumper) constructorDelegationInvocationThrows(member *values.JavaClassMember, pc int, call *values.FunctionCallExpression) ([]string, bool) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || member == nil || call == nil || member.Member != "<init>" || call.FunctionName != member.Member || call.Descriptor != member.Description || strings.ReplaceAll(call.ClassName, ".", "/") != member.Name || call.Kind != values.InvokeSpecial || !call.IsSpecialInvoke || !call.HasOriginPC || call.OriginPC != pc || pc < 0 {
		return nil, false
	}
	target := c.options.TargetSourceVersion
	if target == 0 {
		target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
	}
	if target < 8 {
		return nil, false
	}
	owner := member.Name
	if owner != c.obj.GetClassName() && owner != c.obj.GetSupperClassName() {
		return nil, false
	}
	receiver, known := values.UnpackSoltValue(call.Object).(*values.JavaRef)
	if !known || receiver == nil || !receiver.IsThis || len(call.Arguments) > 128 {
		return nil, false
	}
	actuals := make([]string, len(call.Arguments))
	for i, argument := range call.Arguments {
		if argument == nil || argument.Type() == nil {
			return nil, false
		}
		if values.IsNullLiteral(values.UnpackSoltValue(argument)) {
			actuals[i] = "null"
			continue
		}
		var valid bool
		actuals[i], valid = values.SourceTypeErasure(argument.Type(), c.FuncCtx)
		if !valid || actuals[i] == "" {
			return nil, false
		}
	}
	return constructorGenericInvocationThrows(c.FuncCtx.InvocationMetadata, owner, member.Description, actuals, c.Work)
}

// The bounded domain has method-owned variables in bare parameter and throws
// positions, with concrete ordered bounds. Actual lower bounds are solved
// first. Only an unconstrained throws variable with proper upper bounds among
// Object, Exception and Throwable has the Java throws-inference preference for
// RuntimeException. Nested/dependent/class-variable inference is separate.
// JLS8 18.5.1 introduces throws alpha; 18.4 resolves proper lower bounds first:
// https://docs.oracle.com/javase/specs/jls/se8/html/jls-18.html#jls-18.4
func constructorGenericInvocationThrows(provider callbinding.Provider, owner, descriptor string, actuals []string, work *workbudget.Budget) ([]string, bool) {
	if provider == nil || owner == "" || len(owner)+len(descriptor) > 8192 || len(actuals) > 128 || !nativeProofWork(work, int64(len(owner)+len(descriptor)+len(actuals))+1) {
		return nil, false
	}
	metadata, known := provider(owner)
	if !known || metadata.Name != owner || !metadata.MembersComplete || !metadata.ParentsComplete || metadata.IsInterface || len(metadata.Methods) > 256 {
		return nil, false
	}
	var target callbinding.Method
	matches := 0
	for _, method := range metadata.Methods {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if method.Name == "<init>" && method.Desc == descriptor {
			target, matches = method, matches+1
		}
	}
	if matches != 1 || !target.Generic || target.Static || target.Bridge || target.Varargs || !target.ExceptionsKnown || len(target.Signature) == 0 || len(target.Signature)+len(metadata.Signature) > 8192 || !nativeProofWork(work, int64(len(target.Signature)+len(metadata.Signature))+1) || work != nil && work.CheckAlloc(int64(len(target.Signature)+len(metadata.Signature))*16+4096) != nil {
		return nil, false
	}
	erased, erasedThrows, valid := types.EraseLexicalMethodSignatureWithThrows(metadata.Signature, target.Signature)
	if !valid || erased != descriptor || len(erasedThrows) != len(target.Exceptions) {
		return nil, false
	}
	for i, exception := range erasedThrows {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if !strings.HasPrefix(exception, "L") || !strings.HasSuffix(exception, ";") || callbinding.Name(exception) != target.Exceptions[i] {
			return nil, false
		}
	}
	constraints, bare, valid := types.FormalBoundConstraints(target.Signature)
	if !valid || len(constraints) == 0 || len(constraints) > 32 {
		return nil, false
	}
	_, references, valid := types.SignatureTypeVariableReferences(target.Signature)
	if !valid {
		return nil, false
	}
	for _, name := range references {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if constraints[name] == nil {
			return nil, false // a class formal is not a method-owned inference variable
		}
	}
	_, parameters, result, throws := types.ParseMethodSignatureFullWithThrows(target.Signature, &class_context.ClassContext{})
	physical, ret, err := callbinding.Descriptor(descriptor)
	if err != nil || ret != "V" || result == nil || len(parameters) != len(actuals) || len(physical) != len(actuals) || len(throws) != len(target.Exceptions) {
		return nil, false
	}
	q := newConstructorWideningQuery(func(name string) (callbinding.Class, bool) {
		if !nativeProofWork(work, 1) {
			return callbinding.Class{}, false
		}
		return provider(name)
	})
	bindings := map[string]string{}
	for i, parameter := range parameters {
		if !nativeProofWork(work, 1) || parameter == nil || !constructorInvocationThrowsSimpleType(parameter, bare[i] != "") || !q.assignable(actuals[i], physical[i]) {
			return nil, false
		}
		name := bare[i]
		if name == "" || actuals[i] == "null" {
			continue
		}
		if !callbinding.Reference(actuals[i]) || strings.HasPrefix(actuals[i], "[") {
			return nil, false
		}
		prior := bindings[name]
		if prior == "" || q.assignable(prior, actuals[i]) {
			bindings[name] = actuals[i]
		} else if !q.assignable(actuals[i], prior) {
			return nil, false // general least-upper-bound/intersection inference is separate
		}
	}
	// Preserve overload identity. An unrelated boolean/numeric primitive port
	// excludes a fixed-arity rival without a cast or inference assumption. Other
	// same-arity competitors, and every varargs family, remain outside this proof.
	for _, rival := range metadata.Methods {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if rival.Name != "<init>" || rival.Desc == descriptor {
			continue
		}
		if len(rival.Desc)+len(rival.Signature) > 8192 || rival.Generic != (rival.Signature != "") || !nativeProofWork(work, int64(len(rival.Desc)+len(rival.Signature))+1) {
			return nil, false
		}
		if rival.Generic {
			erased, _, valid := types.EraseLexicalMethodSignatureWithThrows(metadata.Signature, rival.Signature)
			if !valid || erased != rival.Desc {
				return nil, false
			}
		}
		ps, r, err := callbinding.Descriptor(rival.Desc)
		if err != nil || r != "V" || rival.Varargs || rival.Static || rival.Bridge {
			return nil, false
		}
		if len(ps) != len(actuals) {
			continue
		}
		excluded := false
		for i, p := range ps {
			if !nativeProofWork(work, 1) {
				return nil, false
			}
			if len(p) == 1 && len(actuals[i]) == 1 && (p == "Z") != (actuals[i] == "Z") && strings.Contains("ZBCSIJFD", p) && strings.Contains("ZBCSIJFD", actuals[i]) {
				excluded = true
			}
		}
		if !excluded {
			return nil, false
		}
	}
	thrownVariables := map[string]bool{}
	for _, thrown := range throws {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if name, variable := types.TypeVariableName(thrown); variable {
			thrownVariables[name] = true
			bounds := constraints[name]
			if len(bounds) == 0 {
				return nil, false
			}
			raw, known := types.RawClassFQN(bounds[0])
			if !known || !q.assignable("L"+strings.ReplaceAll(raw, ".", "/")+";", "Ljava/lang/Throwable;") {
				return nil, false
			}
		}
	}
	for name, bounds := range constraints {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		candidate := bindings[name]
		if candidate == "" {
			if !thrownVariables[name] {
				return nil, false
			}
			for _, bound := range bounds {
				if !nativeProofWork(work, 1) {
					return nil, false
				}
				raw, known := types.RawClassFQN(bound)
				if !known || raw != "java.lang.Object" && raw != "java.lang.Exception" && raw != "java.lang.Throwable" {
					return nil, false
				}
			}
			candidate = "Ljava/lang/RuntimeException;"
		}
		seen := map[string]bool{}
		for i, bound := range bounds {
			if !nativeProofWork(work, 1) {
				return nil, false
			}
			if !constructorInvocationThrowsSimpleType(bound, false) {
				return nil, false
			}
			raw, known := types.RawClassFQN(bound)
			if !known {
				return nil, false
			}
			name := strings.ReplaceAll(raw, ".", "/")
			desc := "L" + name + ";"
			if seen[desc] || !q.assignable(candidate, desc) {
				return nil, false
			}
			if i > 0 {
				declaration, known := q.class(name)
				if !known || !declaration.IsInterface {
					return nil, false
				}
			}
			seen[desc] = true
		}
		bindings[name] = candidate
	}
	var exceptions []string
	for _, thrown := range throws {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		var exception string
		if name, variable := types.TypeVariableName(thrown); variable {
			exception = bindings[name]
		} else if raw, known := types.RawClassFQN(thrown); known && constructorInvocationThrowsSimpleType(thrown, false) {
			exception = "L" + strings.ReplaceAll(raw, ".", "/") + ";"
		}
		if exception == "" || !q.assignable(exception, "Ljava/lang/Throwable;") {
			return nil, false
		}
		exceptions = append(exceptions, callbinding.Name(exception))
	}
	return slices.Clone(exceptions), !q.exhausted && nativeProofWork(work, int64(len(exceptions))+1)
}

func constructorInvocationThrowsSimpleType(t types.JavaType, variable bool) bool {
	if t == nil || t.IsArray() {
		return false
	}
	if _, parameterized := types.AsParameterizedType(t); parameterized {
		return false
	}
	if _, wildcard := t.(*types.JavaWildcardType); wildcard {
		return false
	}
	_, formal := types.TypeVariableName(t)
	return variable == formal
}
