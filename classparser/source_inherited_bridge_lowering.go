package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type inheritedBridgePacket struct {
	name, descriptor, targetOwner, targetDescriptor string
	visibility                                      uint16
	throws                                          []string
	sourceDescriptor                                string
	sourceName                                      string
	sourceParameters                                []types.JavaType
	implicitVisibilityBridge                        bool
	argumentCasts                                   []string
	checkedArguments                                bool
	invokePC                                        int
	invokeIndex                                     uint16
}

// Only a pure typed forwarding program can be regenerated through a temporary
// narrow source override. Receiver/argument identity, category-two local slots,
// invocation kind and exception declarations are part of the proof.
func inheritedBridgeForwardPacket(object *ClassObject, method *MemberInfo, opcode int, work *workbudget.Budget, referenceAdaptation ...bool) *inheritedBridgePacket {
	return inheritedBridgeNamedForwardPacket(object, method, opcode, work, "", referenceAdaptation...)
}

// The named variant proves a compiler-only declaration whose body still calls
// the independently selected original super target. It grants no name-based
// matching: the caller must own the publication and the exact fresh member.
func inheritedBridgeNamedForwardPacket(object *ClassObject, method *MemberInfo, opcode int, work *workbudget.Budget, targetName string, referenceAdaptation ...bool) *inheritedBridgePacket {
	if len(referenceAdaptation) > 1 || object == nil || method == nil || object.MajorVersion < 49 || object.MajorVersion > 65 || object.MinorVersion != 0 || len(object.Methods) > 4096 {
		return nil
	}
	name, nk := sourceBridgeUTF8(object, method.NameIndex)
	desc, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
	params, result, err := callbinding.Descriptor(desc)
	if !nk || !dk || !class_context.PreservesJava8Identifier(name) || err != nil {
		return nil
	}
	if _, valid := inheritedBridgeAnnotationTables(method, desc, work); !valid {
		return nil
	}
	code := constructorLoweringCode(method)
	if code == nil || len(code.Code) > 512 || len(code.ExceptionTable) != 0 || !nativeProofWork(work, int64(len(code.Code))+1) {
		return nil
	}
	adapt := len(referenceAdaptation) == 1 && referenceAdaptation[0]
	p := &inheritedBridgePacket{name: name, descriptor: desc}
	exceptionCount, codeCount := 0, 0
	// Modern javac emits MethodParameters on synthetic bridges even without
	// -parameters. Validate its descriptor-position table independently of Code;
	// the existing parameter restorer later preserves the exact original table.
	textWork := int64(0)
	if _, _, err := sourceParameterRows(object, method, work, &textWork); err != nil {
		return nil
	}
	for _, a := range method.Attributes {
		switch a := a.(type) {
		case *CodeAttribute:
			codeCount++
			if a == nil || codeCount != 1 {
				return nil
			}
		case *UnparsedAttribute:
			if a == nil || a.Name != "MethodParameters" {
				return nil
			}
		case *SignatureAttribute:
			// A temporary generic source declaration may carry a Signature.
			// Bridges themselves remain signature-free; prove this helper's
			// complete erasure before inspecting its executable forwarding.
			if !adapt || method.AccessFlags != 1 && method.AccessFlags != 4 || a == nil {
				return nil
			}
			sig, known := nativeMethodLocalOriginalSignature(object, method.Attributes, work)
			classSig, classKnown := nativeOriginalClassSignature(object, work)
			size := len(classSig) + len(sig)
			if !known || !classKnown || size > 16384 || !nativeProofWork(work, int64(size)*260+1) || work.CheckAlloc(int64(size)*256) != nil {
				return nil
			}
			erased, throws, valid := types.EraseClassBoundMethodSignatureWithThrows(classSig, sig)
			if !known || !classKnown || sig == "" || !valid || erased != desc || !nativeOriginalSignatureThrows(object, method, throws, work) {
				return nil
			}
		case *ExceptionsAttribute:
			exceptionCount++
			if a == nil || exceptionCount > 1 {
				return nil
			}
			for _, index := range a.ExceptionIndexTable {
				n, ok := sourceBridgeClassName(object, index)
				if !ok || !nativeSourceBinaryName(n) {
					return nil
				}
				p.throws = append(p.throws, n)
			}
		case *RuntimeVisibleAnnotationsAttribute, *RuntimeVisibleParameterAnnotationsAttribute:
			// Validated separately above; fresh completion must retain the
			// exact value graphs at the same declaration/parameter positions.
		default:
			return nil
		}
	}
	d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
	d.Work = work
	if d.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(d)
	if len(ops) < len(params)+3 || len(ops) > 2*len(params)+3 || !enumArrayInstructionSpans(code.Code, ops) || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") {
		return nil
	}
	call := constructorMotionMember(object, ops[len(ops)-2], opcode)
	if targetName == "" {
		targetName = name
	}
	if call == nil || call.Member != targetName || !nativeSourceBinaryName(call.Name) {
		return nil
	}
	args, ret, err := callbinding.Descriptor(call.Description)
	if err != nil || len(args) != len(params) {
		return nil
	}
	p.argumentCasts = make([]string, len(params))
	slot, at := 1, 1
	for i, param := range params {
		if at >= len(ops)-2 || core.GetRetrieveIdx(ops[at]) != slot || !constructorMotionLoad(ops[at], param) {
			return nil
		}
		at++
		if at < len(ops)-2 && ops[at].Instr.OpCode == core.OP_CHECKCAST {
			index, known := nativeEnumCPIndex(ops[at])
			name, valid := sourceBridgeClassName(object, index)
			cast := name
			if !strings.HasPrefix(cast, "[") {
				cast = "L" + cast + ";"
			}
			_, _, syntax := callbinding.Descriptor("(" + cast + ")V")
			if !known || !valid || syntax != nil || !callbinding.Reference(param) || cast != args[i] && !(adapt && callbinding.Reference(args[i])) {
				return nil
			}
			p.argumentCasts[i], p.checkedArguments = cast, true
			at++
		} else if param != args[i] && !(adapt && callbinding.Reference(param) && callbinding.Reference(args[i])) {
			return nil
		}
		slot += constructorEffectType(param).width()
	}
	if at != len(ops)-2 {
		return nil
	}
	stack := slot
	if result == "J" || result == "D" {
		stack = max(stack, 2)
	}
	if call == nil || call.Member != targetName || !nativeSourceBinaryName(call.Name) || !nativeEnumOpcode(ops[len(ops)-1], int(bridgeReturnOpcode(desc))) || int(code.MaxStack) < stack || int(code.MaxLocals) < slot {
		return nil
	}
	reference := func(s string) bool { return strings.HasPrefix(s, "L") || strings.HasPrefix(s, "[") }
	if ret != result && !(reference(ret) && reference(result)) {
		return nil
	}
	p.invokeIndex, _ = nativeEnumCPIndex(ops[at])
	p.invokePC = int(ops[at].CurrentOffset)
	p.targetOwner, p.targetDescriptor = call.Name, call.Description
	return p
}

