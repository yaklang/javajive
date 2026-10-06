package javaclassparser

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type nativeMethodLocalClass struct {
	object       *ClassObject
	owner        *nativeMethodLocalOwner
	constructor  *nativeMethodLocalConstructor
	allocations  map[int]nativeMethodLocalAllocation
	calls        map[int]bool
	bindings     map[string]string
	parameterIDs map[int]*coreutils.VariableId
	source       string
}

// A parameter-only local declaration can be placed at the start of its exact
// declaring method: parameters dominate that block, and their complete source
// def-use proof forbids later writes. A future producer capture requires its
// actual declaration/dominance; it cannot borrow this placement certificate.
func (c *ClassObjectDumper) planNativeMethodLocals(p *nativeMemberFamily) bool {
	p.methodLocals = map[string]*nativeMethodLocalClass{}
	if c.obj.GetClassName() != p.owner {
		return false
	}
	seenMethods := map[string]bool{}
	for _, attribute := range c.obj.Attributes {
		table, ok := attribute.(*InnerClassesAttribute)
		if !ok || table == nil {
			continue
		}
		for _, row := range table.Classes {
			if row == nil || !nativeProofWork(c.Work, 1) {
				return false
			}
			if row.OuterClassInfoIndex != 0 || row.InnerNameIndex == 0 {
				continue
			}
			binary, known := sourceBridgeClassName(c.obj, row.InnerClassInfoIndex)
			if !known {
				return false
			}
			raw, found := c.foldSiblingResolver(binary)
			if !found {
				continue
			}
			local, e := c.parseResolved(raw)
			if e != nil || local.GetClassName() != binary {
				return false
			}
			owner, known := originalMethodLocalOwner(local, c.obj, c.Work)
			if !known {
				continue
			}
			if owner.method == "<init>" || !nativeMemberVersionMetadata(local, c.Work) || local.AccessFlags&0x10 != owner.flags&0x10 || owner.flags & ^uint16(0x10) != 0 || len(p.methodLocals) >= 64 || p.methodLocals[binary] != nil {
				return false
			}
			key := owner.method + owner.descriptor + "\x00" + owner.name
			if seenMethods[key] {
				return false
			}
			seenMethods[key] = true
			if _, known := nativeMethodLocalScopeSignature(local, c.obj, owner, c.Work); !known {
				return false
			}
			constructor, known := originalMethodLocalDefaultConstructor(local, c.obj, c.Work)
			if !known {
				return false
			}
			captureParams, _, captureErr := callbinding.Descriptor(constructor.descriptor)
			if captureErr != nil {
				return false
			}
			for _, method := range local.Methods {
				name, _ := sourceBridgeUTF8(local, method.NameIndex)
				if name == "<init>" && !nativeMethodLocalConstructorParameters(local, method, captureParams, owner, true, c.Work) {
					return false
				}
			}
			sites, known := c.nativeMethodLocalParameterAllocations(local, owner, constructor)
			if !known {
				return false
			}
			if constructor.enclosingField != "" && constructor.enclosingField != "this$0" {
				return false
			}
			for field := range constructor.captures {
				if field == constructor.enclosingField {
					continue
				}
				name, known := strings.CutPrefix(field, "val$")
				if !known || name == "" || class_context.SafeIdentifier(name) != name {
					return false
				}
			}
			// Methods and fields may not invent child scopes, static initializer
			// protocols or declaration annotations unsupported by this local profile.
			for _, m := range local.Methods {
				if m == nil || m.AccessFlags&8 != 0 {
					return false
				}
			}
			for _, a := range local.Attributes {
				if raw, ok := a.(*UnparsedAttribute); ok && raw != nil && strings.Contains(raw.Name, "Annotation") {
					return false
				}
				if table, ok := a.(*InnerClassesAttribute); ok {
					if table == nil {
						return false
					}
					for _, row := range table.Classes {
						if row == nil || !nativeProofWork(c.Work, 1) {
							return false
						}
						outer, known := sourceBridgeClassName(local, row.OuterClassInfoIndex)
						if known && outer == binary {
							return false
						}
						childName, known := sourceBridgeClassName(local, row.InnerClassInfoIndex)
						if !known {
							return false
						}
						if childName != binary && row.OuterClassInfoIndex == 0 {
							raw, known := c.foldSiblingResolver(childName)
							if known {
								child, e := c.parseResolved(raw)
								if e != nil {
									return false
								}
								if enclosing, _, anon := originalAnonymousOwner(child); anon && enclosing == binary {
									return false
								}
							}
						}
					}
				}
				switch a.(type) {
				case *RuntimeVisibleAnnotationsAttribute, *RuntimeVisibleTypeAnnotationsAttribute, *DeprecatedAttribute, *SyntheticAttribute:
					return false
				}
			}
			if c.Work != nil && c.Work.CheckAlloc(int64(len(p.methodLocals)+1)*1024) != nil {
				return false
			}
			p.methodLocals[binary] = &nativeMethodLocalClass{object: local, owner: owner, constructor: constructor, allocations: sites, calls: map[int]bool{}}
			if !nativeMethodLocalCaptureMetadata(c.obj, p.methodLocals[binary], c.Work) {
				return false
			}
			p.lexicalObjects[binary] = local
		}
	}
	// javac numbers each named local spelling in source declaration order,
	// independently of anonymous/constructor-marker ordinals. Validate original
	// method order, not a global counter or a binary-name ownership heuristic.
	ordinals := map[string]int{}
	for _, method := range c.obj.Methods {
		name, named := sourceBridgeUTF8(c.obj, method.NameIndex)
		desc, typed := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
		if !named || !typed {
			return false
		}
		for _, local := range p.methodLocals {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			if local.owner.method == name && local.owner.descriptor == desc {
				ordinals[local.owner.name]++
				if ordinals[local.owner.name] != local.owner.ordinal {
					return false
				}
			}
		}
	}
	return true
}

