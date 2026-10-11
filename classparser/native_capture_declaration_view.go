package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A factory's concrete result is not necessarily the local's source type.
// javac captures the declared local type, recorded in the original synthetic
// field. Restore only a stable, singly initialized reference declaration whose
// entire identity can widen without a cast. Parameters, generic fields and
// already solved declaration views need their own declaration proof.
func (c *ClassObjectDumper) nativeCaptureWidenedDeclaration(body []statements.Statement, ref *values.JavaRef, declaration *statements.AssignStatement, child *nativeAnonymousClass, allocation *values.NewExpression, field, actual, expected string) bool {
	if c == nil || c.FuncCtx == nil || c.obj == nil || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.WebDeclType != nil || declaration == nil || declaration.JavaValue == nil || child == nil || child.object == nil || !strings.HasPrefix(actual, "L") || !strings.HasPrefix(expected, "L") || actual == expected {
		return false
	}
	if _, _, err := callbinding.Descriptor("(" + actual + expected + ")V"); err != nil {
		return false
	}
	if original, known := values.SourceTypeErasure(declaration.JavaValue.Type(), c.FuncCtx); !known || original != actual {
		return false
	}
	if proved, stable := nativeCaptureJoinedDeclaration(body, ref, allocation, c.Work); !stable || proved != declaration {
		return false
	}
	matches := 0
	for _, f := range child.object.Fields {
		if f == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(child.object, f.NameIndex)
		if !nk {
			return false
		}
		if n != field {
			continue
		}
		d, dk := sourceBridgeUTF8(child.object, f.DescriptorIndex)
		flags, onlySynthetic, known := nativeMemberEffectiveFieldFlags(f, c.Work)
		if !dk || d != expected || !known || !onlySynthetic || flags != 0x1010 {
			return false
		}
		matches++
	}
	if matches != 1 {
		return false
	}
	provider := c.FuncCtx.InvocationMetadata
	if provider == nil {
		return false
	}
	remaining := 4096
	bounded := func(name string) (callbinding.Class, bool) {
		remaining--
		if remaining < 0 || !nativeProofWork(c.Work, 1) {
			return callbinding.Class{}, false
		}
		v, known := provider(name)
		return v, known && v.Name == name && v.ParentsComplete
	}
	target, known := bounded(callbinding.Name(expected))
	if !known || !callbinding.Assignable(actual, expected, bounded) {
		return false
	}
	formals := types.ClassFormalTypeParamNames(target.Signature)
	if target.Signature != "" {
		own, references, valid := types.SignatureTypeVariableReferences(target.Signature)
		if !valid || len(own) == 0 && len(references) != 0 {
			return false
		}
	}
	typ, err := types.ParseDescriptor(expected)
	if err != nil || typ == nil {
		return false
	}
	if len(formals) != 0 {
		var proved bool
		// A selected factory can supply a full invariant source result even
		// though its computational Type() deliberately remains descriptor-raw.
		// Project that result through original generic inheritance edges; never
		// erase arguments merely because the capture field records an erasure.
		if call, ok := declaration.JavaValue.(*values.FunctionCallExpression); ok && target.Public && !strings.Contains(target.Name, "$") {
			if source := call.SourceInvocationResultType(c.FuncCtx); source != nil {
				typ, proved = types.ProjectInstantiatedSupertype(c.FuncCtx, source, target.Name, bounded)
			}
		}
		if !proved {
			resolve := c.nativeAnnotationDeclarationResolver()
			definition, known := resolve(target.Name)
			if !known || definition == nil || definition.GetClassName() != target.Name || !nativeMemberTopLevelEvidence(definition, c.Work) ||
				definition.AccessFlags&1 == 0 && nativeBinaryPackage(target.Name) != nativeBinaryPackage(c.obj.GetClassName()) {
				return false
			}
			typ, proved = c.nativeCapturedAnonymousParentType(declaration, actual, expected, target, definition, bounded)
		}
		if !proved {
			return false
		}
	} else {
		// A non-public or nested declaration still needs its original source
		// ownership/access route, rather than a same-name metadata table.
		resolve := c.nativeAnnotationDeclarationResolver()
		definition, known := resolve(target.Name)
		if !known || definition == nil || definition.GetClassName() != target.Name || !nativeMemberTopLevelEvidence(definition, c.Work) || definition.AccessFlags&1 == 0 && nativeBinaryPackage(target.Name) != nativeBinaryPackage(c.obj.GetClassName()) {
			return false
		}
	}
	refs := map[*values.JavaRef]bool{}
	activeValues := map[values.JavaValue]bool{}
	activeStatements := map[statements.Statement]bool{}
	var value func(values.JavaValue) bool
	value = func(v values.JavaValue) bool {
		remaining--
		if remaining < 0 || sourceProofNil(v) || activeValues[v] || !nativeProofWork(c.Work, 1) {
			return false
		}
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				return false
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		activeValues[v] = true
		defer delete(activeValues, v)
		if r, ok := v.(*values.JavaRef); ok && r.Id == ref.Id {
			if r.IsThis || r.IsParam || r.CustomValue != nil || r.StackVar != nil || r.WebDeclType != nil {
				return false
			}
			original, known := values.SourceTypeErasure(r.Type(), c.FuncCtx)
			if !known || original != actual && original != expected {
				return false
			}
			refs[r] = true
		}
		if call, ok := v.(*values.FunctionCallExpression); ok {
			if !call.IsStatic && !value(call.Object) {
				return false
			}
			for _, arg := range call.Arguments {
				if !value(arg) {
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
			if !value(child) {
				return false
			}
		}
		return true
	}
	var walk func([]statements.Statement) bool
	walk = func(list []statements.Statement) bool {
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				return false
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		for _, st := range list {
			remaining--
			if remaining < 0 || sourceProofNil(st) || activeStatements[st] || !nativeProofWork(c.Work, 1) {
				return false
			}
			activeStatements[st] = true
			roots, lists, known := nativeSourceNameChildren(st)
			if !known {
				return false
			}
			for _, root := range roots {
				if !value(root) {
					return false
				}
			}
			for _, nested := range lists {
				if !walk(nested) {
					return false
				}
			}
			delete(activeStatements, st)
		}
		return true
	}
	if !walk(body) || !refs[ref] || c.Work != nil && c.Work.CheckAlloc(int64(len(refs))*128) != nil {
		return false
	}
	// All copies carry the same declaration identity. Repoint their source type
	// without mutating the initializer's type or adding a runtime conversion.
	for r := range refs {
		r.ResetVarType(typ.Copy())
		r.WebDeclType = typ.Copy()
	}
	return true
}