func (c *ClassObjectDumper) inheritedBridgeOriginal(method *MemberInfo) *inheritedBridgePacket {
	// The admitted program uses only the class-super invocation protocol:
	// covariance and synthetic bridges already exist in the Java 5 format.
	// Its receiver, argument slots, direct superclass and exact target are
	// proved below; none of those obligations depends on Java 8 metadata.
	if c.obj.MajorVersion < 49 || c.obj.MajorVersion > 52 || method.AccessFlags != 0x1041 && method.AccessFlags != 0x1044 {
		return nil
	}
	p := inheritedBridgeForwardPacket(c.obj, method, core.OP_INVOKESPECIAL, c.Work, true)
	if p == nil || p.targetOwner != c.obj.GetSupperClassName() {
		return nil
	}
	p.visibility = method.AccessFlags & 7
	// A real declaration already supplies its own bridge; a second narrow
	// override would change its dispatch or collide with an existing overload.
	originalMatches := 0
	for _, m := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		n, _ := sourceBridgeUTF8(c.obj, m.NameIndex)
		d, _ := sourceBridgeUTF8(c.obj, m.DescriptorIndex)
		if n == p.name && d == p.targetDescriptor && m != method {
			return nil
		}
		if n == p.name && d == p.descriptor {
			originalMatches++
		}
	}
	if originalMatches != 1 {
		return nil
	}
	// External original dependencies establish method declarations without
	// granting source ownership of those classes to this independent unit.
	resolve := c.nativeAnnotationDeclarationResolver()
	parent, ok := resolve(p.targetOwner)
	if !ok || parent == nil || parent.GetClassName() != p.targetOwner {
		return nil
	}
	// The symbolic owner is the direct superclass, but JVM class-method
	// resolution may find its declaration further up the same original chain.
	// Keep that symbolic owner for the fresh invokespecial proof; do not replace
	// it with a guessed declaration owner or skip a nearer incompatible method.
	declaring, target := nativeMemberCallTarget(parent, p.name, p.targetDescriptor, true, resolve, c.Work)
	if target == nil {
		return nil
	}
	visibility := target.AccessFlags & ^uint16(0x80)
	// A source override must preserve the original bridge access and may
	// never narrow its inherited target. Both public and protected calls use
	// this exact implicit receiver; no arbitrary protected receiver is granted.
	if visibility != 1 && visibility != 4 || visibility == 1 && p.visibility != 1 {
		return nil
	}
	// ACC_VARARGS affects source applicability, not the physical array port
	// of this already resolved super invocation. Emit a fixed-arity override;
	// fresh completion still proves the identical target descriptor and slots.
	// A varargs marker without an array tail is not a valid declaration proof.
	if target.AccessFlags&0x80 != 0 {
		args, _, err := callbinding.Descriptor(p.targetDescriptor)
		if err != nil || len(args) == 0 || !strings.HasPrefix(args[len(args)-1], "[") {
			return nil
		}
	}
	// Concrete parameterization cannot change this already resolved physical
	// invocation. Prove its complete erasure using the actual declaration's
	// constant pool, including throws. Class variables are permitted only
	// beneath erased arguments. Method-owned variables need the independent
	// lexical certificate below; direct variables need actual substitution.
	signature, known := nativeMethodLocalOriginalSignature(declaring, target.Attributes, c.Work)
	if !known {
		return nil
	}
	p.sourceDescriptor = p.targetDescriptor
	plain := signature == "" || nativeConcreteMethodSignatureClosed(declaring, signature, p.targetDescriptor, target, c.Work) || nativeClassInvariantMethodSignatureClosed(declaring, signature, p.targetDescriptor, target, c.Work) || inheritedBridgeMethodFormalPortsClosed(declaring, signature, p.targetDescriptor, target, c.Work)
	if plain {
		// Preserve the old exact-port grammar for an uninstantiated target.
		if inheritedBridgeForwardPacket(c.obj, method, core.OP_INVOKESPECIAL, c.Work) == nil {
			return nil
		}
	} else if !c.inheritedBridgeGenericView(p, declaring, target, signature) {
		return nil
	}
	// A public subclass exposes a public implementation inherited from its
	// non-public direct parent through a compiler-provided visibility bridge.
	// An explicit same-descriptor override changes the source declaration set
	// and can trigger another erased-interface bridge that the original child
	// never declared. Keep inheritance as the source representation; the final
	// artifact still has to prove the original method universe and bridge Code.
	p.implicitVisibilityBridge = p.visibility == 1 && visibility == 1 && p.descriptor == p.targetDescriptor && c.obj.AccessFlags&1 != 0 && parent.AccessFlags&1 == 0
	return p
}