// Decode each original declaring method once for all local declarations. The
// transaction checks all users and physical sites before projecting any type.
func (z *JarFS) nativeMethodLocalArchiveClosed(p *nativeMemberFamily, index *nativeMemberIndex) bool {
	if p == nil || index == nil || !index.valid {
		return false
	}
	if len(p.methodLocals) == 0 {
		return true
	}
	root := p.lexicalObjects[p.owner]
	if root == nil || len(p.methodLocals) > 64 {
		return false
	}
	work := z.nativeMemberReader(root).Work
	localType := func(name string) bool {
		if p.methodLocals[name] != nil {
			return true
		}
		for binary := range p.methodLocals {
			if strings.Contains(name, "L"+binary+";") {
				return true
			}
		}
		return false
	}
	for binary, local := range p.methodLocals {
		if !nativeProofWork(work, 1) || local == nil || local.owner.owner != p.owner || p.lexicalObjects[binary] != local.object || index.handles[binary] {
			return false
		}
		for user := range index.typeUsers[binary] {
			if !nativeProofWork(work, 1) || user != p.owner && user != binary {
				return false
			}
		}
		for user := range index.constructors[binary] {
			if !nativeProofWork(work, 1) || user != p.owner {
				return false
			}
		}
		for field := range local.constructor.captures {
			for user := range index.captureUsers[nativeMemberCaptureIndexKey(binary, field)] {
				if !nativeProofWork(work, 1) || user != binary {
					return false
				}
			}
		}
	}
	// A local declaration cannot be named by a class field or a method header.
	for _, field := range root.Fields {
		if field == nil || !nativeProofWork(work, 1) {
			return false
		}
		desc, known := sourceBridgeUTF8(root, field.DescriptorIndex)
		if !known || localType(desc) {
			return false
		}
	}
	for _, method := range root.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, named := sourceBridgeUTF8(root, method.NameIndex)
		desc, typed := sourceBridgeUTF8(root, method.DescriptorIndex)
		if !named || !typed || localType(desc) {
			return false
		}
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			if code == nil || !nativeProofWork(work, int64(len(code.Code))) || work != nil && work.CheckAlloc(int64(len(code.Code)+1)*64) != nil {
				return false
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(root.ConstantPool, i) })
			decoder.Work = work
			if decoder.ParseOpcode() != nil {
				return false
			}
			for _, op := range decoder.Opcodes() {
				if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
					return false
				}
				switch op.Instr.OpCode {
				case core.OP_NEW, core.OP_CHECKCAST, core.OP_INSTANCEOF, core.OP_ANEWARRAY, core.OP_MULTIANEWARRAY:
					if len(op.Data) < 2 {
						return false
					}
					target, known := sourceBridgeClassName(root, core.Convert2bytesToInt(op.Data[:2]))
					if !known {
						return false
					}
					if localType(target) {
						local := p.methodLocals[target]
						if local == nil || op.Instr.OpCode != core.OP_NEW || name != local.owner.method || desc != local.owner.descriptor {
							return false
						}
						if _, known := local.allocations[int(op.CurrentOffset)]; !known {
							return false
						}
					}
				case core.OP_LDC, core.OP_LDC_W:
					at := 0
					if len(op.Data) == 1 {
						at = int(op.Data[0])
					} else if len(op.Data) == 2 {
						at = int(core.Convert2bytesToInt(op.Data))
					}
					if at < 1 || at > len(root.ConstantPool) {
						return false
					}
					if constant, ok := root.ConstantPool[at-1].(*ConstantClassInfo); ok {
						if constant == nil {
							return false
						}
						target, known := sourceBridgeUTF8(root, constant.NameIndex)
						if !known || localType(target) {
							return false
						}
					}
				}
				switch op.Instr.OpCode {
				case core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC, core.OP_INVOKEVIRTUAL, core.OP_INVOKEINTERFACE, core.OP_GETFIELD, core.OP_PUTFIELD, core.OP_GETSTATIC, core.OP_PUTSTATIC:
					member := constructorMotionMember(root, op, op.Instr.OpCode)
					if member == nil {
						return false
					}
					if localType(member.Description) {
						return false
					}
					if local := p.methodLocals[member.Name]; local != nil {
						if name != local.owner.method || desc != local.owner.descriptor {
							return false
						}
						if member.Member == "<init>" {
							matched := false
							for _, site := range local.allocations {
								if !nativeProofWork(work, 1) {
									return false
								}
								if site.invokePC == int(op.CurrentOffset) {
									matched = true
								}
							}
							if op.Instr.OpCode != core.OP_INVOKESPECIAL || member.Description != local.constructor.descriptor || !matched {
								return false
							}
						}
					}
				}
			}
		}
	}
	return true
}

