package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A source root is not a lexical member. Keep its access bridges separately;
// adding a fictitious member would corrupt ownership and enclosing operands.
func (p *nativeMemberFamily) constructorBridges(owner string) map[string]*nativeConstructorAccessBridge {
	if p == nil {
		return nil
	}
	if owner == p.owner {
		return p.rootAccessBridges
	}
	if child := p.children[owner]; child != nil {
		return child.accessBridges
	}
	return nil
}

func (p *nativeMemberFamily) bridgeOwners() map[string]map[string]*nativeConstructorAccessBridge {
	out := map[string]map[string]*nativeConstructorAccessBridge{}
	if p == nil {
		return out
	}
	if len(p.rootAccessBridges) > 0 {
		out[p.owner] = p.rootAccessBridges
	}
	for owner, child := range p.children {
		if child == nil {
			out[owner] = nil
			continue
		}
		out[owner] = child.accessBridges
	}
	return out
}

type nativeRootBridgeDelegation struct {
	caller                    *ClassObject
	owner, descriptor, target string
	pc                        int
}

func nativeRootBridgeDelegationKey(owner, descriptor string) string {
	return owner + "\x00" + descriptor
}

// This capability covers a named member's initial SUPER call into the root
// or another owned static member. The target has no enclosing-instance word.
// A nonstatic caller separately proves its original capture before delegating;
// neither that caller capture nor lexical access changes the target packet.
// Private access still needs the same lexical
// transaction and original unused-marker certificate as a root constructor.
// The existing symbolic packet interpreter proves original uninitialized THIS,
// exact parameter origins and the target descriptor. Arbitrary allocations,
// anonymous callers and nonstatic target captures require their own proof.
func (c *ClassObjectDumper) proveNativeRootBridgeDelegations(p *nativeMemberFamily) bool {
	p.rootBridgeDelegations = map[string]*nativeRootBridgeDelegation{}
	metadata := c.buildInvocationMetadata()
	for owner, child := range p.children {
		targetOwner := child.object.GetSupperClassName()
		parent := p.children[targetOwner]
		if targetOwner != p.owner && (parent == nil || !parent.static) {
			continue
		}
		bridges := p.constructorBridges(targetOwner)
		if len(bridges) == 0 {
			continue
		}
		var original *nativeMemberClass
		if !child.static {
			// Reconstruct the capture and constructor from the original named
			// declaration graph. A cached capture PC or source role alone cannot
			// turn an ordinary parameter into this caller's lexical enclosing word.
			if p.lexicalObjects[owner] != child.object || p.lexicalObjects[child.owner] == nil {
				return false
			}
			original = nativeMemberProofWithDeclarations(child.object, p.lexicalObjects[child.owner], c.Work, child.accessBridges, p.lexicalObjects, c.nativeAnnotationDeclarationResolver(), metadata)
			if original == nil || original.static || original.owner != child.owner || original.name != child.name || original.flags != child.flags || original.field != child.field {
				return false
			}
		}
		for _, method := range child.object.Methods {
			name, nok := sourceBridgeUTF8(child.object, method.NameIndex)
			desc, dok := sourceBridgeUTF8(child.object, method.DescriptorIndex)
			if !nok || !dok || !nativeProofWork(c.Work, 1) {
				return false
			}
			if name != "<init>" || child.accessBridges[desc] != nil {
				continue
			}
			params, ret, err := callbinding.Descriptor(desc)
			if err != nil || ret != "V" {
				return false
			}
			seen := false
			for _, attr := range method.Attributes {
				code, ok := attr.(*CodeAttribute)
				if !ok {
					continue
				}
				if seen || !nativeProofWork(c.Work, int64(len(code.Code))) {
					return false
				}
				seen = true
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(child.object.ConstantPool, i) })
				d.Work = c.Work
				if d.ParseOpcode() != nil {
					return false
				}
				ops := constructorMotionOps(d)
				start := 0
				if original != nil {
					actual, cached := original.constructors[desc], child.constructors[desc]
					if actual == nil || cached == nil || actual.descriptor != cached.descriptor || actual.sourceDescriptor != cached.sourceDescriptor || actual.capturePC != cached.capturePC || actual.delegatePC != cached.delegatePC || actual.delegateOwner != cached.delegateOwner || actual.delegateDescriptor != cached.delegateDescriptor {
						return false
					}
					if actual.capturePC < 0 {
						// A THIS chain keeps its own physical enclosing argument;
						// only its terminal constructor records a SUPER capability.
						continue
					}
					start = -1
					for i, op := range ops {
						if int(op.CurrentOffset) == actual.capturePC {
							start = i + 1
							break
						}
					}
					if start < 0 {
						return false
					}
				}
				next, call := constructorMotionDelegation(child.object, ops, start, params, constructorParameterSlots(params), metadata)
				if call == nil {
					continue
				}
				bridge := bridges[call.Description]
				if call.Name != targetOwner || bridge == nil {
					continue
				}
				if next < 2 || ops[next-2].Instr.OpCode != core.OP_ACONST_NULL || len(ops[next-2].Data) != 0 {
					return false
				}
				// Java cannot wrap the initial super argument/delegation packet
				// in this constructor's catch clause. Retain refusal for a valid
				// JVM producer that supplies that additional exception boundary.
				end := int(ops[next-1].CurrentOffset) + 3
				for _, handler := range code.ExceptionTable {
					if handler == nil || !nativeProofWork(c.Work, 1) || int(handler.StartPc) < end && handler.EndPc > 0 {
						return false
					}
				}
				key := nativeRootBridgeDelegationKey(owner, desc)
				if p.rootBridgeDelegations[key] != nil {
					return false
				}
				p.rootBridgeDelegations[key] = &nativeRootBridgeDelegation{caller: child.object, owner: targetOwner, descriptor: call.Description, target: bridge.target, pc: int(ops[next-1].CurrentOffset)}
			}
		}
	}
	return true
}

