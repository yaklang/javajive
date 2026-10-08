package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

// javac injects an instance initializer into each super-delegating constructor,
// never into a this-delegating constructor. Multiple overloads therefore need
// no copied-prefix inference when their original delegation graph has exactly
// one super root. Prove every THIS initialization through immutable original
// verifier frames and retain its target descriptor. Missing targets, duplicate
// declarations, cycles and multiple roots cannot license source extraction.
func (c *ClassObjectDumper) nativeAnonymousInitializerDelegation(required map[string]*nativeAnonymousClass) (initializing, closed bool) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || c.FuncCtx.CurrentMethodDesc == "" || len(c.obj.Methods) > 256 || !nativeProofWork(c.Work, int64(len(c.obj.Methods))) {
		return false, false
	}
	if c.Work != nil && c.Work.CheckAlloc(int64(len(c.obj.Methods))*512) != nil {
		return false, false
	}
	edges := map[string]string{}
	root := ""
	for _, method := range c.obj.Methods {
		if method == nil {
			return false, false
		}
		name, known := sourceBridgeUTF8(c.obj, method.NameIndex)
		if !known {
			return false, false
		}
		if name != "<init>" {
			continue
		}
		desc, known := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
		_, ret, err := callbinding.Descriptor(desc)
		if !known || err != nil || ret != "V" || method.AccessFlags&(StaticFlag|0x0400|0x0100) != 0 || len(edges) >= 64 {
			return false, false
		}
		if _, duplicate := edges[desc]; duplicate {
			return false, false
		}
		var code *CodeAttribute
		for _, attribute := range method.Attributes {
			if candidate, ok := attribute.(*CodeAttribute); ok {
				if code != nil || candidate == nil {
					return false, false
				}
				code = candidate
			}
		}
		if code == nil || !nativeProofWork(c.Work, int64(len(code.Code))) {
			return false, false
		}
		ir, frames, known := c.nativeOriginalMethodSnapshot(method, code)
		if !known {
			return false, false
		}
		var delegate *methodir.Instr
		for _, record := range frames.Instructions {
			if !nativeProofWork(c.Work, 1) {
				return false, false
			}
			invoke, exists := ir.InstrByID(methodir.InstrID(record.PC))
			if !exists || invoke.Opcode != core.OP_INVOKESPECIAL || invoke.Member != "<init>" {
				continue
			}
			args, ret, err := callbinding.Descriptor(invoke.Desc)
			index := len(record.Before.Stack) - nativeMemberParameterWidth(args) - 1
			if err != nil || ret != "V" || index < 0 {
				return false, false
			}
			if record.Before.Stack[index].Kind != frametransfer.UninitThis {
				continue // an argument's fresh NEW is not THIS's delegation
			}
			after, _, err := frametransfer.Transfer(record.Before, frametransfer.FromIR(invoke))
			if err != nil || delegate != nil || !record.Before.ThisUninitialized || after.ThisUninitialized {
				return false, false
			}
			delegate = &invoke
		}
		if delegate == nil {
			return false, false
		}
		// No argument crosses this boundary. Calls before it retain their own
		// descriptor, effects and failures; identifying THIS needs no assertion
		// that an unavailable external declaration is pure or uniquely bound.
		if delegate.Class == c.obj.GetClassName() {
			edges[desc] = delegate.Desc
			continue
		}
		if delegate.Class != c.obj.GetSupperClassName() || root != "" {
			return false, false
		}
		root, edges[desc] = desc, ""
		seen := map[string]int{}
		for _, invoke := range ir.Instrs {
			if invoke.PC <= delegate.PC || invoke.Opcode != core.OP_INVOKESPECIAL || invoke.Member != "<init>" {
				continue
			}
			if child := required[invoke.Class]; child != nil {
				if child.invokePC != int(invoke.PC) || child.descriptor != invoke.Desc {
					return false, false
				}
				seen[invoke.Class]++
			}
		}
		for name := range required {
			if seen[name] != 1 {
				return false, false
			}
		}
	}
	if root == "" {
		return false, false
	}
	if _, known := edges[c.FuncCtx.CurrentMethodDesc]; !known {
		return false, false
	}
	for desc := range edges {
		seen := map[string]bool{}
		for desc != root {
			if seen[desc] || !nativeProofWork(c.Work, 1) {
				return false, false
			}
			seen[desc] = true
			target, known := edges[desc]
			if !known || target == "" {
				return false, false
			}
			desc = target
		}
	}
	return c.FuncCtx.CurrentMethodDesc == root, true
}
