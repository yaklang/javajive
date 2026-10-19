package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// An ordinary method declaration executes nothing at class/instance
// initialization. Its table position is not source-order evidence. Keep the
// complete original body/signature packet as an atom, and exclude declarations
// that allocate lexical names or contribute invokedynamic implementation names.
// Fields, initializer blocks and constructors receive no movement permission.
func (c *ClassObjectDumper) nativeMemberRegistrationMethods(methods []*dumpedMethods) map[string]bool {
	allowed := map[string]bool{}
	if c == nil || c.obj == nil || c.nativeMemberRoot == nil || len(c.nativeMemberRoot.getters) == 0 || c.nativeMemberRoot.owner != c.obj.GetClassName() || len(methods) > 4096 {
		return allowed
	}
	p := c.nativeMemberRoot
	lexical := map[string]bool{}
	if p.anonymousForest != nil {
		if group := p.anonymousForest.groups[p.owner]; group != nil {
			for _, child := range group.children {
				if child == nil {
					return nil
				}
				lexical[child.method] = true
			}
		}
	}
	for _, local := range p.methodLocals {
		if local == nil || local.owner == nil {
			return nil
		}
		if local.owner.owner == p.owner {
			lexical[local.owner.method+local.owner.descriptor] = true
		}
	}
	for _, body := range methods {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		// Event-free declarations cannot unlock a registration schedule.
		// This early refusal saves decoding; actual movement still requires
		// the complete typed packet and the quote-aware event inventory.
		if body == nil || !strings.Contains(body.code, "/*jdec-owned-getter:") && !strings.Contains(body.code, "/*"+nativeConstructorRegistrationPrefix) {
			continue
		}
		if body == nil || body.member == nil || body.code == "" || body.sourceDumper != c || body.sourceObject != c.obj || body.sourceRequest.role != methodSourceDeclaration || !body.sourceRequest.valid() || body.sourceCacheKey == "" || c.dumpedMethodsSet[body.sourceCacheKey] != body || body.sourceReconstructionFailed() || !c.methodParameterNameCacheClosed(body.parameterNameStamp) {
			continue
		}
		method := body.member
		name, named := sourceBridgeUTF8(c.obj, method.NameIndex)
		desc, typed := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
		if !named || !typed || name == "<init>" || name == "<clinit>" || body.methodName != name || isSyntheticMethod(method.AccessFlags) || lexical[name+desc] {
			continue
		}
		owned := 0
		for _, original := range c.obj.Methods {
			if !nativeProofWork(c.Work, 1) {
				return nil
			}
			if original == method {
				owned++
			}
		}
		if owned != 1 {
			continue
		}
		var code *CodeAttribute
		duplicate := false
		for _, attribute := range method.Attributes {
			if candidate, ok := attribute.(*CodeAttribute); ok {
				if code != nil {
					duplicate = true
				}
				code = candidate
			}
		}
		if code == nil || duplicate || len(code.Code) > 65535 || !nativeProofWork(c.Work, int64(len(code.Code))) {
			continue
		}
		decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
		decoder.Work = c.Work
		if decoder.ParseOpcode() != nil {
			continue
		}
		counter := false
		for _, op := range decoder.Opcodes() {
			if op == nil || op.Instr == nil || op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
				counter = true
				break
			}
		}
		text := strings.TrimSpace(body.code)
		// These are conservative refusals, never positive ownership evidence.
		// An unexpected nested source scope cannot borrow its parent's packet.
		if counter || strings.Contains(text, "jdec-owned-anonymous-ordinal:") || strings.Contains(text, "jdec-owned-local-") {
			continue
		}
		if c.Work != nil && c.Work.CheckAlloc(int64(len(text))+64) != nil {
			return nil
		}
		allowed[text] = true
	}
	return allowed
}
