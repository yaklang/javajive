package javaclassparser

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func init() {
	TryRecordLayout = tryRecordLayout
	RecordReconstructionComplete = recordReconstructionComplete
	AfterMembersDumped = afterRecordMembersDumped
}

func afterRecordMembersDumped(c *ClassObjectDumper) {
	if c == nil || c.obj == nil {
		return
	}
	stashDumper(c)
	if v, ok := recordWorkByObj.Load(c.obj); ok {
		w := v.(*recordWork)
		if w.layout != nil {
			applyRecordSkips(c, w.layout, w.components)
		}
	}
	applyPatternSwitchRewrites(c)
}

var dumpersByObj sync.Map // *ClassObject -> *ClassObjectDumper

type recordWork struct {
	complete   bool
	layout     *RecordLayout
	components []recordComponent
}

var recordWorkByObj sync.Map // *ClassObject -> *recordWork

type recordComponent struct {
	Name       string
	Descriptor string
	Signature  string
	Annos      []*AnnotationAttribute
}

func stashDumper(c *ClassObjectDumper) {
	if c != nil && c.obj != nil {
		dumpersByObj.Store(c.obj, c)
	}
}

func dumperFor(obj *ClassObject) *ClassObjectDumper {
	if obj == nil {
		return nil
	}
	if v, ok := dumpersByObj.Load(obj); ok {
		return v.(*ClassObjectDumper)
	}
	return nil
}

func tryRecordLayout(c *ClassObjectDumper) *RecordLayout {
	stashDumper(c)
	applyPatternSwitchRewrites(c)
	if c == nil || c.obj == nil {
		return nil
	}
	target := c.options.TargetSourceVersion
	if target == 0 {
		target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
	}
	if target < 16 {
		recordWorkByObj.Store(c.obj, &recordWork{complete: false})
		return nil
	}
	comps, ok := parseRecordComponents(c.obj)
	if !ok {
		recordWorkByObj.Store(c.obj, &recordWork{complete: false})
		return nil
	}
	layout := buildRecordLayout(c, comps)
	if layout == nil {
		recordWorkByObj.Store(c.obj, &recordWork{complete: false})
		return nil
	}
	applyRecordSkips(c, layout, comps)
	recordWorkByObj.Store(c.obj, &recordWork{complete: true, layout: layout, components: comps})
	return layout
}

func recordReconstructionComplete(obj *ClassObject) bool {
	if obj == nil {
		return false
	}
	v, ok := recordWorkByObj.Load(obj)
	if !ok {
		return false
	}
	w := v.(*recordWork)
	if d := dumperFor(obj); d != nil && d.report != nil && d.report.Source != "" && w.layout != nil {
		d.report.Source = sanitizeRecordSource(d.report.Source, w)
	}
	return w.complete
}

func parseRecordComponents(obj *ClassObject) ([]recordComponent, bool) {
	info := recordAttributeBytes(obj)
	if info == nil {
		return nil, false
	}
	defer func() { _ = recover() }()
	p := &ClassParser{reader: NewClassReader(append([]byte(nil), info...)), classObj: obj}
	n := int(p.reader.readUint16())
	out := make([]recordComponent, 0, n)
	for i := 0; i < n; i++ {
		nameIdx := p.reader.readUint16()
		descIdx := p.reader.readUint16()
		name, err := obj.getUtf8(nameIdx)
		if err != nil {
			return nil, false
		}
		desc, err := obj.getUtf8(descIdx)
		if err != nil {
			return nil, false
		}
		rc := recordComponent{Name: name, Descriptor: desc}
		attrs := p.readAttributes()
		for _, a := range attrs {
			switch t := a.(type) {
			case *SignatureAttribute:
				if s, err := obj.getUtf8(t.SignatureIndex); err == nil {
					rc.Signature = s
				}
			case *RuntimeVisibleAnnotationsAttribute:
				if t != nil && !t.IsInvisible {
					rc.Annos = append(rc.Annos, t.Annotations...)
				}
			}
		}
		out = append(out, rc)
	}
	return out, true
}

