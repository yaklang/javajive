package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A non-generic member constructor can still use its enclosing declaration's
// type variables. The original call uses their erasures. A raw, exact THIS
// qualifier gives javac that same signature (JLS 4.8), without inventing a cast
// to a merely same-spelled caller type variable. Explicit qualification adds
// a null check, so this proof deliberately excludes nullable captured outers.
func nativeMemberConstructorRawThis(p *nativeMemberFamily, child *nativeMemberClass, physicalDescriptor, caller string, operand class_context.SourceCaptureOperand, ctx *class_context.ClassContext, work *workbudget.Budget) string {
	if p == nil || child == nil || child.object == nil || child.static || child.formalCount != 0 || caller != child.owner || ctx == nil || ctx.IsStatic || !operand.Receiver {
		return ""
	}
	v, ok := operand.Value.(values.JavaValue)
	if !ok {
		return ""
	}
	ref, ok := values.UnpackSoltValue(v).(*values.JavaRef)
	if !ok || ref == nil || !ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
		return ""
	}
	if bridge := child.accessBridges[physicalDescriptor]; bridge != nil {
		physicalDescriptor = bridge.target
	}
	signature := ""
	found := false
	for _, m := range child.object.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return ""
		}
		name, _ := sourceBridgeUTF8(child.object, m.NameIndex)
		desc, _ := sourceBridgeUTF8(child.object, m.DescriptorIndex)
		if name != "<init>" || desc != physicalDescriptor {
			continue
		}
		if found {
			return ""
		}
		found = true
		seenSignature := false
		for _, a := range m.Attributes {
			if s, ok := a.(*SignatureAttribute); ok {
				if s == nil || seenSignature {
					return ""
				}
				seenSignature = true
				var valid bool
				signature, valid = sourceBridgeUTF8(child.object, s.SignatureIndex)
				if !valid || !nativeProofWork(work, int64(len(signature))) {
					return ""
				}
			}
		}
	}
	methodFormals, _, valid := types.SignatureTypeVariableReferences(signature)
	if !valid {
		return ""
	}
	start, end := strings.IndexByte(signature, '('), strings.IndexByte(signature, ')')
	if start < 0 || end <= start {
		return ""
	}
	_, references, valid := types.SignatureTypeVariableReferences(signature[start:end+1] + "V")
	if !valid {
		return ""
	}
	owner := p.lexicalObjects[child.owner]
	scope, valid := nativeMemberLexicalTypeScope(owner, p.lexicalObjects, work)
	if !valid {
		return ""
	}
	methodOwned := map[string]bool{}
	for _, n := range methodFormals {
		methodOwned[n] = true
	}
	for _, n := range references {
		if scope[n] && !methodOwned[n] {
			// The renderer has already proved the operand's exact erased
			// owner. THIS is nonnull, so javac's qualifier check cannot add
			// an exception ahead of any original argument producer.
			return "((" + ctx.ShortTypeName(strings.ReplaceAll(child.owner, "/", ".")) + ")(" + operand.Text + "))"
		}
	}
	return ""
}