func (p *nativeMemberFamily) rootBridgeDelegation(obj *ClassObject, name, descriptor, targetOwner, targetDescriptor string, pc int) *nativeRootBridgeDelegation {
	if p == nil || obj == nil || name != "<init>" {
		return nil
	}
	plan := p.rootBridgeDelegations[nativeRootBridgeDelegationKey(obj.GetClassName(), descriptor)]
	if plan == nil || plan.caller != obj || plan.owner != targetOwner || plan.descriptor != targetDescriptor || plan.pc != pc {
		return nil
	}
	return plan
}

func nativeRootBridgeSourceDelegation(p *nativeMemberFamily, obj *ClassObject, ctx, binding *class_context.ClassContext, owner, descriptor string, pc int, args []any) (string, bool) {
	if p == nil || ctx == nil || binding == nil || p.failed {
		return "", false
	}
	name := strings.ReplaceAll(owner, ".", "/")
	plan := p.rootBridgeDelegation(obj, ctx.FunctionName, ctx.CurrentMethodDesc, name, descriptor, pc)
	if plan == nil {
		return "", false
	}
	fail := func() (string, bool) { p.failed = true; return "", false }
	if binding.InvocationMetadata == nil {
		return fail()
	}
	declaration, known := binding.InvocationMetadata(owner)
	if !known || !declaration.MembersComplete || p.failed {
		return fail()
	}
	targets := 0
	for _, method := range declaration.Methods {
		if method.Name == "<init>" && method.Desc == plan.target {
			targets++
		}
	}
	if targets != 1 {
		return fail()
	}
	if len(args) == 0 || !nativeMemberBridgeSourceDummy(args[len(args)-1]) {
		return fail()
	}
	call := &values.FunctionCallExpression{ClassName: owner, FunctionName: "<init>", Descriptor: plan.target, Kind: values.InvokeSpecial, IsSpecialInvoke: true}
	for _, arg := range args[:len(args)-1] {
		value, ok := arg.(values.JavaValue)
		if !ok {
			return fail()
		}
		call.Arguments = append(call.Arguments, value)
	}
	mt, err := types.ParseMethodDescriptor(plan.target)
	if err != nil || len(mt.FunctionType().ParamTypes) != len(call.Arguments) {
		return fail()
	}
	call.FuncType = mt.FunctionType()
	copy := *ctx
	copy.InvocationMetadata = binding.InvocationMetadata
	copy.SiblingClassSig = binding.SiblingClassSig
	arguments := call.ArgumentStrings(&copy)
	if p.failed {
		return fail()
	}
	return "super(" + strings.Join(arguments, ",") + ")" + nativeMemberConstructorRegistration(p, name, descriptor), true
}