func recordAttributeBytes(obj *ClassObject) []byte {
	if obj == nil {
		return nil
	}
	for _, a := range obj.Attributes {
		if u, ok := a.(*UnparsedAttribute); ok && u != nil && u.Name == "Record" {
			return u.Info
		}
	}
	return nil
}

func buildRecordLayout(c *ClassObjectDumper, comps []recordComponent) *RecordLayout {
	typeParams := ""
	if c.FuncCtx != nil {
		for _, attr := range c.obj.Attributes {
			if sig, ok := attr.(*SignatureAttribute); ok {
				if s, err := c.obj.getUtf8(sig.SignatureIndex); err == nil && s != "" {
					typeParams = types.ParseClassSignature(s, c.FuncCtx)
				}
				break
			}
		}
	}
	parts := make([]string, 0, len(comps))
	skipFields := map[string]bool{}
	skipMethods := map[string]bool{}
	var descBuf strings.Builder
	descBuf.WriteByte('(')
	for _, rc := range comps {
		skipFields[rc.Name] = true
		skipMethods[rc.Name+"()"+rc.Descriptor] = true
		annos := ""
		for _, an := range rc.Annos {
			if c != nil {
				if s, err := c.DumpAnnotation(an); err == nil && s != "" {
					annos += s + " "
				}
			}
		}
		parts = append(parts, strings.TrimSpace(annos+renderComponentType(c, rc)+" "+rc.Name))
		descBuf.WriteString(rc.Descriptor)
	}
	descBuf.WriteString(")V")
	canonicalDesc := descBuf.String()
	skipMethods["<init>"+canonicalDesc] = true

	for _, m := range c.obj.Methods {
		name, err1 := c.obj.getUtf8(m.NameIndex)
		desc, err2 := c.obj.getUtf8(m.DescriptorIndex)
		if err1 != nil || err2 != nil {
			continue
		}
		if isDefaultRecordAccessor(c.obj, m, name, desc, comps) {
			skipMethods[name+desc] = true
		}
		if isSyntheticObjectMethods(c.obj, m, name, desc) {
			skipMethods[name+desc] = true
		}
		if name == "<init>" && desc == canonicalDesc && !isDefaultCanonicalCtor(c.obj, m, comps) {
			delete(skipMethods, "<init>"+canonicalDesc)
		}
	}

	return &RecordLayout{
		Keyword:           "record",
		Components:        typeParams + "(" + strings.Join(parts, ", ") + ")",
		DropExtendsRecord: true,
		SkipFieldNames:    skipFields,
		SkipMethodKeys:    skipMethods,
	}
}

func renderComponentType(c *ClassObjectDumper, rc recordComponent) string {
	ctx := &class_context.ClassContext{}
	if c != nil && c.FuncCtx != nil {
		ctx = c.FuncCtx
	}
	if rc.Signature != "" {
		if t := types.ParseSignature(rc.Signature); t != nil {
			return t.String(ctx)
		}
	}
	t, err := types.ParseDescriptor(rc.Descriptor)
	if err != nil || t == nil {
		return rc.Descriptor
	}
	return t.String(ctx)
}

func applyRecordSkips(c *ClassObjectDumper, layout *RecordLayout, comps []recordComponent) {
	if c == nil || layout == nil {
		return
	}
	if c.recordSkipFields == nil {
		c.recordSkipFields = map[string]bool{}
	}
	if c.recordSkipMethods == nil {
		c.recordSkipMethods = map[string]bool{}
	}
	for k, v := range layout.SkipFieldNames {
		c.recordSkipFields[k] = v
	}
	for k, v := range layout.SkipMethodKeys {
		c.recordSkipMethods[k] = v
	}
	canonical := canonicalCtorDesc(comps)
	for _, m := range c.obj.Methods {
		name, err1 := c.obj.getUtf8(m.NameIndex)
		desc, err2 := c.obj.getUtf8(m.DescriptorIndex)
		if err1 != nil || err2 != nil {
			continue
		}
		key := fmt.Sprintf("name:%s,desc:%s", name, desc)
		dumped, ok := c.dumpedMethodsSet[key]
		if !ok || dumped == nil {
			continue
		}
		if name == "<init>" && desc == canonical && !isDefaultCanonicalCtor(c.obj, m, comps) {
			dumped.code = rewriteCompactCtor(dumped.code, comps)
			dumped.bodyCode = dumped.code
			continue
		}
		if layout.SkipMethodKeys[name+desc] {
			dumped.code = ""
			dumped.bodyCode = ""
		}
	}
}

