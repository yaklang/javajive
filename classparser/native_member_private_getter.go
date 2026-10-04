package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
)

type nativeMemberPrivateGetter struct {
	owner, name, descriptor, field, fieldDescriptor string
	ordinal                                         int
	method                                          *MemberInfo
}

func nativeMemberPrivateGetterProof(obj *ClassObject, m *MemberInfo, work *workbudget.Budget) *nativeMemberPrivateGetter {
	if obj == nil || m == nil || obj.MinorVersion != 0 || (obj.MajorVersion != 51 && obj.MajorVersion != 52) || m.AccessFlags != 0x1008 || !nativeProofWork(work, 1) {
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
	if err != nil || len(args) != 1 || args[0] != "L"+obj.GetClassName()+";" || result == "V" {
		return nil
	}
	var code *CodeAttribute
	for _, a := range m.Attributes {
		c, ok := a.(*CodeAttribute)
		if !ok || code != nil {
			return nil
		}
		code = c
	}
	if code == nil || code.MaxLocals != 1 || len(code.ExceptionTable) != 0 || len(code.Code) != 5 || !nativeProofWork(work, 5) {
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
			if a == nil || seenLocals || a.Name != "LocalVariableTable" || len(a.Info) != 12 || !nativeProofWork(work, 12) {
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
	if len(ops) != 3 || ops[0].Instr.OpCode != core.OP_ALOAD_0 {
		return nil
	}
	field := constructorMotionMember(obj, ops[1], core.OP_GETFIELD)
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
	if ops[2].Instr.OpCode != expected || len(ops[2].Data) != 0 || int(code.MaxStack) != width {
		return nil
	}
	found := false
	for _, f := range obj.Fields {
		if !nativeProofWork(work, 1) {
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
		if found || f.AccessFlags&2 == 0 || f.AccessFlags&(8|0x1000) != 0 {
			return nil
		}
		found = true
		for _, a := range f.Attributes {
			if _, constant := a.(*ConstantValueAttribute); constant {
				return nil
			}
		}
	}
	if !found {
		return nil
	}
	return &nativeMemberPrivateGetter{owner: obj.GetClassName(), name: name, descriptor: desc, field: field.Member, fieldDescriptor: result, ordinal: ordinal, method: m}
}
func nativeMemberGetterKey(owner, name, desc string) string {
	return strings.ReplaceAll(owner, ".", "/") + "\x00" + name + desc
}
func nativeMemberCollectPrivateGetters(p *nativeMemberFamily, work *workbudget.Budget) bool {
	p.getters = map[string]*nativeMemberPrivateGetter{}
	needsProjection := false
	for _, child := range p.children {
		if child.static {
			continue
		}
		for _, method := range child.object.Methods {
			if method == nil || !nativeProofWork(work, 1) {
				return false
			}
			name, known := sourceBridgeUTF8(child.object, method.NameIndex)
			if !known {
				return false
			}
			if method.AccessFlags&0x1000 != 0 && strings.HasPrefix(name, "access$") {
				needsProjection = true
			}
		}
	}
	// Static declarations already have a legal exact accessor body. Preserve
	// that established representation unless an instance-member accessor forces
	// javac to regenerate the whole shared ordinal sequence.
	if !needsProjection {
		return true
	}
	ordinals := map[int]bool{}
	fields := map[string]bool{}
	for _, obj := range p.lexicalObjects {
		for _, m := range obj.Methods {
			name, known := sourceBridgeUTF8(obj, m.NameIndex)
			if !known || !nativeProofWork(work, 1) {
				return false
			}
			if !strings.HasPrefix(name, "access$") || m.AccessFlags&0x1000 == 0 {
				continue
			}
			getter := nativeMemberPrivateGetterProof(obj, m, work)
			if getter == nil || ordinals[getter.ordinal] {
				return false
			}
			fieldKey := getter.owner + "\x00" + getter.field + getter.fieldDescriptor
			if fields[fieldKey] {
				return false
			}
			fields[fieldKey] = true
			ordinals[getter.ordinal] = true
			p.getters[nativeMemberGetterKey(getter.owner, getter.name, getter.descriptor)] = getter
		}
	}
	for i := 0; i < len(ordinals); i++ {
		if !ordinals[i*100] {
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
	if len(p.getters) == 0 {
		return
	}
	sites, known := nativeMemberGetterCallSites(c.obj, p, c.Work)
	if !known {
		p.failed = true
		return
	}
	ctx.SourcePrivateGetter = func(owner, name, desc string, pc int, args []any) (string, bool) {
		getter := p.getters[nativeMemberGetterKey(owner, name, desc)]
		if getter == nil {
			return "", false
		}
		if sites[ctx.FunctionName+ctx.CurrentMethodDesc][pc] != getter || len(args) != 1 {
			p.failed = true
			return "", false
		}
		v, ok := args[0].(values.JavaValue)
		if !ok || sourceProofNil(v) {
			p.failed = true
			return "", false
		}
		sourceOwner := ctx.ShortTypeName(strings.ReplaceAll(getter.owner, "/", "."))
		// A raw view retains the original accessor return erasure. The receiver is
		// evaluated once. javac regenerates the private GETFIELD accessor (including
		// its declaring-class initialization) instead of an illegal inner static
		// method. Its exact synthetic name is checked against final source order.
		return "(((" + sourceOwner + ")(" + v.String(ctx) + "))/*jdec-owned-getter:" + strconv.Itoa(getter.ordinal) + ":" + getter.owner + ":" + getter.field + "*/." + getter.field + ")", true
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
	// Anonymous lowering has its own traversal order. Do not assume that lexical
	// comment order is javac's accessor numbering through an anonymous allocation.
	if len(p.anonymousUnits) != 0 {
		return false
	}
	for key, getter := range p.getters {
		if !nativeProofWork(work, 1) || index.getterHandles[key] || index.getterInvalidReferences[key] {
			return false
		}
		used := false
		for user := range index.getterUsers[key] {
			object := p.lexicalObjects[user]
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
						actual = true
					}
				}
			}
			if !actual {
				return false
			}
			used = true
		}
		if !used {
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
	if !nativeProofWork(work, int64(len(source))) {
		return false
	}
	const prefix = "jdec-owned-getter:"
	expected := map[string]*nativeMemberPrivateGetter{}
	for _, getter := range p.getters {
		expected[strconv.Itoa(getter.ordinal)+":"+getter.owner+":"+getter.field] = getter
	}
	seen := map[*nativeMemberPrivateGetter]bool{}
	for i := 0; i < len(source); {
		ch := source[i]
		if ch == '\'' || ch == '"' {
			quote := ch
			i++
			closed := false
			for i < len(source) {
				if source[i] == '\\' {
					i += 2
					continue
				}
				if source[i] == quote {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return false
			}
			continue
		}
		if i+1 < len(source) && source[i:i+2] == "//" {
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}
		if i+1 < len(source) && source[i:i+2] == "/*" {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return false
			}
			comment := source[i+2 : i+2+end]
			if strings.HasPrefix(comment, prefix) {
				getter := expected[strings.TrimPrefix(comment, prefix)]
				if getter == nil {
					return false
				}
				if !seen[getter] {
					if getter.ordinal != len(seen)*100 {
						return false
					}
					seen[getter] = true
				}
			}
			i += end + 4
			continue
		}
		i++
	}
	return len(seen) == len(p.getters)
}