func (c *ClassObjectDumper) wireNativeMethodLocalSource() {
	p := c.nativeMemberRoot
	ctx := c.FuncCtx
	if p == nil || len(p.methodLocals) == 0 {
		return
	}
	if c.nativeMethodLocalCurrent != nil {
		ctx.LexicalClassName = c.nativeMethodLocalCurrent.owner.name
	}
	prior := ctx.DeclarationSourceName
	ctx.DeclarationSourceName = func(binary string) (string, bool) {
		local := p.methodLocals[strings.ReplaceAll(binary, ".", "/")]
		if local != nil && (c.nativeMethodLocalCurrent == local || c.obj.GetClassName() == p.owner && ctx.FunctionName == local.owner.method && ctx.CurrentMethodDesc == local.owner.descriptor) {
			return local.owner.name, true
		}
		if prior != nil {
			return prior(binary)
		}
		return "", false
	}
	ctx.SourceMethodLocalCandidate = func(binary string) bool {
		local := p.methodLocals[strings.ReplaceAll(binary, ".", "/")]
		return c.nativeSourceNamesReady && c.obj.GetClassName() == p.owner && local != nil && local.source != "" && ctx.FunctionName == local.owner.method && ctx.CurrentMethodDesc == local.owner.descriptor
	}
	ctx.SourceMethodLocalAllocation = func(binary, descriptor string, newPC, invokePC int, args []class_context.SourceCaptureOperand) (string, bool) {
		local := p.methodLocals[strings.ReplaceAll(binary, ".", "/")]
		if local == nil {
			return "", false
		}
		fail := func() (string, bool) { p.failed = true; return "", false }
		site, known := local.allocations[newPC]
		descriptors, _, err := callbinding.Descriptor(descriptor)
		if !known || descriptor != local.constructor.descriptor || invokePC != site.invokePC || len(site.slots) != len(args) || err != nil || len(descriptors) != len(args) {
			return fail()
		}
		for i, arg := range args {
			v, known := arg.Value.(values.JavaValue)
			if !known || !nativeMethodLocalParameterOperand(v, site.slots[i], descriptors[i], ctx, c.Work) {
				return fail()
			}
			ref, known := values.UnpackSoltValue(v).(*values.JavaRef)
			if !known || ref == nil || local.parameterIDs[site.slots[i]] != ref.Id {
				return fail()
			}
			field := ""
			for name, index := range local.constructor.captures {
				if index == i {
					field = name
				}
			}
			if field == "" || field != local.constructor.enclosingField && arg.Text != local.bindings[field] {
				return fail()
			}
		}
		local.calls[newPC] = true
		return "new " + local.owner.name + "()", true
	}
}

