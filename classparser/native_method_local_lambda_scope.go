package javaclassparser

import (
	"fmt"
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Lexical metadata and physical allocation bodies are separate original facts.
func (local *nativeMethodLocalClass) executionOwner() *nativeMethodLocalOwner {
	if local.allocationOwner != nil {
		return local.allocationOwner
	}
	return local.owner
}

// Discover the original constructor user without moving an instruction. A
// different method must be the implementation of one original metafactory in
// the lexical method. Source placement, captures and archive closure still
// require their independent proofs. A type-only local retains its lexical role.
func (c *ClassObjectDumper) nativeMethodLocalExecutionOwner(local *ClassObject, owner *nativeMethodLocalOwner) (*nativeMethodLocalOwner, bool) {
	if c == nil || c.obj == nil || local == nil || owner == nil || len(c.obj.Methods) > 4096 {
		return nil, false
	}
	actual, known := originalMethodLocalOwner(local, c.obj, c.Work)
	if !known || *actual != *owner {
		return nil, false
	}
	var chosen *MemberInfo
	bytes := 0
	// Keep the established direct-method domain cheap. Other users remain
	// obligations of the complete archive closure.
	ordered := []*MemberInfo{owner.declaration}
	for _, m := range c.obj.Methods {
		if m != owner.declaration {
			ordered = append(ordered, m)
		}
	}
	for _, m := range ordered {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		codeSeen := false
		for _, a := range m.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			if codeSeen || code == nil || len(code.Code) > (1<<20)-bytes {
				return nil, false
			}
			codeSeen = true
			bytes += len(code.Code)
			if !nativeProofWork(c.Work, int64(len(code.Code))) || c.Work != nil && c.Work.CheckAlloc(int64(len(code.Code)+1)*64) != nil {
				return nil, false
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
			d.Work = c.Work
			if d.ParseOpcode() != nil {
				return nil, false
			}
			for _, op := range d.Opcodes() {
				if op == nil || op.Instr == nil || !nativeProofWork(c.Work, 1) {
					return nil, false
				}
				if op.Instr.OpCode != core.OP_INVOKESPECIAL {
					continue
				}
				member := constructorMotionMember(c.obj, op, core.OP_INVOKESPECIAL)
				if member != nil && member.Name == local.GetClassName() && member.Member == "<init>" {
					if chosen != nil && chosen != m {
						return nil, false
					}
					chosen = m
				}
			}
		}
		if chosen == owner.declaration {
			return owner, true
		}
	}
	if chosen == nil || chosen == owner.declaration {
		return owner, true
	}
	// Native javac8 records the physical implementation as EnclosingMethod;
	// modern javac records this original lexical factory. A source release is
	// not a compiler-lowering certificate. Preserve the direct legacy path above.
	if c.options.SourceCompiler != "" && c.options.SourceCompiler != ModernJavac || c.options.TargetSourceVersion != 0 && c.options.TargetSourceVersion < 8 || c.obj.MajorVersion < 52 || local.MajorVersion != c.obj.MajorVersion || c.obj.MinorVersion != 0 || local.MinorVersion != 0 {
		return nil, false
	}
	n, nk := sourceBridgeUTF8(c.obj, chosen.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, chosen.DescriptorIndex)
	context := nativeLambdaImplementationContext{resolve: c.nativeAnnotationDeclarationResolver(), localCaptures: map[string]*nativeLambdaLocalCaptureSite{}, factorySites: map[string][]*nativeLambdaLocalCaptureSite{}}
	if !nk || !dk || !nativeLambdaImplementationScope(c.obj, n, desc, owner.method+owner.descriptor, true, c.Work, context) {
		return nil, false
	}
	sites := context.factorySites[n+desc]
	if len(sites) != 1 || sites[0] == nil || sites[0].method != owner.declaration {
		return nil, false
	}
	result := *owner
	result.method = n
	result.descriptor = desc
	result.declaration = chosen
	return &result, true
}

// Source installation and publication reopen the original cross-method edge;
// a cached target or the implementation's synthetic flag cannot replace it.
func (c *ClassObjectDumper) nativeMethodLocalExecutionValid(local *nativeMethodLocalClass) bool {
	if local == nil || local.owner == nil {
		return false
	}
	if local.allocationOwner == nil || local.allocationOwner == local.owner {
		return true
	}
	actual, known := c.nativeMethodLocalExecutionOwner(local.object, local.owner)
	if !known || actual == nil || *actual != *local.allocationOwner {
		return false
	}
	// Recompute the original typed allocation vectors at stage boundaries. An
	// equal-erasure port permutation must not borrow the cached method's scope.
	sites, known := c.nativeMethodLocalAllocationFacts(local.object, local.owner, local.constructor, true)
	return known && reflect.DeepEqual(sites, local.allocations)
}

// Allocate generated locals by declaration identity before rendering. A local
// class can make an original SAM binder observable through val$ fields. Its
// name, strings and child declarations cannot pass through a text-wide rename.
func (c *ClassObjectDumper) prepareNativeLambdaMethodLocalNamespace(body []statements.Statement, params []values.JavaValue) bool {
	if c.nativeMemberRoot == nil || c.FuncCtx == nil {
		return false
	}
	protected := map[*coreutils.VariableId]bool{}
	for _, v := range params {
		if ref, ok := v.(*values.JavaRef); ok && ref != nil && ref.Id != nil {
			protected[ref.Id] = true
		}
	}
	found := false
	for _, local := range c.nativeMemberRoot.methodLocals {
		if local == nil || local.owner == nil || local.owner.owner != c.obj.GetClassName() || local.executionOwner().method != c.FuncCtx.FunctionName || local.executionOwner().descriptor != c.FuncCtx.CurrentMethodDesc {
			continue
		}
		found = true
		for _, id := range local.captureIDs {
			if id != nil {
				protected[id] = true
			}
		}
	}
	if !found {
		return false
	}
	c.lambdaLocalSeq++
	c.prepareNativeSourceNamesInNamespace(body, params, c.nativeLexicalCaptures(), protected, fmt.Sprintf("lv%d_", c.lambdaLocalSeq))
	return true
}
