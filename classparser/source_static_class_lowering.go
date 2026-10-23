package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A JVM static synthetic class may have no constructor. Java source cannot
// express either that absence or ACC_SYNTHETIC: javac adds an Object default
// constructor even when every original declaration is static. Publication is
// a separate flat-unit capability, never inferred from a generated-looking name.
func staticClassOriginalDomain(object *ClassObject, work *workbudget.Budget) bool {
	if object == nil || object.MajorVersion != 52 || object.MinorVersion != 0 || object.AccessFlags != 0x1020 || object.GetSupperClassName() != "java/lang/Object" || len(object.Interfaces) != 0 || len(object.Methods) == 0 || len(object.Methods) > 4096 {
		return false
	}
	for _, method := range object.Methods {
		name, known := sourceBridgeUTF8(object, method.NameIndex)
		if !known || name == "<init>" || method.AccessFlags&8 == 0 || !nativeProofWork(work, 1) {
			return false
		}
	}
	for _, field := range object.Fields {
		if field == nil || field.AccessFlags&8 == 0 || !nativeProofWork(work, 1) {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) publishStaticClassLowering(source string) bool {
	a := c.options.sourceConstructorLowering
	if a == nil || !c.reconstructionPhysicalHeader() || !staticClassOriginalDomain(c.obj, c.Work) {
		return true
	}
	if c.sourceDegraded || legacySourceHasFailureMarker(source) {
		a.mu.Lock()
		if a.staticPending == nil {
			a.staticPending = map[string]constructorNativeSourceOrigin{}
		}
		a.staticPending[c.obj.GetClassName()] = constructorNativeSourceOrigin{}
		a.mu.Unlock()
		return true
	}
	if !nativeProofWork(c.Work, int64(len(source))+1) {
		return false
	}
	raw, err := a.jar.readJarEntry(c.obj.GetClassName() + ".class")
	if err != nil || !bytes.Equal(raw, c.obj.Bytes()) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.staticPending == nil {
		a.staticPending = map[string]constructorNativeSourceOrigin{}
	}
	a.staticPending[c.obj.GetClassName()] = constructorNativeSourceOrigin{sha256.Sum256(raw), sha256.Sum256([]byte(source))}
	return true
}

func (a *sourceConstructorLoweringAuthority) staticClassSourceReturned(owner string, original []byte, work *workbudget.Budget) bool {
	if a == nil || !nativeProofWork(work, 1) {
		return false
	}
	a.mu.Lock()
	origin, active := a.staticActive[owner]
	_, pending := a.staticPending[owner]
	a.mu.Unlock()
	if !active && !pending && a.parent != nil && a.jar != nil && a.parent.jar != nil && a.jar.sourceCompiler.effective() == a.parent.jar.sourceCompiler.effective() && a.jar.targetSourceVersion == a.parent.jar.targetSourceVersion {
		a.parent.mu.Lock()
		origin, active = a.parent.staticActive[owner]
		a.parent.mu.Unlock()
	}
	return active && origin.sourceHash != ([32]byte{}) && origin.originalHash == sha256.Sum256(original)
}

func (a *sourceConstructorLoweringAuthority) lowerStaticClassDefaultConstructor(owner string, original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	if !a.staticClassSourceReturned(owner, original, work) {
		return decline()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return nil, constructorLoweringFailure(work, "static class input budget")
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
	if !staticClassOriginalDomain(want, work) || want.GetClassName() != owner || len(got.Methods) != len(want.Methods)+1 || got.MinorVersion != 0 || got.MajorVersion < 52 || got.MajorVersion > 65 {
		return decline()
	}
	withoutMarker := *want
	withoutMarker.AccessFlags &^= 0x1000
	if !sourceParameterClassIdentityClosed(&withoutMarker, got) {
		return decline()
	}
	if err := work.CheckAlloc(int64(len(original)+len(compiled))*5 + int64(len(want.Fields)+len(got.Fields)+len(want.Methods)+len(got.Methods)+1)*512); err != nil {
		return nil, err
	}
	fields, err := sourceFieldIdentities(want, work)
	if err != nil {
		return nil, err
	}
	freshFields, err := sourceFieldIdentities(got, work)
	if err != nil {
		return nil, err
	}
	if len(fields) != len(freshFields) {
		return decline()
	}
	for k, v := range fields {
		f, ok := freshFields[k]
		if !ok || v&^syntheticFieldFlag != f&^syntheticFieldFlag || f&syntheticFieldFlag != 0 && v&syntheticFieldFlag == 0 {
			return decline()
		}
	}
	// Legacy display strings are not JVM symbol identities. Check the raw
	// UTF-16 keys as well before granting even this narrow executable edit.
	rawFields := func(object *ClassObject) (map[sourceFieldIdentity]uint16, bool) {
		keys := map[sourceFieldIdentity]uint16{}
		for _, field := range object.Fields {
			name, nk := sourceParameterUTF8Key(object, field.NameIndex)
			desc, dk := sourceParameterUTF8Key(object, field.DescriptorIndex)
			key := sourceFieldIdentity{name, desc}
			if _, duplicate := keys[key]; !nk || !dk || duplicate || !nativeProofWork(work, int64(len(name)+len(desc))+1) {
				return nil, false
			}
			keys[key] = field.AccessFlags
		}
		return keys, true
	}
	wantKeys, wk := rawFields(want)
	gotKeys, gk := rawFields(got)
	if !wk || !gk || len(wantKeys) != len(gotKeys) {
		return decline()
	}
	for key, flags := range wantKeys {
		f, known := gotKeys[key]
		if !known || flags&^syntheticFieldFlag != f&^syntheticFieldFlag || f&syntheticFieldFlag != 0 && flags&syntheticFieldFlag == 0 {
			return decline()
		}
	}
	identity := func(o *ClassObject, m *MemberInfo) (sourceLambdaMethodIdentity, bool) {
		if m == nil {
			return sourceLambdaMethodIdentity{}, false
		}
		n, nk := sourceParameterUTF8Key(o, m.NameIndex)
		d, dk := sourceParameterUTF8Key(o, m.DescriptorIndex)
		return sourceLambdaMethodIdentity{n, d}, nk && dk
	}
	expected := map[sourceLambdaMethodIdentity]uint16{}
	for _, m := range want.Methods {
		k, ok := identity(want, m)
		if !ok {
			return decline()
		}
		if _, dup := expected[k]; dup {
			return decline()
		}
		expected[k] = m.AccessFlags
	}
	seen := map[sourceLambdaMethodIdentity]bool{}
	var constructor *MemberInfo
	for _, m := range got.Methods {
		k, ok := identity(got, m)
		if !ok || seen[k] || !nativeProofWork(work, int64(len(k.name)+len(k.descriptor))+1) {
			return decline()
		}
		seen[k] = true
		n, _ := sourceBridgeUTF8(got, m.NameIndex)
		d, _ := sourceBridgeUTF8(got, m.DescriptorIndex)
		if n == "<init>" && d == "()V" {
			if constructor != nil || m.AccessFlags != 0 {
				return decline()
			}
			constructor = m
			continue
		}
		if flags, known := expected[k]; !known || flags != m.AccessFlags {
			return decline()
		}
	}
	if constructor == nil || len(seen) != len(expected)+1 {
		return decline()
	}
	code := constructorLoweringCode(constructor)
	if code == nil || len(constructor.Attributes) != 1 || code.MaxLocals != 1 || code.MaxStack != 1 || len(code.ExceptionTable) != 0 || len(code.Code) != 5 || code.Code[0] != 0x2a || code.Code[1] != 0xb7 || code.Code[4] != 0xb1 {
		return decline()
	}
	ops, known := nativeEnumMethodOps(got, constructor, work)
	if !known || len(ops) != 3 || !nativeEnumMemberOperand(got, ops[1], core.OP_INVOKESPECIAL, "java/lang/Object", "<init>", "()V") || !staticClassDefaultDebug(got, code) {
		return decline()
	}
	// A fresh executable or MethodHandle consumer cannot borrow deletion of
	// a compiler default constructor. All original methods remain untouched.
	for _, constant := range got.ConstantPool {
		if !nativeProofWork(work, 1) {
			return decline()
		}
		if h, ok := constant.(*ConstantMethodHandleInfo); ok && h != nil {
			if h.ReferenceIndex == 0 || int(h.ReferenceIndex) > len(got.ConstantPool) {
				return decline()
			}
			r := nativeConstantMember(got.ConstantPool[h.ReferenceIndex-1])
			if r == nil {
				return decline()
			}
			name, ok := sourceBridgeClassName(got, r.ClassIndex)
			if !ok {
				return decline()
			}
			if name == owner {
				if r.NameAndTypeIndex == 0 || int(r.NameAndTypeIndex) > len(got.ConstantPool) {
					return decline()
				}
				nt, known := got.ConstantPool[r.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !known || nt == nil {
					return decline()
				}
				method, mk := sourceBridgeUTF8(got, nt.NameIndex)
				desc, dk := sourceBridgeUTF8(got, nt.DescriptorIndex)
				if !mk || !dk || method == "<init>" && desc == "()V" {
					return decline()
				}
			}
		}
	}
	for _, m := range got.Methods {
		body := constructorLoweringCode(m)
		if body == nil {
			continue
		}
		d := core.NewDecompiler(body.Code, nil)
		d.Work = work
		if !nativeProofWork(work, int64(len(body.Code))+1) || d.ParseOpcode() != nil {
			return decline()
		}
		for _, op := range constructorMotionOps(d) {
			if op.Instr.OpCode != core.OP_INVOKESPECIAL {
				continue
			}
			r := constructorMotionMember(got, op, core.OP_INVOKESPECIAL)
			if r == nil || r.Name == owner && r.Member == "<init>" {
				return decline()
			}
		}
	}
	span, ok := layout.methods[constructor]
	first := len(compiled)
	for _, s := range layout.methods {
		if s.start < first {
			first = s.start
		}
	}
	if !ok || first < layout.poolEnd+2 || first > len(compiled) || binary.BigEndian.Uint16(compiled[first-2:]) != uint16(len(got.Methods)) || layout.poolEnd < 10 || binary.BigEndian.Uint16(compiled[layout.poolEnd:]) != got.AccessFlags {
		return nil, constructorLoweringFailure(work, "static class declaration spans")
	}
	flags, count := make([]byte, 2), make([]byte, 2)
	binary.BigEndian.PutUint16(flags, want.AccessFlags)
	binary.BigEndian.PutUint16(count, uint16(len(got.Methods)-1))
	return applyCompiledMetadataEdits(compiled, []compiledMetadataEdit{{compiledMetadataSpan{layout.poolEnd, layout.poolEnd + 2}, flags}, {compiledMetadataSpan{first - 2, first}, count}, {span, nil}}, work)
}

func staticClassDefaultDebug(object *ClassObject, code *CodeAttribute) bool {
	seen := map[string]bool{}
	for _, attribute := range code.Attributes {
		switch a := attribute.(type) {
		case *LineNumberTableAttribute:
			if a == nil || seen["LineNumberTable"] {
				return false
			}
			seen["LineNumberTable"] = true
			for _, line := range a.LineNumberTable {
				if line == nil || line.StartPc != 0 && line.StartPc != 1 && line.StartPc != 4 {
					return false
				}
			}
		case *UnparsedAttribute:
			if a == nil || seen[a.Name] {
				return false
			}
			seen[a.Name] = true
			if a.Name == "StackMapTable" || a.Name == "LocalVariableTypeTable" {
				if !bytes.Equal(a.Info, []byte{0, 0}) {
					return false
				}
				continue
			}
			if a.Name != "LocalVariableTable" || len(a.Info) != 12 || binary.BigEndian.Uint16(a.Info) != 1 || binary.BigEndian.Uint16(a.Info[2:]) != 0 || binary.BigEndian.Uint16(a.Info[4:]) != 5 || binary.BigEndian.Uint16(a.Info[10:]) != 0 {
				return false
			}
			name, nk := sourceBridgeUTF8(object, binary.BigEndian.Uint16(a.Info[6:]))
			desc, dk := sourceBridgeUTF8(object, binary.BigEndian.Uint16(a.Info[8:]))
			if !nk || !dk || name != "this" || desc != "L"+object.GetClassName()+";" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
