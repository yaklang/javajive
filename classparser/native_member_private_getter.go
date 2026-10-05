package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
)

// Read and plain-write operations share one original field registration. Call
// packets retain their separately proved invocation and symbol identity. The
// legacy type name stays internal; setter selects the plain-write packet.
type nativeMemberPrivateGetter struct {
	owner, name, descriptor, field, fieldDescriptor string
	ordinal                                         int
	method                                          *MemberInfo
	setter, staticField, genericField               bool
	inheritedField                                  bool
	call                                            *nativeMemberPrivateCall
	update                                          *nativeMemberPrivateUpdate
}

func nativeMemberPrivateGetterProof(obj *ClassObject, m *MemberInfo, work *workbudget.Budget) *nativeMemberPrivateGetter {
	return nativeMemberGetterPacketProof(obj, m, nil, work)
}

func nativeMemberGetterPacketProof(obj *ClassObject, m *MemberInfo, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) *nativeMemberPrivateGetter {
	if obj == nil || m == nil || m.AccessFlags != 0x1008 || !nativeAccessorVersion(obj, work) || !nativeProofWork(work, 1) {
		return nil
	}
	name, nok := sourceBridgeUTF8(obj, m.NameIndex)
	desc, dok := sourceBridgeUTF8(obj, m.DescriptorIndex)
	suffix, prefix := strings.CutPrefix(name, "access$")
	ordinal, err := strconv.Atoi(suffix)
	if !nok || !dok || !prefix || err != nil || ordinal < 0 || ordinal%100 != 0 || fmt.Sprintf("%03d", ordinal) != suffix {
		return nil
	}
	args, result, err := callbinding.Descriptor(desc)
	if err != nil || len(args) > 1 || len(args) == 1 && args[0] != "L"+obj.GetClassName()+";" || result == "V" {
		return nil
	}
	staticField := len(args) == 0
	var code *CodeAttribute
	for _, a := range m.Attributes {
		c, ok := a.(*CodeAttribute)
		if !ok || code != nil {
			return nil
		}
		code = c
	}
	if code == nil || int(code.MaxLocals) != len(args) || len(code.ExceptionTable) != 0 || len(code.Code) != 4+len(args) || !nativeProofWork(work, 5) {
		return nil
	}
	seenLines, seenLocals := false, false
	if len(code.Attributes) > 2 {
		return nil
	}
	for _, attr := range code.Attributes {
		if !nativeProofWork(work, 1) {
			return nil
		}
		switch a := attr.(type) {
		case *LineNumberTableAttribute:
			if a == nil || seenLines || len(a.LineNumberTable) != 1 || a.LineNumberTable[0] == nil || a.LineNumberTable[0].StartPc != 0 {
				return nil
			}
			seenLines = true
		case *UnparsedAttribute:
			if a == nil || seenLocals || a.Name != "LocalVariableTable" || len(args) != 1 || len(a.Info) != 12 || !nativeProofWork(work, 12) {
				return nil
			}
			u := func(offset int) uint16 { return binary.BigEndian.Uint16(a.Info[offset : offset+2]) }
			n, nok := sourceBridgeUTF8(obj, u(6))
			d, dok := sourceBridgeUTF8(obj, u(8))
			if u(0) != 1 || u(2) != 0 || u(4) != 5 || u(10) != 0 || !nok || !dok || class_context.SafeIdentifier(n) != n || d != "L"+obj.GetClassName()+";" {
				return nil
			}
			seenLocals = true
		default:
			return nil
		}
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(decoder)
	if len(ops) != 2+len(args) || !staticField && ops[0].Instr.OpCode != core.OP_ALOAD_0 {
		return nil
	}
	fieldOp := core.OP_GETFIELD
	if staticField {
		fieldOp = core.OP_GETSTATIC
	}
	field := constructorMotionMember(obj, ops[len(args)], fieldOp)
	if field == nil || field.Name != obj.GetClassName() || field.Description != result || class_context.SafeIdentifier(field.Member) != field.Member {
		return nil
	}
	expected := core.OP_ARETURN
	width := 1
	switch result {
	case "J":
		expected = core.OP_LRETURN
		width = 2
	case "D":
		expected = core.OP_DRETURN
		width = 2
	case "F":
		expected = core.OP_FRETURN
	case "Z", "B", "C", "S", "I":
		expected = core.OP_IRETURN
	}
	if ops[len(args)+1].Instr.OpCode != expected || len(ops[len(args)+1].Data) != 0 || int(code.MaxStack) != width {
		return nil
	}
	found := false
	genericField := false
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nok := sourceBridgeUTF8(obj, f.NameIndex)
		d, dok := sourceBridgeUTF8(obj, f.DescriptorIndex)
		if !nok || !dok {
			return nil
		}
		if n != field.Member || d != result {
			continue
		}
		if found || f.AccessFlags&2 == 0 || f.AccessFlags&0x1000 != 0 || (f.AccessFlags&8 != 0) != staticField {
			return nil
		}
		found = true
		for _, a := range f.Attributes {
			if _, generic := a.(*SignatureAttribute); generic {
				genericField = true
			}
			if _, constant := a.(*ConstantValueAttribute); constant {
				return nil
			}
		}
	}
	if !found {
		if !staticField || resolve == nil {
			return nil
		}
		owner, target := nativeMemberInheritedFieldTarget(obj, field.Member, result, resolve, work)
		if target == nil || owner == nil || owner == obj || target.AccessFlags&7 != 4 || target.AccessFlags&(8|0x1000|0x4000) != 8 || nativeBinaryPackage(owner.GetClassName()) == nativeBinaryPackage(obj.GetClassName()) {
			return nil
		}
		genericField, found = nativeMemberInheritedFieldSignature(owner, target, result, work)
		if !found {
			return nil
		}
		return &nativeMemberPrivateGetter{owner: obj.GetClassName(), name: name, descriptor: desc, field: field.Member, fieldDescriptor: result, ordinal: ordinal, method: m, staticField: true, genericField: genericField, inheritedField: true}
	}
	return &nativeMemberPrivateGetter{owner: obj.GetClassName(), name: name, descriptor: desc, field: field.Member, fieldDescriptor: result, ordinal: ordinal, method: m, staticField: staticField, genericField: genericField}
}
func nativeMemberGetterKey(owner, name, desc string) string {
	return strings.ReplaceAll(owner, ".", "/") + "\x00" + name + desc
}
func nativeMemberCollectPrivateGetters(p *nativeMemberFamily, work *workbudget.Budget) bool {
	return nativeMemberCollectPrivateGettersResolved(p, nil, work)
}
func nativeMemberCollectPrivateGettersResolved(p *nativeMemberFamily, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	p.getters = map[string]*nativeMemberPrivateGetter{}
	needsProjection := false
	for _, object := range p.lexicalObjects {
		if object == nil || !nativeProofWork(work, 1) {
			return false
		}
		for _, method := range object.Methods {
			if method == nil || !nativeProofWork(work, 1) {
				return false
			}
			name, known := sourceBridgeUTF8(object, method.NameIndex)
			if !known {
				return false
			}
			if method.AccessFlags&0x1000 != 0 && strings.HasPrefix(name, "access$") {
				needsProjection = true
			}
		}
	}
	// A source declaration cannot retain ACC_SYNTHETIC, even when a static
	// accessor body is otherwise legal Java. Project the entire original lexical
	// family together so javac regenerates its flags and shared ordinal sequence;
	// every packet, symbolic user and final source order is still proved below.
	if !needsProjection {
		return true
	}
	ordinals := map[int]string{}
	fields := map[string]int{}
	operations := map[string]bool{}
	for _, obj := range p.lexicalObjects {
		for _, m := range obj.Methods {
			name, known := sourceBridgeUTF8(obj, m.NameIndex)
			if !known || !nativeProofWork(work, 1) {
				return false
			}
			if !strings.HasPrefix(name, "access$") || m.AccessFlags&0x1000 == 0 {
				continue
			}
			getter := nativeMemberPrivateAccessProof(obj, m, work)
			if getter == nil && resolve != nil {
				getter = nativeMemberProtectedCallProof(obj, m, resolve, work)
			}
			if getter == nil && resolve != nil {
				getter = nativeMemberProtectedStaticFieldProof(obj, m, resolve, work)
			}
			if getter == nil {
				return false
			}
			fieldKey := nativeMemberAccessorSymbolKey(getter)
			if prior, known := fields[fieldKey]; known && prior != getter.ordinal {
				return false
			}
			if prior, known := ordinals[getter.ordinal]; known && prior != fieldKey {
				return false
			}
			operationKey := fieldKey + "\x00read"
			if getter.setter {
				operationKey = fieldKey + "\x00write"
			} else if getter.update != nil {
				operationKey = fieldKey + "\x00update:" + strconv.Itoa(getter.update.accessCode)
			}
			if operations[operationKey] {
				return false
			}
			operations[operationKey] = true
			fields[fieldKey] = getter.ordinal
			ordinals[getter.ordinal] = fieldKey
			p.getters[nativeMemberGetterKey(getter.owner, getter.name, getter.descriptor)] = getter
		}
	}
	constructors, known := nativeMemberConstructorRegistrations(p, work)
	if !known {
		return false
	}
	// javac's shared accessed-symbol sequence also contains private
	// constructors, which have no access$NNN method. Every intervening event
	// still has to be regenerated at its actual source site below.
	for ordinal := range ordinals {
		if ordinal/100 >= len(ordinals)+len(constructors) {
			return false
		}
	}
	return true
}

