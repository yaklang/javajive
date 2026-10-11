package javaclassparser

import (
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Register allocation capture names independently from the IR's merged local
// web. Distinct source locals may reuse a bytecode slot on exclusive edges. A
// single web name then cannot reproduce both original synthetic field names.
// At a direct return, a pure, definitely assigned local can instead be copied
// into a fresh source binder before NEW. This changes no producer evaluation,
// conversion, captured word, allocation position or superclass argument.
func (c *ClassObjectDumper) prepareNativeCaptureIdentitySnapshots(body []statements.Statement, params []values.JavaValue) ([]statements.Statement, bool) {
	if c == nil || c.FuncCtx == nil || c.nativeAnonymousRoot == nil {
		return body, true
	}
	relevant := false
	for _, child := range c.nativeAnonymousRoot.children {
		if child == nil || !nativeProofWork(c.Work, 1) {
			return body, false
		}
		if child.method == c.FuncCtx.FunctionName+c.FuncCtx.CurrentMethodDesc || child.method == "" && (c.FuncCtx.FunctionName == "<init>" || c.FuncCtx.FunctionName == "<clinit>") || nativeAnonymousAllocationScope(c.obj, child, c.FuncCtx.FunctionName, c.FuncCtx.CurrentMethodDesc, c.Work) {
			relevant = true
		}
	}
	if !relevant {
		return body, true
	}
	parameterIDs := map[*coreutils.VariableId]bool{}
	for _, param := range params {
		if ref, ok := param.(*values.JavaRef); ok && ref != nil && ref.Id != nil {
			parameterIDs[ref.Id] = true
		}
	}
	type site struct {
		allocation *values.NewExpression
		statement  *statements.ReturnStatement
	}
	sites := []site{}
	identityNames := map[*coreutils.VariableId]map[string]bool{}
	remaining := 16384
	activeValues := map[values.JavaValue]bool{}
	activeStatements := map[statements.Statement]bool{}
	seenAllocations := map[*values.NewExpression]bool{}
	enter := func() bool {
		remaining--
		return remaining >= 0 && nativeProofWork(c.Work, 1) && (c.Work == nil || c.Work.Enter(workbudget.CounterASTDepth) == nil)
	}
	leave := func() {
		if c.Work != nil {
			c.Work.Leave(workbudget.CounterASTDepth)
		}
	}
	var value func(values.JavaValue, *statements.ReturnStatement) bool
	value = func(v values.JavaValue, direct *statements.ReturnStatement) bool {
		if sourceProofNil(v) || activeValues[v] || !enter() {
			return false
		}
		defer leave()
		activeValues[v] = true
		defer delete(activeValues, v)
		if allocation, ok := v.(*values.NewExpression); ok && allocation.ConstructorCall != nil {
			call := allocation.ConstructorCall
			if child := c.nativeAnonymousRoot.children[strings.ReplaceAll(call.ClassName, ".", "/")]; child != nil {
				if seenAllocations[allocation] {
					return false
				}
				seenAllocations[allocation] = true
				sites = append(sites, site{allocation, direct})
				for field, index := range child.fields {
					if index < 0 || index >= len(call.Arguments) || !nativeProofWork(c.Work, 1) {
						return false
					}
					operand, bounded := nativeMemberEnclosingUnpack(call.Arguments[index], c.Work)
					if !bounded {
						return false
					}
					ref, ok := operand.(*values.JavaRef)
					name, named := strings.CutPrefix(field, "val$")
					if ok && ref != nil && ref.Id != nil && !ref.IsThis && named {
						if identityNames[ref.Id] == nil {
							identityNames[ref.Id] = map[string]bool{}
						}
						identityNames[ref.Id][name] = true
					}
				}
			}
		}
		if call, ok := v.(*values.FunctionCallExpression); ok {
			if !call.IsStatic && !value(call.Object, nil) {
				return false
			}
			for _, arg := range call.Arguments {
				if !value(arg, nil) {
					return false
				}
			}
			return true
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !value(child, nil) {
				return false
			}
		}
		return true
	}
	var walk func([]statements.Statement, bool) bool
	walk = func(list []statements.Statement, directContour bool) bool {
		if !enter() {
			return false
		}
		defer leave()
		for _, st := range list {
			if sourceProofNil(st) || activeStatements[st] || !nativeProofWork(c.Work, 1) {
				return false
			}
			activeStatements[st] = true
			roots, lists, known := nativeSourceNameChildren(st)
			if !known {
				return false
			}
			for _, root := range roots {
				var direct *statements.ReturnStatement
				if ret, ok := st.(*statements.ReturnStatement); ok && directContour && ret.JavaValue == root {
					direct = ret
				}
				if !value(root, direct) {
					return false
				}
			}
			_, conditional := st.(*statements.IfStatement)
			for _, nested := range lists {
				if !walk(nested, directContour && conditional) {
					return false
				}
			}
			delete(activeStatements, st)
		}
		return true
	}
	if !walk(body, true) {
		return body, false
	}
	type replacement struct {
		call  *values.FunctionCallExpression
		index int
		alias *values.JavaRef
	}
	replacements := []replacement{}
	prefixes := map[*statements.ReturnStatement][]statements.Statement{}
	for _, s := range sites {
		allocation, call := s.allocation, s.allocation.ConstructorCall
		child := c.nativeAnonymousRoot.children[strings.ReplaceAll(call.ClassName, ".", "/")]
		indices := map[int]string{}
		sources := map[int]*values.JavaRef{}
		for field, index := range child.fields {
			operand, bounded := nativeMemberEnclosingUnpack(call.Arguments[index], c.Work)
			if !bounded {
				return body, false
			}
			ref, ok := operand.(*values.JavaRef)
			if !ok || ref == nil || len(identityNames[ref.Id]) < 2 {
				continue
			}
			if s.statement == nil || parameterIDs[ref.Id] || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.Type() == nil || !allocation.HasOriginPC || !call.HasOriginPC || call.Object != allocation || call.Kind != values.InvokeSpecial || !call.IsSpecialInvoke || call.FunctionName != "<init>" || call.Descriptor != child.descriptor || allocation.OriginPC != child.newPC || call.OriginPC != child.invokePC {
				return body, false
			}
			allocated, known := types.RawClassFQN(allocation.Type())
			if !known || strings.ReplaceAll(allocated, ".", "/") != strings.ReplaceAll(call.ClassName, ".", "/") {
				return body, false
			}
			name, named := strings.CutPrefix(field, "val$")
			if !named || name == "" || class_context.SafeIdentifier(name) != name || indices[index] != "" {
				return body, false
			}
			parameters, _, err := callbinding.Descriptor(call.Descriptor)
			erasure, known := values.SourceTypeErasure(ref.Type(), c.FuncCtx)
			if err != nil || len(parameters) != len(call.Arguments) || index >= len(parameters) || !known || erasure != parameters[index] {
				return body, false
			}
			if _, stable := nativeCaptureJoinedDeclaration(body, ref, allocation, c.Work); !stable {
				return body, false
			}
			indices[index] = name
			sources[index] = ref
		}
		ordered := []int{}
		for index := range indices {
			ordered = append(ordered, index)
		}
		sort.Ints(ordered)
		for _, index := range ordered {
			ref := sources[index]
			alias := values.NewJavaRef(coreutils.NewRootVariableId(), nil, ref.Type().Copy())
			prefixes[s.statement] = append(prefixes[s.statement], &statements.AssignStatement{LeftValue: alias, JavaValue: ref, IsFirst: true})
			replacements = append(replacements, replacement{call, index, alias})
		}
	}
	if len(replacements) == 0 {
		return body, true
	}
	if c.Work != nil && c.Work.CheckAlloc(int64(16384+len(replacements))*128) != nil {
		return body, false
	}
	// Copy only list/conditional containers. No argument or source name changes
	// until every original site and its definite-assignment certificate close.
	var rewrite func([]statements.Statement) []statements.Statement
	rewrite = func(list []statements.Statement) []statements.Statement {
		out := make([]statements.Statement, 0, len(list))
		for _, st := range list {
			if ret, ok := st.(*statements.ReturnStatement); ok {
				out = append(out, prefixes[ret]...)
			}
			if branch, ok := st.(*statements.IfStatement); ok {
				copy := *branch
				copy.IfBody, copy.ElseBody = rewrite(branch.IfBody), rewrite(branch.ElseBody)
				st = &copy
			}
			out = append(out, st)
		}
		return out
	}
	result := rewrite(body)
	for _, r := range replacements {
		r.call.Arguments[r.index] = r.alias
	}
	return result, true
}