func canonicalCtorDesc(comps []recordComponent) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, c := range comps {
		b.WriteString(c.Descriptor)
	}
	b.WriteString(")V")
	return b.String()
}

func isDefaultRecordAccessor(obj *ClassObject, m *MemberInfo, name, desc string, comps []recordComponent) bool {
	var wantDesc string
	found := false
	for _, c := range comps {
		if c.Name == name {
			wantDesc = "()" + c.Descriptor
			found = true
			break
		}
	}
	if !found || desc != wantDesc {
		return false
	}
	code := methodCodeBytes(m)
	if len(code) < 5 || code[0] != 0x2a || code[1] != 0xb4 { // aload_0; getfield
		return false
	}
	switch code[len(code)-1] {
	case 0xac, 0xad, 0xae, 0xaf, 0xb0, 0xb1: // *return
		return len(code) <= 6
	}
	return false
}

func isSyntheticObjectMethods(obj *ClassObject, m *MemberInfo, name, desc string) bool {
	switch name {
	case "toString", "equals", "hashCode":
	default:
		return false
	}
	code := methodCodeBytes(m)
	if len(code) == 0 {
		return false
	}
	d := core.NewDecompiler(code, func(id int) values.JavaValue {
		return GetValueFromCP(obj.ConstantPool, id)
	})
	d.ConstantPoolInvokeDynamicInfo = func(index int) (uint16, string, string) {
		constant := obj.ConstantPool[index-1]
		switch ret := constant.(type) {
		case *ConstantInvokeDynamicInfo:
			n, ds := getNameAndType(obj.ConstantPool, ret.NameAndTypeIndex)
			return ret.BootstrapMethodAttrIndex, n, ds
		default:
			return 0, "", ""
		}
	}
	attachBootstrap(d, obj)
	if err := d.ParseOpcode(); err != nil {
		return false
	}
	for _, op := range d.Opcodes() {
		if op == nil || op.Instr == nil || op.Instr.OpCode != core.OP_INVOKEDYNAMIC {
			continue
		}
		if len(op.Data) < 2 {
			continue
		}
		index, csName, _ := d.ConstantPoolInvokeDynamicInfo(int(core.Convert2bytesToInt(op.Data)))
		if csName != name {
			continue
		}
		if int(index) >= 0 && int(index) < len(d.BootstrapMethods) && d.BootstrapMethods[index] != nil {
			if mem, ok := d.BootstrapMethods[index].Ref.(*values.JavaClassMember); ok && mem != nil {
				id, err := core.IdentityFromMember(mem)
				if err == nil && id.Equal(core.IdentityObjectMethods) {
					return true
				}
			}
		}
	}
	return false
}

func isDefaultCanonicalCtor(obj *ClassObject, m *MemberInfo, comps []recordComponent) bool {
	code := methodCodeBytes(m)
	if len(code) == 0 {
		return false
	}
	d := core.NewDecompiler(code, func(id int) values.JavaValue {
		return GetValueFromCP(obj.ConstantPool, id)
	})
	if err := d.ParseOpcode(); err != nil {
		return false
	}
	ops := d.Opcodes()
	putfields := 0
	other := 0
	for _, op := range ops {
		if op == nil || op.Instr == nil || op.Instr.OpCode == core.OP_START {
			continue
		}
		switch op.Instr.OpCode {
		case core.OP_ALOAD, core.OP_ALOAD_0, core.OP_ALOAD_1, core.OP_ALOAD_2, core.OP_ALOAD_3,
			core.OP_ILOAD, core.OP_ILOAD_0, core.OP_ILOAD_1, core.OP_ILOAD_2, core.OP_ILOAD_3,
			core.OP_LLOAD, core.OP_LLOAD_0, core.OP_LLOAD_1, core.OP_LLOAD_2, core.OP_LLOAD_3,
			core.OP_FLOAD, core.OP_FLOAD_0, core.OP_FLOAD_1, core.OP_FLOAD_2, core.OP_FLOAD_3,
			core.OP_DLOAD, core.OP_DLOAD_0, core.OP_DLOAD_1, core.OP_DLOAD_2, core.OP_DLOAD_3,
			core.OP_INVOKESPECIAL, core.OP_RETURN, core.OP_NOP:
			continue
		case core.OP_PUTFIELD:
			putfields++
		default:
			other++
		}
	}
	return other == 0 && putfields == len(comps)
}