func nativeMemberGetterCallSites(obj *ClassObject, p *nativeMemberFamily, work *workbudget.Budget) (map[string]map[int]*nativeMemberPrivateGetter, bool) {
	out := map[string]map[int]*nativeMemberPrivateGetter{}
	for _, m := range obj.Methods {
		if m == nil {
			return nil, false
		}
		name, nok := sourceBridgeUTF8(obj, m.NameIndex)
		desc, dok := sourceBridgeUTF8(obj, m.DescriptorIndex)
		if !nok || !dok || !nativeProofWork(work, 1) {
			return nil, false
		}
		sites := map[int]*nativeMemberPrivateGetter{}
		if out[name+desc] != nil {
			return nil, false
		}
		out[name+desc] = sites
		codeSeen := false
		for _, a := range m.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			if codeSeen {
				return nil, false
			}
			codeSeen = true
			if !nativeProofWork(work, int64(len(code.Code))) {
				return nil, false
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			decoder.Work = work
			if decoder.ParseOpcode() != nil {
				return nil, false
			}
			for _, op := range decoder.Opcodes() {
				call := constructorMotionMember(obj, op, op.Instr.OpCode)
				if op.Instr.OpCode != core.OP_INVOKESTATIC && op.Instr.OpCode != core.OP_INVOKEVIRTUAL && op.Instr.OpCode != core.OP_INVOKESPECIAL && op.Instr.OpCode != core.OP_INVOKEINTERFACE {
					call = nil
				}
				if call != nil {
					if getter := p.getters[nativeMemberGetterKey(call.Name, call.Member, call.Description)]; getter != nil {
						// An own-class call would no longer require javac's private accessor.
						// Its class-init/stack-frame protocol needs a separate source proof.
						if obj.GetClassName() == getter.owner || op.Instr.OpCode != core.OP_INVOKESTATIC {
							return nil, false
						}
						sites[int(op.CurrentOffset)] = getter
					}
				}
			}
		}
	}
	return out, true
}

