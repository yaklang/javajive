package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Ordinary fixed-arity references do not allocate LambdaToMethod ordinals.
// Their original bootstrap edge and actual canonical source occurrence still
// need independent ownership; an unaccounted :: must never disappear merely
// because another reference in this class requires a compiler adapter.
type ordinaryReferenceOccurrence struct {
	receipt *interfaceInitializerMethodReference
	context *class_context.ClassContext
	packet  string
}

func (c *ClassObjectDumper) originalOrdinaryReferenceSite(f *lambdaSourceFactory) (values.MethodReferenceSource, bool) {
	var id values.MethodReferenceSource
	if c == nil || c.obj == nil || c.FuncCtx == nil || c.FuncCtx.InvocationMetadata == nil || !f.closed() || !f.sealed || f.object != c.obj || isSyntheticMethod(f.method.AccessFlags) || f.pc+5 > len(f.code.Code) {
		return id, false
	}
	if constructorLoweringCode(f.method) != f.code || len(c.obj.Methods) > 4096 {
		return id, false
	}
	owned := 0
	for _, method := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return id, false
		}
		if method == f.method {
			owned++
		}
	}
	if owned != 1 {
		return id, false
	}
	op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_INVOKEDYNAMIC}, CurrentOffset: uint16(f.pc), Data: f.code.Code[f.pc+1 : f.pc+5]}
	remaining := 4096
	desc, known := c.constructorReceiverDynamicDescriptor(c.obj, f.code, op, &remaining)
	params, _, err := callbinding.Descriptor(desc)
	if !known || err != nil || len(params) != 0 {
		return id, false
	}
	cp := func(i uint16) ConstantInfo {
		if i == 0 || int(i) > len(c.obj.ConstantPool) {
			return nil
		}
		return c.obj.ConstantPool[i-1]
	}
	site, ok := cp(uint16(core.Convert2bytesToInt(op.Data[:2]))).(*ConstantInvokeDynamicInfo)
	if !ok || site == nil {
		return id, false
	}
	nt, ok := cp(site.NameAndTypeIndex).(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return id, false
	}
	var table *BootstrapMethodsAttribute
	for _, a := range c.obj.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return id, false
		}
		if b, ok := a.(*BootstrapMethodsAttribute); ok {
			if b == nil || table != nil {
				return id, false
			}
			table = b
		}
	}
	if table == nil || int(table.NumBootstrapMethods) != len(table.BootstrapMethods) || int(site.BootstrapMethodAttrIndex) >= len(table.BootstrapMethods) {
		return id, false
	}
	b := table.BootstrapMethods[site.BootstrapMethodAttrIndex]
	if b == nil || b.NumBootstrapArguments != 3 || len(b.BootstrapArguments) != 3 {
		return id, false
	}
	boot, ok := cp(b.BootstrapMethodRef).(*ConstantMethodHandleInfo)
	bt, valid := nativeLambdaOriginalHandleTarget(c.obj, boot)
	if !ok || !valid || boot.ReferenceKind != 6 || bt.interfaceRef || bt.owner != "java/lang/invoke/LambdaMetafactory" || bt.name != "metafactory" || bt.descriptor != "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;" {
		return id, false
	}
	impl, ok := cp(b.BootstrapArguments[1]).(*ConstantMethodHandleInfo)
	target, valid := nativeLambdaOriginalHandleTarget(c.obj, impl)
	if !ok || !valid || impl.ReferenceKind != 5 && impl.ReferenceKind != 6 && impl.ReferenceKind != 9 {
		return id, false
	}
	id.Kind, id.Owner, id.Member, id.Descriptor, id.CallDescriptor = impl.ReferenceKind, target.owner, target.name, target.descriptor, desc
	id.CallName, known = sourceBridgeUTF8(c.obj, nt.NameIndex)
	if !known {
		return id, false
	}
	for index, output := range map[int]*string{0: &id.ErasedSAM, 2: &id.Instantiated} {
		mt, ok := cp(b.BootstrapArguments[index]).(*ConstantMethodTypeInfo)
		if !ok || mt == nil {
			return id, false
		}
		*output, known = sourceBridgeUTF8(c.obj, mt.DescriptorIndex)
		if !known {
			return id, false
		}
	}
	owner, known := c.FuncCtx.InvocationMetadata(id.Owner)
	if !known || owner.Name != id.Owner || !owner.MembersComplete || len(owner.Methods) > 4096 {
		return id, false
	}
	matched := 0
	for _, method := range owner.Methods {
		if !nativeProofWork(c.Work, 1) {
			return id, false
		}
		if method.Name == id.Member && method.Desc == id.Descriptor {
			if !method.Public || method.Varargs || method.Bridge || method.Static != (id.Kind == 6) {
				return id, false
			}
			matched++
		}
	}
	return id, matched == 1
}