func (c *ClassObjectDumper) prepareNativeMethodLocalDeclarations(body []statements.Statement, params []values.JavaValue) ([]string, error) {
	p := c.nativeMemberRoot
	ctx := c.FuncCtx
	if p == nil || c.obj.GetClassName() != p.owner {
		return nil, nil
	}
	locals := []*nativeMethodLocalClass{}
	for _, local := range p.methodLocals {
		if local.owner.method == ctx.FunctionName && local.owner.descriptor == ctx.CurrentMethodDesc {
			locals = append(locals, local)
		}
	}
	if len(locals) == 0 {
		return nil, nil
	}
	sort.Slice(locals, func(i, j int) bool {
		first := func(l *nativeMethodLocalClass) int {
			pc := 65536
			for p := range l.allocations {
				if p < pc {
					pc = p
				}
			}
			return pc
		}
		return first(locals[i]) < first(locals[j])
	})
	bySlot := map[int]*values.JavaRef{}
	paramIDs := map[*coreutils.VariableId]bool{}
	for _, v := range params {
		ref, ok := v.(*values.JavaRef)
		if !ok || ref == nil {
			continue
		}
		paramIDs[ref.Id] = true
		if ref.IsThis && !ctx.IsStatic {
			bySlot[0] = ref
		} else if slot, known := ref.OriginalParameterSlot(); known {
			bySlot[slot] = ref
		}
	}
	if ctx.LocalNames == nil {
		ctx.LocalNames = map[*coreutils.VariableId]string{}
	}
	reserved := map[string]bool{}
	protected := map[*coreutils.VariableId]bool{}
	assigned := map[string]*coreutils.VariableId{}
	fail := func(reason string) ([]string, error) {
		p.failed = true
		return nil, fmt.Errorf("unproved method-local declaration/capture placement: %s", reason)
	}
	for _, local := range locals {
		local.bindings = map[string]string{}
		local.parameterIDs = map[int]*coreutils.VariableId{}
		var site nativeMethodLocalAllocation
		for _, allocation := range local.allocations {
			site = allocation
			break
		}
		for field, index := range local.constructor.captures {
			ref := bySlot[site.slots[index]]
			if ref == nil || ref.Id == nil {
				return fail("actual descriptor-seeded parameter missing")
			}
			local.parameterIDs[site.slots[index]] = ref.Id
			if field == local.constructor.enclosingField {
				if !ref.IsThis {
					return fail("actual enclosing THIS missing")
				}
				local.bindings[field] = ctx.ShortTypeName(ctx.ClassName) + ".this"
				continue
			}
			_, stable := nativeCaptureDeclaration(body, ref, paramIDs[ref.Id], c.Work)
			if !stable {
				return fail("captured parameter is written or has an ambiguous declaration")
			}
			name, known := strings.CutPrefix(field, "val$")
			if !known || assigned[name] != nil && assigned[name] != ref.Id {
				return fail("capture source name collision")
			}
			if old, known := ctx.LocalNames[ref.Id]; known && old != name {
				return fail("previous lexical name differs")
			}
			ctx.LocalNames[ref.Id] = name
			local.bindings[field] = name
			reserved[name] = true
			protected[ref.Id] = true
			assigned[name] = ref.Id
		}
	}
	c.prepareNativeSourceNames(body, params, reserved, protected)
	if c.nativeCaptureFailed {
		return fail("namespace proof failed")
	}
	declarations := []string{}
	for _, local := range locals {
		sub := NewClassObjectDumper(local.object)
		sub.options = c.options
		sub.Work = c.Work
		sub.foldSiblingResolver = c.foldSiblingResolver
		sub.declarationResolver = c.declarationResolver
		sub.nativeMemberLookup = c.nativeMemberLookup
		sub.nativeMemberRoot = p
		sub.nativeMethodLocalCurrent = local
		sub.nativeCaptureFields = local.bindings
		sub.nativeCaptureTypes = map[string]types.JavaType{}
		lexical := *ctx
		methodSignature, scoped := nativeMethodLocalScopeSignature(local.object, c.obj, local.owner, c.Work)
		if !scoped {
			return fail("original lexical generic scope failed")
		}
		lexical.CurrentMethodSig = methodSignature
		if ctx.IsStatic {
			lexical.ClassSig = ""
			lexical.ClassTypeParams = nil
			lexical.LexicalTypeParamSignatures = nil
		}
		sub.nativeOuterContext = &lexical
		sub.nativeTypeParams = append([]string(nil), ctx.TypeParams...)
		sub.sourceInnerClassBody = true
		var site nativeMethodLocalAllocation
		for _, allocation := range local.allocations {
			site = allocation
			break
		}
		for field, index := range local.constructor.captures {
			if field != local.constructor.enclosingField {
				sub.nativeCaptureTypes[field] = bySlot[site.slots[index]].Type().Copy()
			}
		}
		sub.nativeCapturedReads = map[string]map[int]string{}
		for _, m := range local.object.Methods {
			n, _ := sourceBridgeUTF8(local.object, m.NameIndex)
			desc, _ := sourceBridgeUTF8(local.object, m.DescriptorIndex)
			key := n + desc
			sub.nativeCapturedReads[key] = map[int]string{}
			for _, a := range m.Attributes {
				code, ok := a.(*CodeAttribute)
				if !ok {
					continue
				}
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(local.object.ConstantPool, i) })
				d.Work = c.Work
				if d.ParseOpcode() != nil {
					return fail("original captured-read decoder failed")
				}
				for _, op := range d.Opcodes() {
					field := constructorMotionMember(local.object, op, core.OP_GETFIELD)
					if field != nil && field.Name == local.object.GetClassName() {
						if _, captured := local.bindings[field.Member]; captured {
							sub.nativeCapturedReads[key][int(op.CurrentOffset)] = field.Member
						}
					}
				}
			}
		}
		source, e := sub.DumpClass()
		if e != nil || sub.nativeCaptureFailed || p.failed || len(sub.constructorBoundaryHelpers) != 0 || strings.Contains(source, DecompileStubMarker) {
			return fail("rendered body or joint scope failed")
		}
		for _, method := range sub.dumpedMethodsSet {
			if method != nil && method.checkedEscape {
				return fail("checked escape helper requires separate lexical proof")
			}
		}
		// This is an actual named-local declaration, not an anonymous expression.
		// Strip only compilation-unit package/import directives; its proved header,
		// method bodies and type scope remain untouched.
		content, known := javaClassBodyContentKnown(source)
		if !known {
			return fail("unbalanced local body")
		}
		header := nativeMethodLocalDeclarationHeader(local, source)
		if header == "" {
			return fail("local header unavailable")
		}
		if !nativeMethodLocalCaptureSourceComplete(local, source, c.Work) {
			return fail("a captured field has no emitted original lexical use")
		}
		local.source = header + " {\n" + content + "\n}"
		if e := c.ensureOutput(int64(len(local.source))); e != nil {
			return nil, e
		}
		declarations = append(declarations, local.source)
		for _, imp := range javaExtractImports(source) {
			ctx.Import(imp)
		}
	}
	return declarations, nil
}