// A non-generic override of a generic method's erasure is a subsignature.
// Here it exists only to make javac regenerate an inherited bridge. Permit it
// when method inference cannot change any invocation port, including throws.
// The original typed forwarding proof and final fresh-Code/unique-consumer
// proof remain mandatory; erasure alone never licenses an executable rewrite.
func inheritedBridgeMethodFormalPortsClosed(owner *ClassObject, signature, descriptor string, target *MemberInfo, work *workbudget.Budget) bool {
	if owner == nil || target == nil || len(signature) > 4096 || !nativeProofWork(work, int64(len(signature))*130+1) || len(types.MethodFormalTypeParamNames(signature)) == 0 {
		return false
	}
	classSignature, known := nativeMethodLocalOriginalSignature(owner, owner.Attributes, work)
	size := len(signature) + len(classSignature)
	if !known || len(classSignature) > 4096 || !nativeProofWork(work, int64(size)*260+1) || work != nil && work.CheckAlloc(int64(size)*256) != nil {
		return false
	}
	erased, throws, known := types.EraseLexicalInvariantMethodSignatureWithThrows(classSignature, signature)
	return known && erased == descriptor && (classSignature == "" || nativeOriginalClassSignatureSupersClosed(owner, classSignature, work)) && nativeOriginalSignatureThrows(owner, target, throws, work)
}