func (c *ClassObjectDumper) wireNativeMemberPrivateGetters(p *nativeMemberFamily, ctx *class_context.ClassContext) {
	constructors, known := nativeMemberConstructorRegistrations(p, c.Work)
	if !known {
		p.failed = true
		return
	}
	if len(p.getters) != 0 || len(constructors) != 0 {
		ctx.SourceInvocationReceiver = func(source string) (string, string) {
			receiver, registration, valid := nativeInvocationReceiverSource(p, source, constructors, c.Work)
			if !valid {
				p.failed = true
			}
			return receiver, registration
		}
	}
	if len(p.getters) == 0 {
		return
	}
	sites, known := nativeMemberGetterCallSites(c.obj, p, c.Work)
	if !known {
		p.failed = true
		return
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	lexicalStatic := map[*nativeMemberPrivateGetter]bool{}
	ctx.SourcePrivateGetter = func(owner, name, desc string, pc int, args []any, statement bool) (string, bool) {
		getter := p.getters[nativeMemberGetterKey(owner, name, desc)]
		if getter == nil {
			return "", false
		}
		sourceOwner := ctx.ShortTypeName(strings.ReplaceAll(getter.owner, "/", "."))
		unqualified := getter.staticField && nativeStaticAccessorQualifierShadowed(sourceOwner, ctx)
		// Provisional IR rendering cannot commit a lexical binding before this
		// method has reserved its actual parameter/local declaration identities.
		if unqualified && !c.nativeSourceNamesReady {
			return "", false
		}
		if unqualified {
			if getter.inheritedField {
				p.failed = true
				return "", false
			}
			if _, checked := lexicalStatic[getter]; !checked {
				lexicalStatic[getter] = nativeStaticAccessorLexicalField(getter, c.obj.GetClassName(), p, resolve, c.Work)
			}
			if !lexicalStatic[getter] || c.nativeCaptureFailed {
				p.failed = true
				return "", false
			}
		}
		expectedArgs := 1
		if getter.staticField {
			expectedArgs = 0
		}
		if getter.setter || getter.update != nil && !getter.update.unary {
			expectedArgs++
		}
		if getter.call != nil {
			expectedArgs = getter.call.argumentCount
		}
		if sites[ctx.FunctionName+ctx.CurrentMethodDesc][pc] != getter || len(args) != expectedArgs {
			p.failed = true
			return "", false
		}
		var v values.JavaValue
		if !getter.staticField && (getter.call == nil || !getter.call.static) {
			var ok bool
			v, ok = args[0].(values.JavaValue)
			if !ok || sourceProofNil(v) {
				p.failed = true
				return "", false
			}
		}
		if getter.call != nil {
			source, known := nativeMemberPrivateCallSource(getter, args, ctx)
			if !known {
				p.failed = true
			}
			return source, known
		}
		if getter.update != nil {
			source, known := nativeMemberPrivateUpdateSource(getter, args, sourceOwner, ctx, statement)
			if !known {
				p.failed = true
			}
			return source, known
		}
		// A source write regenerates the original static accessor: the receiver and
		// RHS complete before its class initialization and null dereference. javac
		// registers the source LHS field before lowering RHS accessors; the
		// marker records that compiler order without moving runtime evaluation.
		if getter.setter {
			rhs, ok := args[1].(values.JavaValue)
			if !ok || sourceProofNil(rhs) {
				p.failed = true
				return "", false
			}
			mt, e := types.ParseMethodDescriptor(getter.descriptor)
			if e != nil {
				p.failed = true
				return "", false
			}
			// The Z bridge parameter is a JVM int word. Java has no numeric-to-
			// boolean cast: project bit zero at this consumer, never retype a
			// shared producer. The view retains the operand's single evaluation.
			var value string
			if getter.fieldDescriptor == "Z" {
				view, known := values.BooleanStackConsumerView(rhs)
				if !known {
					p.failed = true
					return "", false
				}
				value = view.String(ctx)
			} else {
				value = "((" + mt.FunctionType().ReturnType.String(ctx) + ")(" + rhs.String(ctx) + "))"
			}
			assignment := fmt.Sprintf("((%s)(%s)).%s/*jdec-owned-getter:%d:%s:%s:put*/ = %s", sourceOwner, v.String(ctx), getter.field, getter.ordinal, getter.owner, getter.field, value)
			if statement {
				return assignment, true
			}
			return "(" + assignment + ")", true
		}

		// A discarded read needs a separately proved statement materialization;
		// a bare field read is not a Java statement expression. Fail this family
		// closed rather than silently dropping its dereference/initialization.
		if statement {
			p.failed = true
			return "", false
		}
		// The accessor result has its original descriptor erasure. Merely spelling
		// the source field can reintroduce generic arguments (especially for static
		// fields of nongeneric owners) and change downstream overload/inference.
		// A same-erasure reference view keeps the field access/registration intact.
		readView := func(source string) (string, bool) {
			if !getter.genericField || !strings.HasPrefix(getter.fieldDescriptor, "L") && !strings.HasPrefix(getter.fieldDescriptor, "[") {
				return source, true
			}
			typ, e := types.ParseDescriptor(getter.fieldDescriptor)
			if e != nil {
				p.failed = true
				return "", false
			}
			spelling := typ.String(ctx)
			if nativeStaticAccessorQualifierShadowed(strings.TrimRight(spelling, "[]"), ctx) {
				p.failed = true
				return "", false
			}
			return "((" + spelling + ")(" + source + "))", true
		}
		// A static field access performs declaring-class initialization at the same
		// point as the original zero-argument INVOKESTATIC access bridge.
		if getter.staticField {
			if unqualified {
				return readView("(/*jdec-owned-getter:" + strconv.Itoa(getter.ordinal) + ":" + getter.owner + ":" + getter.field + "*/" + getter.field + ")")
			}
			return readView("(" + sourceOwner + "/*jdec-owned-getter:" + strconv.Itoa(getter.ordinal) + ":" + getter.owner + ":" + getter.field + "*/." + getter.field + ")")
		}

		// A raw view retains the original accessor return erasure. The receiver is
		// evaluated once. javac regenerates the private GETFIELD accessor (including
		// its declaring-class initialization) instead of an illegal inner static
		// method. Its exact synthetic name is checked against final source order.
		return readView("(((" + sourceOwner + ")(" + v.String(ctx) + "))/*jdec-owned-getter:" + strconv.Itoa(getter.ordinal) + ":" + getter.owner + ":" + getter.field + "*/." + getter.field + ")")
	}
}

// Dropping a synthetic declaration is safe only when every original symbolic
// user belongs to this compilation unit and actually invokes the proved getter.
// A method handle or an opaque/dead CP dependency needs a separate ABI proof.
func nativeMemberPrivateGetterReferencesClosed(p *nativeMemberFamily, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if p == nil || index == nil || !index.valid {
		return false
	}
	if len(p.getters) == 0 {
		return true
	}
	// Anonymous users require the same committed ownership forest. The final
	// source-order proof still checks every regenerated accessor ordinal;
	// merely sharing a binary-name prefix never licenses private access.
	if len(p.anonymousUnits) != 0 {
		if p.anonymousForest == nil || p.anonymousForest.members != p {
			return false
		}
		for user := range p.anonymousUnits {
			if !nativeMemberJointAnonymousAccess(p, user, work) {
				return false
			}
		}
	}
	for key, getter := range p.getters {
		if !nativeProofWork(work, 1) || index.getterHandles[key] || index.getterInvalidReferences[key] {
			return false
		}
		used := false
		siteCount := 0
		for user := range index.getterUsers[key] {
			object := p.lexicalObjects[user]
			if object == nil && p.anonymousForest != nil && nativeMemberJointAnonymousAccess(p, user, work) {
				object = p.anonymousForest.objects[user]
			}
			if object == nil || user == getter.owner {
				return false
			}
			sites, known := nativeMemberGetterCallSites(object, p, work)
			if !known {
				return false
			}
			actual := false
			for _, method := range sites {
				for _, call := range method {
					if call == getter {
						siteCount++
						actual = true
					}
				}
			}
			if !actual {
				return false
			}
			used = true
		}
		if !used || nativeMemberAccessorClonedSymbol(getter) && siteCount != 1 {
			return false
		}
	}
	return true
}

// javac assigns access$NNN when it first lowers a private access. Preserve the
// original global ordinal rather than merely producing a similarly named helper.
// Markers follow the receiver, so its nested getters precede the outer getter.
func nativeMemberPrivateGetterSourceClosed(p *nativeMemberFamily, source string, work *workbudget.Budget) bool {
	if p == nil || p.failed {
		return false
	}
	if len(p.getters) == 0 {
		return true
	}
	constructors, known := nativeMemberConstructorRegistrations(p, work)
	if !known {
		return false
	}
	events, known := nativeMemberAccessorEvents(p, source, constructors, work)
	if !known {
		return false
	}
	state := newNativeAccessorOrderState()
	if !state.apply(events) {
		return false
	}
	return len(state.getters) == len(p.getters) && len(state.constructors) == len(constructors)
}
