package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// Reopen original qualified-THIS declaration evidence without formatting a
// source name. The committing renderer still checks the binding and complete
// family output. A type query grants neither a source spelling nor an effect.
func (c *ClassObjectDumper) nativeAnonymousLexicalThisRead(read *nativeMemberLexicalRead, value any, pc int, name string, ctx *class_context.ClassContext) bool {
	if c == nil || c.obj == nil || ctx == nil || read == nil || c.nativeAnonymousForest == nil || !nativeProofWork(c.Work, 1) {
		return false
	}
	if c.Work != nil && c.Work.CheckAlloc(64*96) != nil {
		return false
	}
	forest := c.nativeAnonymousForest
	p := forest.members
	current := c.obj.GetClassName()
	if p != nil && (p.failed || p.owner != forest.root || p.anonymousForest != forest) {
		return false
	}
	if strings.ReplaceAll(ctx.ClassName, ".", "/") != current || forest.objects[current] != c.obj || forest.reads[current][ctx.FunctionName+ctx.CurrentMethodDesc][pc] != read || !forest.lexicalThis[current][ctx.FunctionName+ctx.CurrentMethodDesc][pc] || read.pc != pc || name != read.field {
		return false
	}
	object := forest.objects[read.owner]
	owner := ""
	if object == nil || object.GetClassName() != read.owner || p != nil && p.lexicalObjects[read.owner] != object {
		return false
	}
	if child := forest.units[read.owner]; child != nil {
		// Anonymous THIS has no source name. Only its original capture of a
		// named enclosing instance contributes this declaration view.
		if child.object != object || child.parentAnonymous || child.enclosingField != name {
			return false
		}
		original, method, known := originalAnonymousOwner(object)
		group := forest.groups[original]
		if !known || method != child.method || group == nil || group.failed || group.forest != forest || group.children[read.owner] != child {
			return false
		}
		owner = original
	} else {
		if p == nil {
			return false
		}
		named := p.children[read.owner]
		if named == nil || named.object != object || named.static || named.field != name {
			return false
		}
		original, n, flags, known := originalMemberOwner(object)
		if !known || original != named.owner || n != named.name || flags != named.flags || flags&8 != 0 {
			return false
		}
		owner = original
	}
	if read.descriptor != "L"+owner+";" || forest.objects[owner] == nil || p != nil && p.lexicalObjects[owner] != forest.objects[owner] || !nativeProofWork(c.Work, int64(len(owner)+len(name)+len(read.descriptor))) {
		return false
	}
	if owner == forest.root && p == nil && !nativeMemberTopLevelEvidence(forest.objects[owner], c.Work) {
		return false
	}
	if owner != forest.root {
		if p == nil {
			return false
		}
		outer := p.children[owner]
		if outer == nil || outer.object == nil || outer.object != forest.objects[owner] {
			return false
		}
	}
	count := 0
	// A cached path cannot supply a removed or changed capture declaration.
	for _, f := range object.Fields {
		if f == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		n, known := sourceBridgeUTF8(object, f.NameIndex)
		if !known {
			return false
		}
		if n != name {
			continue
		}
		count++
		d, known := sourceBridgeUTF8(object, f.DescriptorIndex)
		flags, synth, valid := nativeMemberEffectiveFieldFlags(f, c.Work)
		if !known || d != read.descriptor || !valid || !synth || flags != 0x1010 {
			return false
		}
	}
	return count == 1 && nativeMemberLexicalReadOperand(value, read, c.Work, ctx)
}
