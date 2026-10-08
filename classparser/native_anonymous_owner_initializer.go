package javaclassparser

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An anonymous class with a zero EnclosingMethod index must remain in an
// initializer, even when its bytecode physically occurs in <init>. Retain the
// entire post-super prefix through its last allocation as an instance block.
// This keeps preceding calls, field stores and abrupt completion in order;
// hoisting only the allocating assignment would not do so.
//
// This certificate covers one physical initializing constructor, including
// overloads proved to reach it through THIS delegation. Common prefixes copied
// across super-delegating constructors need a separate equivalence proof. The
// prefix may use THIS and fields, but no constructor parameter or local whose
// declaration would cross the new source scope.
func (c *ClassObjectDumper) nativeAnonymousOwnerInitializerPrefix(body []statements.Statement) (int, error) {
	p := c.nativeAnonymousRoot
	if p == nil || c.FuncCtx == nil || c.FuncCtx.FunctionName != "<init>" {
		return 0, nil
	}
	required := map[string]*nativeAnonymousClass{}
	for name, child := range p.children {
		if child != nil && child.method == "" {
			static, known := nativeAnonymousOriginalContext(c.obj, child.object, "", c.Work)
			if !known {
				p.failed = true
				return 0, fmt.Errorf("unproved original anonymous initializer allocation context")
			}
			if !static {
				required[name] = child
			}
		}
	}
	if len(required) == 0 {
		return 0, nil
	}
	fail := func() (int, error) {
		p.failed = true
		return 0, fmt.Errorf("unproved original anonymous instance initializer scope")
	}
	constructors := 0
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return fail()
		}
		name, known := sourceBridgeUTF8(c.obj, method.NameIndex)
		if !known {
			return fail()
		}
		if name == "<init>" {
			constructors++
		}
	}
	if constructors != 1 {
		initializing, closed := c.nativeAnonymousInitializerDelegation(required)
		if !closed {
			return fail()
		}
		if !initializing {
			return 0, nil
		}
	}
	remaining := 4096
	active := map[values.JavaValue]bool{}
	seen := map[string]bool{}
	var visit func(values.JavaValue) bool
	visit = func(v values.JavaValue) bool {
		if v == nil {
			return true // absent static-call receiver
		}
		if sourceProofNil(v) || remaining <= 0 || active[v] || !nativeProofWork(c.Work, 1) {
			return false
		}
		remaining--
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				return false
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		active[v] = true
		defer delete(active, v)
		if ref, ok := v.(*values.JavaRef); ok && (!ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil) {
			return false
		}
		if allocation, ok := v.(*values.NewExpression); ok && allocation.ConstructorCall != nil {
			call := allocation.ConstructorCall
			name := strings.ReplaceAll(call.ClassName, ".", "/")
			if child := required[name]; child != nil {
				if seen[name] || !allocation.HasOriginPC || !call.HasOriginPC || call.Object != allocation || call.Kind != values.InvokeSpecial || !call.IsSpecialInvoke || call.FunctionName != "<init>" || allocation.OriginPC != child.newPC || call.OriginPC != child.invokePC || call.Descriptor != child.descriptor {
					return false
				}
				params, result, err := callbinding.Descriptor(child.descriptor)
				if err != nil || result != "V" || len(params) != len(call.Arguments) {
					return false
				}
				seen[name] = true
			}
		}
		if call, ok := v.(*values.FunctionCallExpression); ok && call.FunctionName == "<init>" {
			return false // delegation is handled separately; NEW owns its arguments
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !visit(child) {
				return false
			}
		}
		return true
	}
	last := 0
	for i, st := range body {
		if sourceProofNil(st) || !nativeProofWork(c.Work, 1) {
			return fail()
		}
		if marker, ok := st.(*statements.MiddleStatement); ok && (marker.Flag == "start" || marker.Flag == "end") {
			continue
		}
		if expr, ok := st.(*statements.ExpressionStatement); ok && i == 0 {
			if call, ok := values.UnpackSoltValue(expr.Expression).(*values.FunctionCallExpression); ok && call.IsSpecialInvoke && call.FunctionName == "<init>" {
				if strings.ReplaceAll(call.ClassName, ".", "/") != c.obj.GetSupperClassName() {
					return fail()
				}
				continue // delegation stays in the constructor
			}
		}
		switch st.(type) {
		case *statements.AssignStatement, *statements.ExpressionStatement:
		default:
			return fail()
		}
		roots, blocks, known := nativeSourceNameChildren(st)
		if !known || len(blocks) != 0 || len(c.nativeMethodLocalPlacements[st]) != 0 {
			return fail()
		}
		for _, root := range roots {
			if !visit(root) {
				return fail()
			}
		}
		if len(seen) == len(required) {
			last = i + 1
			break
		}
	}
	if last == 0 {
		return fail()
	}
	return last, nil
}