// Preserve the complete header rendered from original generic declarations.
// This bounded extraction discards compilation-unit directives only; it never
// rebuilds a raw parent/interface and loses their original type arguments.
func nativeMethodLocalDeclarationHeader(local *nativeMethodLocalClass, source string) string {
	open := javaIndexTopBrace(source)
	if local == nil || open < 0 {
		return ""
	}
	prefix := source[:open]
	line := strings.LastIndexByte(prefix, '\n') + 1
	header := strings.TrimSpace(prefix[line:])
	expected := "class " + local.owner.name
	if local.owner.flags&0x10 != 0 {
		expected = "final " + expected
	}
	if !strings.HasPrefix(header, expected) {
		return ""
	}
	suffix := header[len(expected):]
	if suffix != "" && suffix[0] != ' ' && suffix[0] != '<' {
		return ""
	}
	return header
}

func nativeMethodLocalSourceComplete(p *nativeMemberFamily) bool {
	if p == nil || p.failed {
		return false
	}
	for _, local := range p.methodLocals {
		if local == nil || local.source == "" || len(local.calls) != len(local.allocations) {
			return false
		}
		for pc := range local.allocations {
			if !local.calls[pc] {
				return false
			}
		}
	}
	return true
}

// A static declaring method cannot inherit class formals; a method formal
// shadows an equal-spelled class formal. Bind the original Signature before
// checking the local class's own declarations, never a renderer hint.
func nativeMethodLocalScopeSignature(local, enclosing *ClassObject, owner *nativeMethodLocalOwner, work *workbudget.Budget) (string, bool) {
	signature := func(obj *ClassObject, attributes []AttributeInfo) (string, bool) {
		result := ""
		seen := false
		for _, a := range attributes {
			if !nativeProofWork(work, 1) {
				return "", false
			}
			if sig, ok := a.(*SignatureAttribute); ok {
				if seen || sig == nil {
					return "", false
				}
				seen = true
				var known bool
				result, known = sourceBridgeUTF8(obj, sig.SignatureIndex)
				if !known || !nativeProofWork(work, int64(len(result))) {
					return "", false
				}
			}
		}
		return result, true
	}
	classSig, known := signature(enclosing, enclosing.Attributes)
	if !known {
		return "", false
	}
	methodSig, known := signature(enclosing, owner.declaration.Attributes)
	if !known {
		return "", false
	}
	scope := map[string]bool{}
	if owner.declaration.AccessFlags&8 == 0 {
		for _, name := range types.ClassFormalTypeParamNames(classSig) {
			scope[name] = true
		}
	}
	if methodSig != "" {
		own, refs, known := types.SignatureTypeVariableReferences(methodSig)
		if !known {
			return "", false
		}
		for _, name := range own {
			scope[name] = true
		}
		for _, name := range refs {
			if !scope[name] {
				return "", false
			}
		}
	}
	return methodSig, nativeMemberTypeScope(local, scope, work)
}

