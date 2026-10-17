package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func javaAnnotationStringLiteral(v any) string {
	units := annotationStringUnits(v)
	if units != nil {
		return values.JavaUnitsToStringLiteral(units)
	}
	return values.JavaStringToLiteral(v)
}

func utf16UnitsEqual(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func javaUtf8IndexToStringLiteral(obj *ClassObject, index uint16) string {
	if obj == nil {
		return values.JavaStringToLiteral("")
	}
	units, err := obj.getUtf8Units(index)
	if err == nil && units != nil {
		return values.JavaUnitsToStringLiteral(units)
	}
	s, _ := obj.getUtf8(index)
	return values.JavaStringToLiteral(s)
}

func captureReaderWindow(r *ClassReader) []byte {
	if r == nil || r.Remaining() == 0 {
		return nil
	}
	if !r.reserve(int64(r.Remaining()), 1) {
		return nil
	}
	return append([]byte{}, r.orig[r.pos:r.end]...)
}

func (a *RuntimeVisibleParameterAnnotationsAttribute) readInfo(cp *ClassParser) {
	a.Info = captureReaderWindow(cp.reader)
	a.NumParameters = cp.reader.readUint8()
	if cp.reader.Err() != nil {
		return
	}
	if !cp.reader.reserve(int64(a.NumParameters), 2) {
		return
	}
	a.ParameterAnnotations = make([][]*AnnotationAttribute, a.NumParameters)
	for i := range a.ParameterAnnotations {
		n := cp.reader.readUint16()
		if cp.reader.Err() != nil {
			return
		}
		if !cp.reader.reserve(int64(n), 4) {
			return
		}
		list := make([]*AnnotationAttribute, n)
		for j := range list {
			list[j] = ParseAnnotation(cp)
			if cp.reader.Err() != nil {
				return
			}
		}
		a.ParameterAnnotations[i] = list
	}
}

func paramAnnosFromMethod(method *MemberInfo, invisible bool) *RuntimeVisibleParameterAnnotationsAttribute {
	if method == nil {
		return nil
	}
	for _, attr := range method.Attributes {
		pa, ok := attr.(*RuntimeVisibleParameterAnnotationsAttribute)
		if !ok {
			continue
		}
		if pa.IsInvisible == invisible {
			return pa
		}
	}
	return nil
}

func descriptorSyntheticPrefix(c *ClassObjectDumper, methodName string) int {
	if methodName != "<init>" || c == nil {
		return 0
	}
	// Enum constructors are dumped without the synthetic String/int prefix.
	if c.isGenuineEnum() {
		return 2
	}
	// Native ownership projects the proved enclosing operand from the source
	// parameter list; original descriptor/table identities remain unchanged.
	if c.nativeMemberCurrent != nil && !c.nativeMemberCurrent.static {
		return 1
	}
	return 0
}

func dumpedParamAnnoIndex(c *ClassObjectDumper, methodName, descriptor string, dumpedCount, tableLen, dumpedIndex int) int {
	descCount := len(methodParamFieldDescriptors(descriptor))
	prefix := descriptorSyntheticPrefix(c, methodName)
	userStart := 0
	if methodName == "<init>" && prefix == 0 && c != nil && c.hasOuterThisField() && !c.isGenuineEnum() {
		// Outer this is still present in the dumped parameter list.
		userStart = 1
	}
	if tableLen == descCount {
		di := prefix + dumpedIndex
		if di >= 0 && di < tableLen {
			return di
		}
		return -1
	}
	// javac often stores a source-arity table (not JVMS descriptor arity) on
	// constructors with mandated/synthetic parameters. Map user formals in order.
	if dumpedIndex < userStart {
		return -1
	}
	src := dumpedIndex - userStart
	if src >= 0 && src < tableLen {
		return src
	}
	return -1
}

func (c *ClassObjectDumper) formatAnnotationList(annos []*AnnotationAttribute) string {
	if len(annos) == 0 {
		return ""
	}
	parts := make([]string, 0, len(annos))
	for _, a := range annos {
		if a == nil {
			continue
		}
		s, err := c.DumpAnnotation(a)
		if err != nil || s == "" {
			continue
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func (c *ClassObjectDumper) applyParameterAnnotations(method *MemberInfo, methodName, descriptor string, decls []string) []string {
	if method == nil || len(decls) == 0 {
		return decls
	}
	vis := paramAnnosFromMethod(method, false)
	inv := paramAnnosFromMethod(method, true)
	if vis == nil && inv == nil {
		return decls
	}
	visSlots, invSlots := [][]*AnnotationAttribute(nil), [][]*AnnotationAttribute(nil)
	if vis != nil {
		visSlots = vis.ParameterAnnotations
	}
	if inv != nil {
		invSlots = inv.ParameterAnnotations
	}
	out := append([]string{}, decls...)
	for i := range out {
		var parts []string
		if vi := dumpedParamAnnoIndex(c, methodName, descriptor, len(out), len(visSlots), i); vi >= 0 && vi < len(visSlots) {
			if s := c.formatAnnotationList(visSlots[vi]); s != "" {
				parts = append(parts, s)
			}
		}
		if ii := dumpedParamAnnoIndex(c, methodName, descriptor, len(out), len(invSlots), i); ii >= 0 && ii < len(invSlots) {
			if s := c.formatAnnotationList(invSlots[ii]); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			out[i] = strings.Join(parts, " ") + " " + out[i]
		}
	}
	return out
}

// MethodParameters indexes logical descriptor parameters, not JVM local words
// or the shorter source parameter list. Retain original finality after source
// projection; Java's effectively-final capture rule does not recreate this flag.
func (c *ClassObjectDumper) originalSourceParameterFinality(method *MemberInfo, descriptor string) (map[int]bool, bool) {
	if c == nil || c.obj == nil || method == nil || !nativeProofWork(c.Work, 1) {
		return nil, false
	}
	actual, known := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	physical, _, err := callbinding.Descriptor(descriptor)
	if !known || actual != descriptor || err != nil || len(physical) > 255 {
		return nil, false
	}
	var table *UnparsedAttribute
	for _, a := range method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if p, ok := a.(*UnparsedAttribute); ok {
			if p == nil {
				return nil, false
			}
			if p.Name == "MethodParameters" {
				if table != nil {
					return nil, false
				}
				table = p
			}
		}
	}
	if table == nil {
		return nil, true
	}
	// A newer producer can attach this table to an older instruction subset.
	// The supported VM reads it independently of classfile major, so retain
	// its observable declaration flags after validating the actual packet.
	if c.obj.MajorVersion < 45 || table.Length != uint32(len(table.Info)) || len(table.Info) != 1+4*len(physical) || int(table.Info[0]) != len(physical) || !nativeProofWork(c.Work, int64(len(table.Info))) || c.Work != nil && c.Work.CheckAlloc(int64(len(physical))*64) != nil {
		return nil, false
	}
	final := map[int]bool{}
	for i := range physical {
		entry := table.Info[1+4*i : 5+4*i]
		index := binary.BigEndian.Uint16(entry[:2])
		flags := binary.BigEndian.Uint16(entry[2:])
		if flags & ^uint16(0x9010) != 0 {
			return nil, false
		}
		if index != 0 {
			name, known := sourceBridgeUTF8(c.obj, index)
			if !known || name == "" {
				return nil, false
			}
		}
		if flags&0x10 != 0 {
			final[i] = true
		}
	}
	return final, true
}

// Both Code and abstract declarations project the same physical parameter
// table. This mapping concerns logical parameters, never JVM local words.
func (c *ClassObjectDumper) applyOriginalSourceParameterFinality(method *MemberInfo, descriptor string, declarations []string) ([]string, bool) {
	final, known := c.originalSourceParameterFinality(method, descriptor)
	physical, _, err := callbinding.Descriptor(descriptor)
	offset := len(physical) - len(declarations)
	if !known || err != nil || offset < 0 {
		return nil, false
	}
	out := append([]string(nil), declarations...)
	for i := range out {
		if final[offset+i] {
			out[i] = "final " + out[i]
		}
	}
	return out, true
}

// A MethodParameters final bit describes the entry declaration, not the
// storage of a JVM local slot after a STORE. Keep the original immutable
// parameter as the source header, and give mutable body storage a fresh
// declaration initialized by an inert, same-type entry copy. All rebinding is
// by typed declaration identity; source text is never scanned or replaced.
//
// The bounded domain is ordinary methods with complete original frames and
// same-category full-slot primitive writes. Copying a reference entry would
// keep its old referent live after the original JVM slot is overwritten. That
// needs a separate lifetime proof, including weak/finalizer observations.
// Partial category-2 overlap, constructor
// headers, dynamic bodies and lexical captures need separate definition webs.
func (c *ClassObjectDumper) prepareFinalParameterStorage(method *MemberInfo, descriptor string, code *CodeAttribute, header []values.JavaValue, body []statements.Statement, lexicalDeclarations bool) ([]values.JavaValue, []statements.Statement, bool) {
	final, known := c.originalSourceParameterFinality(method, descriptor)
	if !known || code == nil {
		return nil, nil, false
	}
	if len(final) == 0 {
		return header, body, true
	}
	physical, result, err := callbinding.Descriptor(descriptor)
	if err != nil || result == "" || len(physical) > 255 || len(code.Code) == 0 || len(code.Code) > 65535 || !nativeProofWork(c.Work, int64(len(code.Code))+1) {
		return nil, nil, false
	}
	slots := make([]int, len(physical))
	finalWordOwner := map[int]int{}
	words := 0
	if method.AccessFlags&8 == 0 {
		words = 1
	}
	for i, p := range physical {
		slots[i] = words
		if final[i] {
			finalWordOwner[words] = i
			if p == "J" || p == "D" {
				finalWordOwner[words+1] = i
			}
		}
		words++
		if p == "J" || p == "D" {
			words++
		}
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return nil, nil, false
	}
	written := map[int]bool{}
	dynamic := false
	for _, op := range decoder.Opcodes() {
		if op == nil || op.Instr == nil || !nativeProofWork(c.Work, 1) {
			return nil, nil, false
		}
		dynamic = dynamic || op.Instr.OpCode == core.OP_INVOKEDYNAMIC
		access := core.LocalAccessOf(op.Instr.OpCode)
		if !access.Write {
			continue
		}
		store := core.GetStoreIdx(op)
		if store < 0 || access.Width < 1 || access.Width > 2 {
			return nil, nil, false
		}
		for word := 0; word < access.Width; word++ {
			if !nativeProofWork(c.Work, 1) {
				return nil, nil, false
			}
			i, overlaps := finalWordOwner[store+word]
			if !overlaps {
				continue
			}
			width := 1
			if physical[i] == "J" || physical[i] == "D" {
				width = 2
			}
			if store != slots[i] || access.Width != width || !originalParameterStoreCategory(op.Instr.OpCode, physical[i]) {
				return nil, nil, false
			}
			written[i] = true
		}
	}
	if len(written) == 0 {
		return header, body, true
	}
	for i := range written {
		if !nativeProofWork(c.Work, 1) || callbinding.Reference(physical[i]) {
			return nil, nil, false
		}
	}
	name, named := sourceBridgeUTF8(c.obj, method.NameIndex)
	if !named || name == "<init>" || name == "<clinit>" || c.FuncCtx == nil || len(header) != len(physical) || lexicalDeclarations || dynamic || c.nativeAnonymousRoot != nil && len(c.nativeAnonymousRoot.children) != 0 {
		return nil, nil, false
	}
	headerIDs := map[*coreutils.VariableId]bool{}
	for i, parameter := range header {
		ref, ok := parameter.(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || headerIDs[ref.Id] || !nativeProofWork(c.Work, 1) {
			return nil, nil, false
		}
		slot, original := ref.OriginalParameterSlot()
		if !original || slot != slots[i] {
			return nil, nil, false
		}
		headerIDs[ref.Id] = true
	}
	matches, codes := 0, 0
	for _, original := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil, nil, false
		}
		if original == method {
			matches++
		}
	}
	for _, a := range method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return nil, nil, false
		}
		if original, ok := a.(*CodeAttribute); ok {
			codes++
			if original != code {
				return nil, nil, false
			}
		}
	}
	if matches != 1 || codes != 1 {
		return nil, nil, false
	}
	if _, _, valid := c.nativeOriginalMethodSnapshot(method, code); !valid {
		return nil, nil, false
	}
	refs, valid := c.finalParameterStorageReferences(header, body)
	if !valid || c.Work != nil && c.Work.CheckAlloc(int64(len(refs))*128+int64(len(physical))*512) != nil {
		return nil, nil, false
	}
	used := map[string]bool{}
	aliases := map[*coreutils.VariableId][]*values.JavaRef{}
	for _, ref := range refs {
		used[ref.String(c.FuncCtx)] = true
		aliases[ref.Id] = append(aliases[ref.Id], ref)
	}
	for _, name := range c.FuncCtx.Arguments {
		used[name] = true
	}
	for _, name := range c.FuncCtx.LocalNames {
		used[name] = true
	}
	projected := append([]values.JavaValue(nil), header...)
	rebinding := map[*coreutils.VariableId]*coreutils.VariableId{}
	var prefix []statements.Statement
	var storageNames []string
	for i, parameter := range header {
		if !written[i] {
			continue
		}
		ref, ok := parameter.(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || ref.CustomValue != nil || ref.StackVar != nil || rebinding[ref.Id] != nil {
			return nil, nil, false
		}
		slot, sealed := ref.OriginalParameterSlot()
		erased, denotable := values.SourceTypeErasure(ref.Type(), c.FuncCtx)
		if !sealed || slot != slots[i] || !denotable || erased != physical[i] {
			return nil, nil, false
		}
		// A spelling shared by incompatible local definitions is not a
		// same-type storage cell. This projection cannot split that web.
		for _, alias := range aliases[ref.Id] {
			aliasErasure, valid := values.SourceTypeErasure(alias.Type(), c.FuncCtx)
			if !valid || aliasErasure != erased || !nativeProofWork(c.Work, 1) {
				return nil, nil, false
			}
		}
		// The new header has no original-slot witness. It cannot grant any
		// physical capture or initialization permission to a source consumer.
		entry := values.NewJavaRef(ref.Id, nil, ref.Type().Copy())
		entry.IsParam = true
		fresh := coreutils.NewRootVariableId()
		for suffix := 0; ; suffix++ {
			if !nativeProofWork(c.Work, 1) {
				return nil, nil, false
			}
			candidate := fmt.Sprintf("jdecParameterStorage%d_%d", i, suffix)
			if !used[candidate] {
				fresh.SetName(candidate)
				used[candidate] = true
				break
			}
		}
		rebinding[ref.Id] = fresh
		storageNames = append(storageNames, fresh.String())
		projected[i] = entry
		prefix = append(prefix, &statements.AssignStatement{LeftValue: ref, JavaValue: entry, IsFirst: true})
	}
	// No mutation precedes the complete proof above. Retain original seeds,
	// VarUid and STORE witnesses; only this source view's binder IDs change.
	for _, ref := range refs {
		if fresh := rebinding[ref.Id]; fresh != nil {
			ref.Id = fresh
		}
	}
	// The source type renderer consults Arguments when qualifying a static
	// owner shadowed by a value name. The caller restores this method scope.
	arguments := append([]string(nil), c.FuncCtx.Arguments...)
	arguments = append(arguments, storageNames...)
	c.FuncCtx.Arguments = arguments
	return projected, append(prefix, body...), true
}

func originalParameterStoreCategory(opcode int, descriptor string) bool {
	switch descriptor {
	case "Z", "B", "C", "S", "I":
		return opcode == core.OP_IINC || opcode == core.OP_ISTORE || opcode >= core.OP_ISTORE_0 && opcode <= core.OP_ISTORE_3
	case "J":
		return opcode == core.OP_LSTORE || opcode >= core.OP_LSTORE_0 && opcode <= core.OP_LSTORE_3
	case "F":
		return opcode == core.OP_FSTORE || opcode >= core.OP_FSTORE_0 && opcode <= core.OP_FSTORE_3
	case "D":
		return opcode == core.OP_DSTORE || opcode >= core.OP_DSTORE_0 && opcode <= core.OP_DSTORE_3
	default:
		return opcode == core.OP_ASTORE || opcode >= core.OP_ASTORE_0 && opcode <= core.OP_ASTORE_3
	}
}

func (c *ClassObjectDumper) finalParameterStorageReferences(params []values.JavaValue, body []statements.Statement) ([]*values.JavaRef, bool) {
	refs := map[*values.JavaRef]bool{}
	activeValues := map[values.JavaValue]bool{}
	activeStatements := map[statements.Statement]bool{}
	remaining := 8192
	var value func(values.JavaValue, int) bool
	value = func(v values.JavaValue, depth int) bool {
		if sourceProofNil(v) {
			return true // Optional receiver/array-length dependencies are absent.
		}
		remaining--
		if depth >= 128 || remaining < 0 || activeValues[v] || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(8192-remaining)*192) != nil {
			return false
		}
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				return false
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		activeValues[v] = true
		defer delete(activeValues, v)
		if ref, ok := v.(*values.JavaRef); ok && ref.CustomValue == nil && ref.StackVar == nil {
			if ref.Id == nil {
				return false
			}
			refs[ref] = true
			return true
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !value(child, depth+1) {
				return false
			}
		}
		return true
	}
	var walk func([]statements.Statement, int) bool
	walk = func(list []statements.Statement, depth int) bool {
		if depth >= 128 {
			return false
		}
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				return false
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		for _, st := range list {
			remaining--
			if sourceProofNil(st) || remaining < 0 || activeStatements[st] || !nativeProofWork(c.Work, 1) {
				return false
			}
			activeStatements[st] = true
			roots, children, known := nativeSourceNameChildren(st)
			if !known {
				return false
			}
			if region, ok := st.(*statements.TryCatchStatement); ok {
				for _, exception := range region.Exception {
					if !value(exception, 0) {
						return false
					}
				}
			}
			for _, root := range roots {
				if !value(root, 0) {
					return false
				}
			}
			for _, child := range children {
				if !walk(child, depth+1) {
					return false
				}
			}
			delete(activeStatements, st)
		}
		return true
	}
	for _, parameter := range params {
		if !value(parameter, 0) {
			return nil, false
		}
	}
	if !walk(body, 0) {
		return nil, false
	}
	out := make([]*values.JavaRef, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	return out, true
}
