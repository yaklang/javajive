package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// Java groups zero-argument overrides without their return descriptor. A
// directly declared public getter remains the source target on a raw receiver
// when it overrides every ancestor candidate and every same-owner alternative
// is an independently certified, unchanged-argument covariant forwarder.
// ACC_BRIDGE alone is never enough to discard an executable method.
func erasedZeroArgumentOverrideFamily(f *FunctionCallExpression, ctx *class_context.ClassContext) bool {
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	meta, known := ctx.InvocationMetadata(owner)
	if !known || meta.Name != owner || !meta.MembersComplete || !meta.ParentsComplete {
		return false
	}
	var target *callbinding.Method
	for i := range meta.Methods {
		m := &meta.Methods[i]
		if m.Name == f.FunctionName && m.Desc == f.Descriptor {
			if target != nil {
				return false
			}
			target = m
		}
	}
	if target == nil || !target.AccessFlagsKnown || target.AccessFlags&0xf != 1 || target.Static || target.Bridge || target.Varargs {
		return false
	}
	params, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(params) != 0 || !callbinding.Reference(result) {
		return false
	}
	active, done := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(name string) bool {
		if active[name] || len(done)+len(active) >= 64 {
			return false
		}
		if done[name] {
			return true
		}
		c, available := ctx.InvocationMetadata(name)
		if !available || c.Name != name || !c.MembersComplete || !c.ParentsComplete {
			return false
		}
		active[name] = true
		seen := map[string]bool{}
		for _, m := range c.Methods {
			if m.Name != f.FunctionName {
				continue
			}
			ps, ret, e := callbinding.Descriptor(m.Desc)
			if e != nil || seen[m.Desc] || m.Varargs {
				return false
			}
			seen[m.Desc] = true
			if len(ps) != 0 || name == owner && m.Desc == f.Descriptor {
				continue
			}
			if !m.AccessFlagsKnown || m.AccessFlags&0xf != 1 || m.Static || !callbinding.Reference(ret) || !callbinding.Assignable(result, ret, ctx.InvocationMetadata) {
				return false
			}
			if name == owner {
				if !m.Bridge || m.AccessFlags != 0x1041 || m.CovariantBridgeTarget != f.Descriptor {
					return false
				}
			} else if m.AccessFlags&0x10 != 0 || strings.HasPrefix(m.Signature, "<") {
				// Final methods cannot be overridden. Method formals require a
				// separate override/subsignature proof; class formals are erased
				// only on the already established raw receiver view.
				return false
			}
		}
		for _, parent := range c.Parents {
			if !visit(parent) {
				return false
			}
		}
		delete(active, name)
		done[name] = true
		return true
	}
	return visit(owner)
}
