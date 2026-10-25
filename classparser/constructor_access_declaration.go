package javaclassparser

import "crypto/sha256"

// An explicit source declaration of a compiler access constructor must retain
// its erased physical ports. The private target's generic signature is not the
// bridge's signature. This receipt only protects the actual writer's output;
// it grants no metadata edit, constructor folding, or invocation retargeting.
type constructorAccessDeclaration struct {
	packet     nativeConstructorAccessBridge
	code       *CodeAttribute
	codeHash   [32]byte
	stack      uint16
	locals     uint16
	exceptions string
	present    bool
	published  string
}

func (c *ClassObjectDumper) sealConstructorAccessDeclaration(m *dumpedMethods) *constructorAccessDeclaration {
	if c.options.sourceConstructorLowering == nil || !c.reconstructionPhysicalHeader() || m == nil || m.member == nil || m.methodName != "<init>" || m.sourceRequest.role != methodSourceDeclaration || m.sourceReconstructionFailed() {
		return nil
	}
	// DumpMethods records the declaration inventory after this concrete writer
	// returns. Read its original descriptor here, not that later bookkeeping.
	descriptor, known := sourceBridgeUTF8(c.obj, m.member.DescriptorIndex)
	if !known {
		return nil
	}
	packet := c.originalNativeConstructorAccessBridges()[descriptor]
	if packet == nil || packet.method != m.member {
		return nil
	}
	code := constructorLoweringCode(m.member)
	exceptions, present, known := sourceAnonymousExceptionNames(c.obj, m.member, c.Work)
	if code == nil || !known || !nativeProofWork(c.Work, int64(len(code.Code))+1) {
		return nil
	}
	return &constructorAccessDeclaration{packet: *packet, code: code, codeHash: sha256.Sum256(code.Code), stack: code.MaxStack, locals: code.MaxLocals, exceptions: exceptions, present: present, published: m.code}
}

func (p *constructorAccessDeclaration) fresh(c *ClassObjectDumper, m *dumpedMethods) bool {
	if p == nil || c.options.sourceConstructorLowering == nil || !c.reconstructionPhysicalHeader() || m == nil || m.member != p.packet.method || m.methodName != "<init>" || m.descriptor != p.packet.descriptor || m.code != p.published {
		return false
	}
	packet := c.originalNativeConstructorAccessBridges()[m.descriptor]
	code := constructorLoweringCode(m.member)
	if packet == nil || *packet != p.packet || code == nil || code != p.code || code.MaxStack != p.stack || code.MaxLocals != p.locals || !nativeProofWork(c.Work, int64(len(code.Code))+1) || sha256.Sum256(code.Code) != p.codeHash {
		return false
	}
	exceptions, present, known := sourceAnonymousExceptionNames(c.obj, m.member, c.Work)
	return known && present == p.present && exceptions == p.exceptions
}