func (c *ClassObjectDumper) recordOrdinaryReferenceEmission(parent *dumpedMethods, code *CodeAttribute, ctx *class_context.ClassContext, value any, packet string) {
	frame := c.lambdaMethodRendering
	if frame == nil || frame.body != parent {
		return
	}
	v, ok := value.(*values.CustomValue)
	pc, captures, known := v.OriginalLambdaFactoryEvaluation()
	if !ok || !known || len(captures) != 0 {
		frame.invalid = true
		return
	}
	r := c.interfaceInitializerOriginalMethodReference(v, code, pc)
	if r == nil || r.factory.method != parent.member {
		frame.invalid = true
		return
	}
	// Other ordinary reference domains keep their old source path. They cannot
	// be omitted from a mixed compiler-registration proof below.
	id, direct := c.originalOrdinaryReferenceSite(r.factory)
	if !direct || id != r.identity {
		return
	}
	if frame.ended || frame.invalid || packet == "" || len(packet) > 1<<20 || len(frame.references) >= methodSourceLambdaRequestLimit || int64(len(packet)) > (4<<20)-frame.bytes || !nativeProofWork(c.Work, int64(len(packet))+1) || c.Work != nil && c.Work.CheckAlloc(frame.bytes+int64(len(packet))+int64(len(frame.references)+1)*128) != nil {
		frame.invalid = true
		return
	}
	for i, prior := range frame.references {
		if prior.receipt.factory.pc == pc {
			if i == len(frame.references)-1 && prior.receipt.value == v && prior.packet == packet {
				return
			}
			frame.invalid = true
			return
		}
	}
	snapshot, known := ctx.SnapshotForSourceProof(c.Work)
	if !known {
		frame.invalid = true
		return
	}
	frame.references = append(frame.references, ordinaryReferenceOccurrence{r, snapshot, strings.Clone(packet)})
	frame.bytes += int64(len(packet))
}

func (c *ClassObjectDumper) ordinaryReferencePositions(source string, parent *dumpedMethods, charge func(int64) bool) (map[int]bool, bool) {
	positions := map[int]bool{}
	frame := parent.lambdaEmission
	if frame == nil {
		return positions, true
	}
	if !frame.closed() || frame.body != parent || frame.request != parent.sourceRequest || len(frame.references) > methodSourceLambdaRequestLimit {
		return nil, false
	}
	tokens, known := c.lambdaOwnImplementationTokens(source, true)
	if !known {
		return nil, false
	}
	previous := -1
	for _, event := range frame.references {
		r := event.receipt
		if r == nil || !r.closed(c) || r.factory.method != parent.member || r.factory.code != constructorLoweringCode(parent.member) || event.context == nil || !charge(int64(len(event.packet))+int64(len(source))) {
			return nil, false
		}
		name, nk := sourceBridgeUTF8(c.obj, parent.member.NameIndex)
		desc, dk := sourceBridgeUTF8(c.obj, parent.member.DescriptorIndex)
		id, direct := c.originalOrdinaryReferenceSite(r.factory)
		if !nk || !dk || strings.ReplaceAll(event.context.ClassName, ".", "/") != c.obj.GetClassName() || event.context.IsStatic != (parent.member.AccessFlags&StaticFlag != 0) || event.context.FunctionName != name || event.context.CurrentMethodDesc != desc || !direct || id != r.identity || r.value.String(event.context) != event.packet {
			return nil, false
		}
		local, known := nativeLambdaSourceImplementationTokens(event.packet, c.Work, true)
		if !known || len(local) != 1 || event.packet[local[0]:local[0]+2] != "::" {
			return nil, false
		}
		found := -1
		for _, at := range tokens {
			if !nativeProofWork(c.Work, 1) {
				return nil, false
			}
			if at <= previous || source[at:at+2] != "::" {
				continue
			}
			start := at - local[0]
			if start >= 0 && len(event.packet) <= len(source)-start && source[start:start+len(event.packet)] == event.packet {
				found = at
				break
			}
		}
		if found < 0 {
			return nil, false
		}
		positions[found], previous = true, found
	}
	return positions, true
}

func (c *ClassObjectDumper) lambdaWithoutOrdinaryReferences(source string, parent *dumpedMethods, tokens []int, charge func(int64) bool) ([]int, bool) {
	positions, known := c.ordinaryReferencePositions(source, parent, charge)
	if !known {
		return nil, false
	}
	filtered := make([]int, 0, len(tokens))
	for _, at := range tokens {
		if !positions[at] {
			filtered = append(filtered, at)
		}
	}
	return filtered, true
}

func (c *ClassObjectDumper) lambdaClassWithoutOrdinaryReferences(source string, methods []*dumpedMethods, tokens []int, charge func(int64) bool) ([]int, bool) {
	positions := map[int]bool{}
	for _, parent := range methods {
		if parent == nil || parent.lambdaEmission == nil || len(parent.lambdaEmission.references) == 0 {
			continue
		}
		if parent.sourceDumper != c || parent.sourceObject != c.obj || parent.sourceRequest.role != methodSourceDeclaration || c.dumpedMethodsSet[parent.sourceCacheKey] != parent || parent.sourceReconstructionFailed() {
			return nil, false
		}
		declaration, offset, known := c.lambdaParentDeclaration(source, parent)
		if !known {
			return nil, false
		}
		local, known := c.ordinaryReferencePositions(declaration, parent, charge)
		if !known {
			return nil, false
		}
		for at := range local {
			if positions[offset+at] {
				return nil, false
			}
			positions[offset+at] = true
		}
	}
	filtered := make([]int, 0, len(tokens))
	for _, at := range tokens {
		if !positions[at] {
			filtered = append(filtered, at)
		}
	}
	return filtered, true
}