func (c *ClassObjectDumper) inheritedBridgeSource(method *MemberInfo) *dumpedMethods {
	if !c.reconstructionPhysicalHeader() {
		return nil
	}
	p := c.inheritedBridgeOriginal(method)
	if p == nil {
		p = c.finalInheritedBridgeOriginal(method)
	}
	if p == nil || p.implicitVisibilityBridge || !c.inheritedBridgeAnnotationsRepresentable(method, p.descriptor) {
		return nil
	}
	// Several erased return descriptors may share one narrow source override.
	// A single declaration can regenerate their annotations only when those
	// original metadata contracts agree; otherwise keep the existing refusal.
	for _, other := range c.obj.Methods {
		if other == method || other == nil || other.AccessFlags != 0x1041 && other.AccessFlags != 0x1044 {
			continue
		}
		q := c.inheritedBridgeOriginal(other)
		if q != nil && q.name == p.name && q.sourceDescriptor == p.sourceDescriptor && (q.visibility != p.visibility || !inheritedBridgeAnnotationsEqual(method, p.descriptor, other, q.descriptor, c.Work)) {
			return nil
		}
	}
	typ, err := types.ParseMethodDescriptor(p.sourceDescriptor)
	if err != nil {
		return nil
	}
	ft := typ.FunctionType()
	if p.sourceParameters != nil {
		ft.ParamTypes = p.sourceParameters
	}
	params, args := []string{}, []string{}
	for i, t := range ft.ParamTypes {
		name := fmt.Sprintf("bridgeArgument%d", i)
		params = append(params, t.String(c.FuncCtx)+" "+name)
		args = append(args, name)
	}
	params = c.applyParameterAnnotations(method, p.name, p.descriptor, params)
	var annotations []string
	for _, attribute := range method.Attributes {
		if table, ok := attribute.(*RuntimeVisibleAnnotationsAttribute); ok {
			for _, annotation := range table.Annotations {
				text, err := c.DumpAnnotation(annotation)
				if err != nil {
					return nil
				}
				annotations = append(annotations, text)
			}
		}
	}
	throws := []string{}
	for _, n := range p.throws {
		t, err := types.ParseDescriptor("L" + n + ";")
		if err != nil {
			return nil
		}
		throws = append(throws, t.String(c.FuncCtx))
	}
	tail := ""
	if len(throws) != 0 {
		tail = " throws " + strings.Join(throws, ", ")
	}
	call := "super." + p.name + "(" + strings.Join(args, ", ") + ");"
	_, result, _ := callbinding.Descriptor(p.targetDescriptor)
	if result != "V" {
		call = "return " + call
	}
	access := "public "
	if p.visibility == 4 {
		access = "protected "
	}
	sourceName := p.name
	if p.sourceName != "" {
		sourceName = p.sourceName
		access = "private "
	}
	source := access + ft.ReturnType.String(c.FuncCtx) + " " + sourceName + "(" + strings.Join(params, ", ") + ")" + tail + " {\n\t" + call + "\n}"
	if len(annotations) > 0 {
		source = strings.Join(annotations, "\n") + "\n" + source
	}
	return &dumpedMethods{methodName: sourceName, descriptor: p.sourceDescriptor, code: source, bodyCode: source, inheritedBridge: p, inheritedBridgeCode: source}
}