// Every ordinary hidden capture must survive as a real lexical use in emitted
// source, otherwise javac would omit its field/constructor parameter. An
// original dead capture cannot be recreated by inventing an unreachable read.
// The mandated enclosing capture is independent of use in this Java-8 profile.
func nativeMethodLocalCaptureSourceComplete(local *nativeMethodLocalClass, source string, work *workbudget.Budget) bool {
	if local == nil || local.constructor == nil {
		return false
	}
	used := map[string]bool{}
	prefix := "jdec-owned-local-capture:" + local.object.GetClassName() + ":"
	for i := 0; i < len(source); {
		if !nativeProofWork(work, 1) {
			return false
		}
		ch := source[i]
		if ch == '\'' || ch == '"' {
			quote := ch
			i++
			closed := false
			for i < len(source) {
				if !nativeProofWork(work, 1) {
					return false
				}
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
		if strings.HasPrefix(source[i:], "//") {
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				break
			}
			if !nativeProofWork(work, int64(end)) {
				return false
			}
			i += end + 1
			continue
		}
		if strings.HasPrefix(source[i:], "/*") {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 || !nativeProofWork(work, int64(end)) {
				return false
			}
			comment := source[i+2 : i+2+end]
			if field, known := strings.CutPrefix(comment, prefix); known {
				if _, captured := local.constructor.captures[field]; !captured {
					return false
				}
				if work != nil && work.CheckAlloc(int64(len(used)+1)*64) != nil {
					return false
				}
				used[field] = true
			}
			i += end + 4
			continue
		}
		i++
	}
	for field := range local.constructor.captures {
		if !nativeProofWork(work, 1) || field != local.constructor.enclosingField && !used[field] {
			return false
		}
	}
	return true
}

// javac's hidden captures carry erased descriptors and no field Signature,
// even when the lexical parameter is a method formal or parameterized type.
// Preserve that physical ABI; an extra original attribute cannot be silently
// discarded merely because an arithmetic round trip happens to pass.
func nativeMethodLocalCaptureMetadata(enclosing *ClassObject, local *nativeMethodLocalClass, work *workbudget.Budget) bool {
	if enclosing == nil || local == nil || local.owner == nil || local.constructor == nil {
		return false
	}
	methodSig, scoped := nativeMethodLocalScopeSignature(local.object, enclosing, local.owner, work)
	if !scoped {
		return false
	}
	_, _, err := callbinding.Descriptor(local.owner.descriptor)
	if err != nil {
		return false
	}
	if methodSig != "" {
		classSig := ""
		for _, a := range enclosing.Attributes {
			if signature, ok := a.(*SignatureAttribute); ok && local.owner.declaration.AccessFlags&8 == 0 {
				if signature == nil {
					return false
				}
				var known bool
				classSig, known = sourceBridgeUTF8(enclosing, signature.SignatureIndex)
				if !known {
					return false
				}
			}
		}
		erased, _, known := types.EraseLexicalMethodSignatureWithThrows(classSig, methodSig)
		if !known || erased != local.owner.descriptor {
			return false
		}
	}
	for _, field := range local.object.Fields {
		if field == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, named := sourceBridgeUTF8(local.object, field.NameIndex)
		descriptor, typed := sourceBridgeUTF8(local.object, field.DescriptorIndex)
		_, found := local.constructor.captures[name]
		if !named || !typed || descriptor == "" || !found || len(field.Attributes) != 0 {
			return false
		}
	}
	return true
}