func rewriteCompactCtor(code string, comps []recordComponent) string {
	// public RecordCustom(int var1, String var2) { body } → public RecordCustom { body without this.x=... }
	open := strings.Index(code, "(")
	blk := strings.Index(code, "{")
	if open < 0 || blk < 0 || blk < open {
		return code
	}
	header := strings.TrimSpace(code[:open])
	body := code[blk:]
	paramsPart := code[open+1 : blk]
	if i := strings.LastIndex(paramsPart, ")"); i >= 0 {
		paramsPart = paramsPart[:i]
	}
	params := splitParams(paramsPart)
	for i, p := range params {
		if i >= len(comps) {
			break
		}
		name := paramIdent(p)
		if name == "" {
			continue
		}
		body = wordReplace(body, name, comps[i].Name)
	}
	for _, c := range comps {
		re := regexp.MustCompile(`(?m)^\s*this\.` + regexp.QuoteMeta(c.Name) + `\s*=\s*` + regexp.QuoteMeta(c.Name) + `\s*;\s*\n?`)
		body = re.ReplaceAllString(body, "")
	}
	reSuper := regexp.MustCompile(`(?m)^\s*super\s*\(\s*\)\s*;\s*\n?`)
	body = reSuper.ReplaceAllString(body, "")
	return header + " " + strings.TrimSpace(body)
}

func splitParams(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func paramIdent(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	fs := strings.Fields(p)
	if len(fs) == 0 {
		return ""
	}
	return fs[len(fs)-1]
}

func wordReplace(s, old, neu string) string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(old) + `\b`)
	return re.ReplaceAllString(s, neu)
}

func methodCodeBytes(m *MemberInfo) []byte {
	if m == nil {
		return nil
	}
	for _, a := range m.Attributes {
		if c, ok := a.(*CodeAttribute); ok && c != nil {
			return c.Code
		}
	}
	return nil
}

func attachBootstrap(d *core.Decompiler, obj *ClassObject) {
	var methods []*BootstrapMethod
	for _, a := range obj.Attributes {
		if b, ok := a.(*BootstrapMethodsAttribute); ok && b != nil {
			methods = b.BootstrapMethods
			break
		}
	}
	for _, method := range methods {
		val := GetValueFromCP(obj.ConstantPool, int(method.BootstrapMethodRef))
		arguments := make([]values.JavaValue, len(method.BootstrapArguments))
		for i, arg := range method.BootstrapArguments {
			arguments[i] = GetLiteralFromCP(obj.ConstantPool, int(arg))
		}
		d.BootstrapMethods = append(d.BootstrapMethods, &core.BootstrapMethod{Ref: val, Arguments: arguments})
	}
}

func sanitizeRecordSource(src string, w *recordWork) string {
	if w == nil || w.layout == nil {
		return src
	}
	for name := range w.layout.SkipFieldNames {
		re := regexp.MustCompile(`(?m)^[ \t]*(public |private |protected )?(static )?(final )?[\w.$<>,\[\] ]+ \b` + regexp.QuoteMeta(name) + `\s*;\s*\n`)
		src = re.ReplaceAllString(src, "")
	}
	src = strings.ReplaceAll(src, "\n\t// Fields\n", "\n")
	src = strings.ReplaceAll(src, "\n// Fields\n", "\n")
	if !strings.Contains(src, "record ") && strings.Contains(src, " class ") {
		src = strings.Replace(src, " class ", " record ", 1)
	}
	src = strings.ReplaceAll(src, " extends Record", "")
	src = strings.ReplaceAll(src, " extends java.lang.Record", "")
	return src
}