func (c *ClassObjectDumper) publishInheritedBridgeLowering(source string, methods []*dumpedMethods) bool {
	a := c.options.sourceConstructorLowering
	if a == nil {
		return true
	}
	found := c.reconstructionPhysicalHeader() && (legacyAbstractInterfaceDomain(c.obj, c.Work) || c.inheritedVisibilityDomainParent() != nil)
	for _, m := range methods {
		if m == nil || m.inheritedBridge == nil {
			continue
		}
		found = true
		if m.isStub || m.code != m.inheritedBridgeCode || strings.Count(source, m.code) != 1 {
			return false
		}
	}
	if !found {
		return true
	}
	origin := constructorNativeSourceOrigin{}
	if !c.sourceDegraded && !legacySourceHasFailureMarker(source) {
		raw, err := a.jar.readJarEntry(c.obj.GetClassName() + ".class")
		if err != nil || !nativeProofWork(c.Work, int64(len(raw)+len(source))+1) || !bytes.Equal(raw, c.obj.Bytes()) {
			return false
		}
		origin = constructorNativeSourceOrigin{sha256.Sum256(raw), sha256.Sum256([]byte(source))}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bridgePending == nil {
		a.bridgePending = map[string]constructorNativeSourceOrigin{}
	}
	a.bridgePending[c.obj.GetClassName()] = origin
	return true
}

func (a *sourceConstructorLoweringAuthority) bridgeSourceReturned(owner string, original []byte, work *workbudget.Budget) bool {
	if a == nil || !nativeProofWork(work, 1) {
		return false
	}
	a.mu.Lock()
	origin, active := a.bridgeActive[owner]
	_, pending := a.bridgePending[owner]
	// A completed native source can itself be the physical independent root.
	// Only its exact public return grants this receipt, never a child comment.
	if !active && !pending {
		origin, active = a.nativeActive[owner]
	}
	a.mu.Unlock()
	if !active && !pending && a.parent != nil && a.jar != nil && a.parent.jar != nil && a.jar.sourceCompiler.effective() == a.parent.jar.sourceCompiler.effective() && a.jar.targetSourceVersion == a.parent.jar.targetSourceVersion {
		a.parent.mu.Lock()
		origin, active = a.parent.bridgeActive[owner]
		a.parent.mu.Unlock()
	}
	return active && origin.sourceHash != ([32]byte{}) && origin.originalHash == sha256.Sum256(original)
}

// The compiler generates a real wide bridge through the temporary narrow
// override. Inline that fresh override only after proving the complete original
// super-binding and its unique fresh consumer. No original Code is imported.
func (a *sourceConstructorLoweringAuthority) lowerInheritedBridges(owner string, original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	if !a.bridgeSourceReturned(owner, original, work) {
		return decline()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return nil, sourceLambdaMetadataFailure(work, "inherited bridge input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	reader, err := NewClassReaderWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
	got, err := parseWithMetadataLayout(reader, nil, layout)
	if err != nil {
		return nil, err
	}
	if !sourceParameterClassIdentityClosed(want, got) || len(want.Methods) > len(got.Methods) || len(got.Methods) > 4096 {
		return decline()
	}
	if work.CheckAlloc(int64(len(got.Methods))*512) != nil {
		return nil, work.Err()
	}
	d := NewClassObjectDumper(want)
	d.Work = work
	d.foldSiblingResolver = a.jar.enumSiblingResolver()
	d.declarationResolver = a.jar.declarationResolver
	key := func(o *ClassObject, m *MemberInfo) string {
		n, _ := sourceBridgeUTF8(o, m.NameIndex)
		s, _ := sourceBridgeUTF8(o, m.DescriptorIndex)
		return n + "\x00" + s
	}
	before, after := map[string]*MemberInfo{}, map[string]*MemberInfo{}
	for _, pair := range []struct {
		o     *ClassObject
		index map[string]*MemberInfo
	}{{want, before}, {got, after}} {
		for _, m := range pair.o.Methods {
			k := key(pair.o, m)
			if pair.index[k] != nil || !nativeProofWork(work, 1) {
				return decline()
			}
			pair.index[k] = m
		}
	}
	for k := range before {
		if after[k] == nil {
			return decline()
		}
	}
	var edits []compiledMetadataEdit
	visibility := map[*MemberInfo]bool{}
	if parent := d.inheritedVisibilityDomainParent(); parent != nil {
		for k, method := range after {
			if before[k] != nil || !d.extraInheritedVisibilityBridge(got, method, parent) {
				continue
			}
			span, known := layout.methods[method]
			if !known || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 || binary.BigEndian.Uint16(compiled[span.start:]) != method.AccessFlags {
				return decline()
			}
			visibility[method] = true
			edits = append(edits, compiledMetadataEdit{span, nil})
		}
	}
	exact := 0
	consumers := map[*MemberInfo]map[*MemberInfo]bool{}
	packets := map[*MemberInfo]*inheritedBridgePacket{}
	redundant := map[*MemberInfo]*inheritedBridgePacket{}
	for k, m := range before {
		p := d.inheritedBridgeOriginal(m)
		if p == nil {
			continue
		}
		bridge := after[k]
		if !inheritedBridgeAnnotationsEqual(m, p.descriptor, bridge, p.descriptor, work) {
			return decline()
		}
		if p.descriptor == p.targetDescriptor {
			// A visibility bridge need not change the descriptor. Its explicit
			// source override already is the entire forwarding program: retain
			// every fresh Code byte and restore only the unspellable flags. Unlike
			// the covariant case below, no temporary method is removed, so other
			// callers and method handles continue naming this same declaration.
			fresh := inheritedBridgeForwardPacket(got, bridge, core.OP_INVOKESPECIAL, work)
			if bridge.AccessFlags != p.visibility && bridge.AccessFlags != m.AccessFlags || fresh == nil || fresh.targetOwner != p.targetOwner || fresh.targetDescriptor != p.targetDescriptor || !slices.Equal(p.throws, fresh.throws) || !slices.Equal(p.argumentCasts, fresh.argumentCasts) {
				return decline()
			}
			span, known := layout.methods[bridge]
			if !known || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 || binary.BigEndian.Uint16(compiled[span.start:]) != bridge.AccessFlags {
				return decline()
			}
			if bridge.AccessFlags != m.AccessFlags {
				flags := []byte{byte(m.AccessFlags >> 8), byte(m.AccessFlags)}
				edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{span.start, span.start + 2}, flags})
			}
			exact++
			continue
		}
		helper := after[p.name+"\x00"+p.sourceDescriptor]
		if helper == nil || before[key(got, helper)] != nil || helper.AccessFlags != p.visibility || bridge.AccessFlags != m.AccessFlags || !inheritedBridgeAnnotationsEqual(m, p.descriptor, helper, p.sourceDescriptor, work) {
			return decline()
		}
		fresh := inheritedBridgeForwardPacket(got, helper, core.OP_INVOKESPECIAL, work, p.sourceParameters != nil)
		forward := inheritedBridgeForwardPacket(got, bridge, core.OP_INVOKEVIRTUAL, work)
		if fresh == nil || forward == nil || fresh.checkedArguments || fresh.targetOwner != p.targetOwner || fresh.targetDescriptor != p.targetDescriptor || forward.targetOwner != owner || forward.targetDescriptor != p.sourceDescriptor || !slices.Equal(p.throws, fresh.throws) || !slices.Equal(p.throws, forward.throws) || !slices.Equal(p.argumentCasts, forward.argumentCasts) {
			return decline()
		}
		bridgeCode, err := enumArrayCodePayload(compiled, got, layout.methods[bridge], work)
		if err != nil {
			return nil, err
		}
		// Retain the compiler's bridge loads, CHECKCASTs, return and every PC.
		// Inline only the helper's proven super invocation, using its fresh CP
		// entry. Copying the narrow helper body would lose the bridge's checks
		// and change both verifier types and ClassCastException timing.
		data := bytes.Clone(compiled[bridgeCode.span.start:bridgeCode.span.end])
		code := constructorLoweringCode(bridge)
		if code == nil || len(data) < 14+len(code.Code) || int(binary.BigEndian.Uint32(data[10:])) != len(code.Code) || !bytes.Equal(data[14:14+len(code.Code)], code.Code) || forward.invokePC < 0 || forward.invokePC+3 > len(code.Code) || fresh.invokeIndex == 0 {
			return decline()
		}
		pos := 14 + forward.invokePC
		if data[pos] != core.OP_INVOKEVIRTUAL || binary.BigEndian.Uint16(data[pos+1:]) != forward.invokeIndex {
			return decline()
		}
		data[pos] = core.OP_INVOKESPECIAL
		binary.BigEndian.PutUint16(data[pos+1:], fresh.invokeIndex)
		edits = append(edits, compiledMetadataEdit{bridgeCode.span, data})
		if consumers[helper] == nil {
			consumers[helper] = map[*MemberInfo]bool{}
			packets[helper] = p
			edits = append(edits, compiledMetadataEdit{layout.methods[helper], nil})
		}
		consumers[helper][bridge] = true
		// A narrower generic override also causes javac to bridge the erased
		// parent declaration. The original inherited that method directly.
		// Remove only this proven temporary forwarding node, after proving
		// that nobody calls it and including its helper edge in closure.
		if p.sourceParameters != nil && p.sourceDescriptor != p.targetDescriptor {
			extra := after[p.name+"\x00"+p.targetDescriptor]
			if extra != nil {
				if before[key(got, extra)] != nil || visibility[extra] || extra.AccessFlags != 0x1040|p.visibility {
					return decline()
				}
				ep := inheritedBridgeForwardPacket(got, extra, core.OP_INVOKEVIRTUAL, work)
				if ep == nil || ep.targetOwner != owner || ep.targetDescriptor != p.sourceDescriptor || !slices.Equal(p.throws, ep.throws) || !inheritedBridgeAnnotationsEqual(helper, p.sourceDescriptor, extra, p.targetDescriptor, work) {
					return decline()
				}
				args, _, _ := callbinding.Descriptor(p.targetDescriptor)
				narrow, _, _ := callbinding.Descriptor(p.sourceDescriptor)
				for i, arg := range args {
					cast := ""
					if arg != narrow[i] {
						cast = narrow[i]
					}
					if ep.argumentCasts[i] != cast {
						return decline()
					}
				}
				if redundant[extra] == nil {
					redundant[extra] = &inheritedBridgePacket{name: p.name, sourceDescriptor: p.targetDescriptor}
					edits = append(edits, compiledMetadataEdit{layout.methods[extra], nil})
				}
				consumers[helper][extra] = true
			}
		}
	}
	if len(consumers) == 0 && exact == 0 && len(visibility) == 0 || len(after)-len(consumers)-len(visibility)-len(redundant) != len(before) {
		return decline()
	}
	for _, packet := range redundant {
		if !inheritedBridgeConsumersClosed(got, nil, packet, work) {
			return decline()
		}
	}
	for helper, bridges := range consumers {
		if !inheritedBridgeConsumersClosed(got, bridges, packets[helper], work) {
			return decline()
		}
	}
	if len(consumers) == 0 && len(visibility) == 0 {
		return applyCompiledMetadataEdits(compiled, edits, work)
	}
	first := len(compiled)
	for _, span := range layout.methods {
		if span.start < first {
			first = span.start
		}
	}
	if first < layout.poolEnd+2 || first > len(compiled) || binary.BigEndian.Uint16(compiled[first-2:]) != uint16(len(got.Methods)) {
		return nil, sourceLambdaMetadataFailure(work, "inherited bridge method span")
	}
	count := make([]byte, 2)
	binary.BigEndian.PutUint16(count, uint16(len(before)))
	edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{first - 2, first}, count})
	return applyCompiledMetadataEdits(compiled, edits, work)
}