func nativeMemberJointBridgeEquivalent(p *nativeMemberFamily, obj *ClassObject, m *MemberInfo, desc string, work *workbudget.Budget) bool {
	if p == nil || obj == nil {
		return false
	}
	b := p.constructorBridges(obj.GetClassName())[desc]
	if b == nil {
		return false
	}
	reader := NewClassObjectDumper(obj)
	reader.Work = work
	fresh := reader.nativeConstructorAccessBridges()[desc]
	return fresh != nil && fresh.method == m && fresh.target == b.target && fresh.marker == b.marker
}

// Root allocations share the original private descriptor binding, but have no
// enclosing-instance parameter or lexical member name. Only the proved unused
// marker is erased, after the original allocation and call PCs have matched.
func nativeRootBridgeSourceAllocation(plan *nativeMemberAllocation, args []class_context.SourceCaptureOperand, ctx, binding *class_context.ClassContext, p *nativeMemberFamily) (string, bool) {
	fail := func() (string, bool) {
		if p != nil {
			p.failed = true
		}
		return "", false
	}
	if plan == nil || plan.rootObject == nil || p == nil || p.failed || ctx == nil || binding == nil || plan.rootObject != p.lexicalObjects[p.owner] || binding.InvocationMetadata == nil {
		return fail()
	}
	bridge := p.rootAccessBridges[plan.descriptor]
	if bridge == nil || len(args) == 0 || !nativeMemberBridgeSourceDummy(args[len(args)-1].Value) {
		return fail()
	}
	owner := strings.ReplaceAll(p.owner, "/", ".")
	declaration, known := binding.InvocationMetadata(owner)
	if !known || !declaration.MembersComplete || p.failed {
		return fail()
	}
	count := 0
	for _, m := range declaration.Methods {
		if m.Name == "<init>" && m.Desc == bridge.target {
			count++
		}
	}
	if count != 1 {
		return fail()
	}
	invoke := &values.FunctionCallExpression{ClassName: owner, FunctionName: "<init>", Descriptor: bridge.target, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: plan.invokePC}
	for _, arg := range args[:len(args)-1] {
		v, ok := arg.Value.(values.JavaValue)
		if !ok {
			return fail()
		}
		invoke.Arguments = append(invoke.Arguments, v)
	}
	mt, err := types.ParseMethodDescriptor(bridge.target)
	if err != nil || len(mt.FunctionType().ParamTypes) != len(invoke.Arguments) {
		return fail()
	}
	invoke.FuncType = mt.FunctionType()
	allocationBinding := *ctx
	allocationBinding.InvocationMetadata = binding.InvocationMetadata
	allocationBinding.SiblingClassSig = binding.SiblingClassSig
	arguments := invoke.ArgumentStrings(&allocationBinding)
	if p.failed {
		return fail()
	}
	name := ctx.ShortTypeName(owner)
	node := &values.NewExpression{JavaType: types.NewJavaClass(owner), ConstructorCall: invoke}
	diamond := node.SourceConstructorDiamond(&allocationBinding)
	source := "new " + name + diamond + "(" + strings.Join(arguments, ",") + ")" + nativeMemberConstructorRegistration(p, p.owner, plan.descriptor)
	if diamond != "" {
		source = "((" + name + ")(" + source + "))"
	}
	return source, true
}