func inheritedBridgeConsumersClosed(object *ClassObject, bridges map[*MemberInfo]bool, p *inheritedBridgePacket, work *workbudget.Budget) bool {
	refs := map[uint16]bool{}
	for i, c := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		r := nativeConstantMember(c)
		if r == nil {
			continue
		}
		owner, ok := sourceBridgeClassName(object, r.ClassIndex)
		if !ok || r.NameAndTypeIndex == 0 || int(r.NameAndTypeIndex) > len(object.ConstantPool) {
			return false
		}
		nt, ok := object.ConstantPool[r.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			return false
		}
		n, _ := sourceBridgeUTF8(object, nt.NameIndex)
		d, _ := sourceBridgeUTF8(object, nt.DescriptorIndex)
		if owner == object.GetClassName() && n == p.name && d == p.sourceDescriptor {
			refs[uint16(i+1)] = true
		}
	}
	for _, c := range object.ConstantPool {
		if h, ok := c.(*ConstantMethodHandleInfo); ok && h != nil && refs[h.ReferenceIndex] {
			return false
		}
	}
	calls := 0
	for _, m := range object.Methods {
		code := constructorLoweringCode(m)
		if code == nil {
			continue
		}
		if !nativeProofWork(work, int64(len(code.Code))+1) {
			return false
		}
		d := core.NewDecompiler(code.Code, nil)
		d.Work = work
		if d.ParseOpcode() != nil {
			return false
		}
		for _, op := range constructorMotionOps(d) {
			switch op.Instr.OpCode {
			case core.OP_INVOKEVIRTUAL, core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE:
				index, ok := sourceInvocationCPIndex(object, op, work)
				if !ok {
					return false
				}
				if refs[index] {
					if !bridges[m] || op.Instr.OpCode != core.OP_INVOKEVIRTUAL {
						return false
					}
					calls++
				}
			}
		}
	}
	// An empty admitted set proves that a removed forwarding node has no users.
	return calls == len(bridges)
}
