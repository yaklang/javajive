package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"sort"
	"strings"
)

type nativeMemberConstructor struct {
	descriptor, sourceDescriptor, delegateOwner, delegateDescriptor string
	capturePC, delegatePC                                           int
	projectedSuper                                                  bool
	enclosingSuperPath                                              *nativeMemberLexicalRead
}
type nativeMemberClass struct {
	lambdaContext                 nativeLambdaImplementationContext
	lambdaImplementations         map[*MemberInfo]bool
	assertions                    *nativeMemberAssertion
	enumSynthesis                 *nativeMemberEnumSynthesis
	sourceName                    string
	object                        *ClassObject
	owner, name, field            string
	static                        bool
	formalCount, outerFormalCount int
	flags                         uint16
	constructors                  map[string]*nativeMemberConstructor
	accessBridges                 map[string]*nativeConstructorAccessBridge
}
type nativeMemberFamily struct {
	modernNestObjects      map[string]*ClassObject
	methodLocals           map[string]*nativeMethodLocalClass
	enumConstants          map[string]*nativeEnumConstantBody
	enumSwitchTables       map[string]*nativeEnumSwitchTable
	registrationLayouts    map[string]*nativeMemberRegistrationScope
	sourceDependencies     map[string]string
	allocationDependencies map[string]*nativeMemberClass
	rootAccessBridges      map[string]*nativeConstructorAccessBridge
	rootBridgeDelegations  map[string]*nativeRootBridgeDelegation
	getters                map[string]*nativeMemberPrivateGetter
	retainedAccessors      map[string]*nativeMemberPrivateGetter
	nestmateAccessors      bool
	lexicalObjects         map[string]*ClassObject
	anonymous              *nativeAnonymousFamily
	anonymousUnits         map[string]*nativeAnonymousFamily
	memberAnonymous        map[string]*nativeAnonymousFamily
	anonymousForest        *nativeAnonymousForest
	owner                  string
	children               map[string]*nativeMemberClass
	failed                 bool
	bridgeCalls            map[string]int
	emptyMarkers           map[string]*ClassObject
	// Newer compilers use nestmates and need not emit an unused access marker.
	// Keep its original class as a separate source unit instead of consuming it.
	retainEmptyMarkers bool
}

// Source ownership comes from one original self row, never dollar spelling.
func originalMemberOwner(obj *ClassObject) (owner, name string, flags uint16, valid bool) {
	if obj == nil {
		return
	}
	count := 0
	for _, a := range obj.Attributes {
		if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
			return "", "", 0, false
		}
		if inner, ok := a.(*InnerClassesAttribute); ok && inner != nil {
			for _, row := range inner.Classes {
				if row == nil {
					return "", "", 0, false
				}
				self, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
				if !known {
					return "", "", 0, false
				}
				if self != obj.GetClassName() {
					continue
				}
				count++
				var ok bool
				owner, ok = sourceBridgeClassName(obj, row.OuterClassInfoIndex)
				if !ok {
					return "", "", 0, false
				}
				name, ok = sourceBridgeUTF8(obj, row.InnerNameIndex)
				if !ok {
					return "", "", 0, false
				}
				flags = row.InnerClassAccessFlags
			}
		}
	}
	valid = count == 1 && owner != "" && name != "" && class_context.SafeIdentifier(name) == name && obj.GetClassName() == owner+"$"+name
	return
}

// A static member's type-variable scope starts at its own declaration. Read
// the original Signature grammar, including formal bounds; names available in
// an enclosing dumper are not evidence of lexical binding.
func nativeMemberTypeScope(obj *ClassObject, inherited map[string]bool, work *workbudget.Budget) bool {
	signature := func(attrs []AttributeInfo) (string, bool) {
		result := ""
		for _, attr := range attrs {
			if a, ok := attr.(*SignatureAttribute); ok {
				if result != "" {
					return "", false
				}
				var valid bool
				result, valid = sourceBridgeUTF8(obj, a.SignatureIndex)
				if !valid || result == "" || !nativeProofWork(work, int64(len(result))) {
					return "", false
				}
			}
		}
		return result, true
	}
	check := func(sig string, inherited map[string]bool, declaration bool) ([]string, bool) {
		if sig == "" {
			return nil, true
		}
		own, refs, ok := types.SignatureTypeVariableReferences(sig)
		if !ok || !declaration && len(own) != 0 {
			return nil, false
		}
		scope := map[string]bool{}
		for n := range inherited {
			scope[n] = true
		}
		for _, n := range own {
			scope[n] = true
		}
		for _, n := range refs {
			if !scope[n] {
				return nil, false
			}
		}
		return own, true
	}
	sig, ok := signature(obj.Attributes)
	if !ok {
		return false
	}
	own, ok := check(sig, inherited, true)
	if !ok {
		return false
	}
	classScope := map[string]bool{}
	for n := range inherited {
		classScope[n] = true
	}
	for _, n := range own {
		classScope[n] = true
	}
	for _, field := range obj.Fields {
		if field == nil {
			return false
		}
		sig, ok := signature(field.Attributes)
		scope := classScope
		if field.AccessFlags&8 != 0 {
			scope = nil
		}
		if !ok {
			return false
		}
		if _, ok := check(sig, scope, false); !ok {
			return false
		}
	}
	for _, method := range obj.Methods {
		if method == nil {
			return false
		}
		sig, ok := signature(method.Attributes)
		scope := classScope
		if method.AccessFlags&8 != 0 {
			scope = nil
		}
		if !ok {
			return false
		}
		if _, ok := check(sig, scope, true); !ok {
			return false
		}
	}
	return true
}

// Prove the compiler-recreated enclosing instance for every constructor,
// including this chains. The original descriptor, Code and IR remain intact.
func nativeMemberProof(obj *ClassObject, work *workbudget.Budget, providers ...callbinding.Provider) *nativeMemberClass {
	return nativeMemberProofWithOwner(obj, nil, work, providers...)
}

func nativeMemberProofWithOwner(obj, enclosing *ClassObject, work *workbudget.Budget, providers ...callbinding.Provider) *nativeMemberClass {
	return nativeMemberProofWithinJointOwner(obj, enclosing, work, nil, providers...)
}

func nativeMemberProofWithinJointOwner(obj, enclosing *ClassObject, work *workbudget.Budget, bridges map[string]*nativeConstructorAccessBridge, providers ...callbinding.Provider) *nativeMemberClass {
	return nativeMemberProofWithLexicalGraph(obj, enclosing, work, bridges, nil, providers...)
}

func nativeMemberProofWithLexicalGraph(obj, enclosing *ClassObject, work *workbudget.Budget, bridges map[string]*nativeConstructorAccessBridge, lexical map[string]*ClassObject, providers ...callbinding.Provider) *nativeMemberClass {
	return nativeMemberProofWithDeclarations(obj, enclosing, work, bridges, lexical, nil, providers...)
}

func nativeMemberProofWithDeclarations(obj, enclosing *ClassObject, work *workbudget.Budget, bridges map[string]*nativeConstructorAccessBridge, lexical map[string]*ClassObject, resolve func(string) (*ClassObject, bool), providers ...callbinding.Provider) *nativeMemberClass {
	expectedCapture := "this$0"
	if lexical != nil {
		var known bool
		expectedCapture, known = nativeMemberLexicalCaptureField(enclosing, lexical, work)
		if !known {
			return nil
		}
	}
	var provider callbinding.Provider
	if len(providers) > 0 {
		provider = providers[0]
	}
	owner, name, flags, known := originalMemberOwner(obj)
	var enumSynthesis *nativeMemberEnumSynthesis
	if flags&0x4000 != 0 {
		enumSynthesis = nativeMemberEnumSynthesisWithDeclarations(obj, flags, resolve, work)
	}
	kind := false
	if flags&0x2000 != 0 {
		kind = nativeMemberAnnotationDeclaration(obj, flags, work, resolve, provider)
	} else {
		kind = nativeMemberDeclarationKindRepresentable(obj, flags, work, resolve)
	}
	if !known || !nativeMemberVersionMetadata(obj, work) || !(kind || enumSynthesis != nil) {
		return nil
	}
	if !nativeMemberDeprecatedMarkerRepresentable(obj, work) {
		return nil
	}
	formalCount, outerFormalCount := 0, 0
	for _, a := range obj.Attributes {
		switch a := a.(type) {
		case *RuntimeVisibleAnnotationsAttribute:
			if !nativeAnnotationDependencies([]AttributeInfo{a}, work, func(string) {}) {
				return nil
			}
		case *DeprecatedAttribute:
			// The original paired encoding was proved above.
		case *RuntimeVisibleTypeAnnotationsAttribute:
			return nil
		case *SignatureAttribute:
			signature, ok := sourceBridgeUTF8(obj, a.SignatureIndex)
			if !ok {
				return nil
			}
			formalCount = len(types.ClassFormalTypeParamNames(signature))
			if flags&8 == 0 && formalCount > 0 {
				if enclosing == nil || enclosing.GetClassName() != owner {
					return nil
				}
				scope := map[string]bool{}
				if lexical != nil {
					var valid bool
					scope, valid = nativeMemberLexicalTypeScope(enclosing, lexical, work)
					if !valid {
						return nil
					}
				}
				found := false
				for _, attr := range enclosing.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						if found {
							return nil
						}
						found = true
						raw, ok := sourceBridgeUTF8(enclosing, sig.SignatureIndex)
						if !ok || !nativeProofWork(work, int64(len(raw))) {
							return nil
						}
						own, refs, ok := types.SignatureTypeVariableReferences(raw)
						if !ok {
							return nil
						}
						outerFormalCount = len(own)
						for _, n := range own {
							scope[n] = true
						}
						for _, n := range refs {
							if !scope[n] {
								return nil
							}
						}
					}
				}
				if !nativeMemberTypeScope(obj, scope, work) {
					return nil
				}
			}
		case *UnparsedAttribute:
			if strings.Contains(a.Name, "Annotation") {
				return nil
			}
		}
	}
	if flags&8 == 0 && lexical != nil {
		scope, valid := nativeMemberLexicalTypeScope(enclosing, lexical, work)
		if !valid || !nativeMemberTypeScope(obj, scope, work) {
			return nil
		}
	}
	if flags&8 != 0 && !nativeMemberTypeScope(obj, nil, work) {
		return nil
	}
	p := &nativeMemberClass{lambdaContext: nativeLambdaImplementationContext{resolve: resolve, metadata: provider}, enumSynthesis: enumSynthesis, object: obj, owner: owner, name: name, static: flags&8 != 0, formalCount: formalCount, outerFormalCount: outerFormalCount, flags: flags, constructors: map[string]*nativeMemberConstructor{}, accessBridges: bridges}
	if lexical != nil && len(lexical) > 0 {
		outermost := owner
		for depth := 0; depth < 64; depth++ {
			o := lexical[outermost]
			if o == nil {
				return nil
			}
			next, _, _, nested := originalMemberOwner(o)
			if !nested {
				break
			}
			outermost = next
			if depth == 63 {
				return nil
			}
		}
		var valid bool
		if enumSynthesis != nil && enumSynthesis.assertions != nil {
			p.assertions, valid = enumSynthesis.assertions, enumSynthesis.assertions.statusOwner == outermost
		} else {
			p.assertions, valid = nativeMemberAssertionProof(obj, outermost, work)
		}
		if !valid {
			return nil
		}
	}
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nok := sourceBridgeUTF8(obj, f.NameIndex)
		d, dok := sourceBridgeUTF8(obj, f.DescriptorIndex)
		if !nok || !dok {
			return nil
		}
		flags, onlySynthetic, known := nativeMemberEffectiveFieldFlags(f, work)
		if !known {
			return nil
		}
		if p.assertions != nil && n == nativeAssertionField {
			continue
		}
		if p.static {
			if p.enumSynthesis != nil && p.enumSynthesis.valuesField == f {
				continue
			}
			if flags&0x1000 != 0 {
				return nil
			}
			continue
		}
		if flags&0x1000 != 0 {
			if p.field != "" || n != expectedCapture || flags != 0x1010 || d != "L"+owner+";" || !onlySynthetic {
				return nil
			}
			p.field = n
		} else if f.AccessFlags&0x0008 != 0 && !nativeMemberStaticConstantField(obj, f, work) {
			return nil
		}
	}
	if !p.static && p.field == "" {
		return nil
	}
	for _, m := range obj.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nok := sourceBridgeUTF8(obj, m.NameIndex)
		desc, dok := sourceBridgeUTF8(obj, m.DescriptorIndex)
		if !nok || !dok || !p.static && m.AccessFlags&0x0008 != 0 && (p.assertions == nil || p.assertions.initializer != m) && (lexical == nil || nativeMemberPrivateAccessProofWithDeclarations(obj, m, resolve, work, lexical) == nil) && !nativeMemberLambdaImplementation(p, m, work) {
			return nil
		}
		for _, a := range m.Attributes {
			switch a := a.(type) {
			case *RuntimeVisibleTypeAnnotationsAttribute:
				return nil
			case *RuntimeVisibleParameterAnnotationsAttribute:
				if !nativeMemberParameterAnnotationsClosed(a, n, desc, !p.static, work) {
					return nil
				}
				for _, attribute := range m.Attributes {
					if types, ok := attribute.(*TypeAnnotationsAttribute); ok {
						if types == nil {
							return nil
						}
						for _, annotation := range types.Annotations {
							if annotation == nil || annotation.TargetType == 0x16 {
								return nil
							}
						}
					}
				}
			case *UnparsedAttribute:
				if strings.Contains(a.Name, "TypeAnnotations") {
					return nil
				}
			}
		}
		if n == "<init>" && m.AccessFlags&0x1000 != 0 {
			if bridge := bridges[desc]; bridge == nil || bridge.method != m {
				return nil
			}
			continue
		}
		if p.static {
			continue
		}
		if n != "<init>" {
			for _, a := range m.Attributes {
				if code, ok := a.(*CodeAttribute); ok {
					if !nativeProofWork(work, int64(len(code.Code))) {
						return nil
					}
					d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					d.Work = work
					if d.ParseOpcode() != nil {
						return nil
					}
					for _, op := range d.Opcodes() {
						if field := constructorMotionMember(obj, op, core.OP_PUTFIELD); field != nil && field.Name == obj.GetClassName() && field.Member == p.field {
							return nil
						}
					}
				}
			}
			continue
		}
		ps, ret, e := callbinding.Descriptor(desc)
		if e != nil || ret != "V" || len(ps) == 0 || ps[0] != "L"+owner+";" || p.constructors[desc] != nil || m.AccessFlags&^uint16(0x0087) != 0 {
			return nil
		}
		seenSignature := false
		for _, a := range m.Attributes {
			if sig, ok := a.(*SignatureAttribute); ok {
				if seenSignature {
					return nil
				}
				seenSignature = true
				raw, ok := sourceBridgeUTF8(obj, sig.SignatureIndex)
				if !ok || !nativeProofWork(work, int64(len(raw))) {
					return nil
				}
				_, sourceParams, result := types.ParseMethodSignatureFull(raw, nil)
				if result == nil || result.String(&class_context.ClassContext{}) != "void" || len(sourceParams) != len(ps)-1 {
					return nil
				}
			}
		}
		var code *CodeAttribute
		for _, a := range m.Attributes {
			if ca, ok := a.(*CodeAttribute); ok {
				if code != nil {
					return nil
				}
				code = ca
			}
		}
		if code == nil || code.MaxStack < 2 || int(code.MaxLocals) < nativeMemberParameterWidth(ps)+1 || len(code.Code) > 65535 || !nativeProofWork(work, int64(len(code.Code))) {
			return nil
		}
		d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
		d.Work = work
		if d.ParseOpcode() != nil {
			return nil
		}
		ops := constructorMotionOps(d)
		cp := &nativeMemberConstructor{descriptor: desc, sourceDescriptor: "(" + strings.Join(ps[1:], "") + ")V", capturePC: -1, delegatePC: -1}
		start := 0
		if len(ops) >= 3 {
			if field := constructorMotionMember(obj, ops[2], core.OP_PUTFIELD); field != nil && field.Name == obj.GetClassName() && field.Member == p.field && field.Description == ps[0] && core.GetRetrieveIdx(ops[0]) == 0 && constructorMotionLoad(ops[0], "Ljava/lang/Object;") && core.GetRetrieveIdx(ops[1]) == 1 && constructorMotionLoad(ops[1], ps[0]) {
				cp.capturePC = int(ops[2].CurrentOffset)
				start = 3
			}
		}
		// The existing abstract stack proof preserves parameter/literal computations
		// and identifies the actual uninitialized receiver delegation.
		next, call := constructorMotionDelegation(obj, ops, start, ps, constructorParameterSlots(ps), provider)
		if next == 0 || call == nil {
			// A named lexical capture is regenerated before the original
			// delegation; no capture is moved across these operand effects.
			next, call = nativeMemberFrameDelegationWithMetadata(obj, m, code, ops, start, work, provider)
		}
		if next == 0 || call == nil || call.Member != "<init>" {
			return nil
		}
		cp.delegatePC = int(ops[next-1].CurrentOffset)
		cp.delegateOwner = call.Name
		cp.delegateDescriptor = call.Description
		if cp.capturePC < 0 {
			if call.Name != obj.GetClassName() {
				return nil
			}
			targets, _, err := callbinding.Descriptor(call.Description)
			if err != nil || len(targets) == 0 || targets[0] != ps[0] {
				return nil
			}
			if len(ops) < 3 || core.GetRetrieveIdx(ops[1]) != 1 || !constructorMotionLoad(ops[1], ps[0]) {
				return nil
			}
		} else if call.Name != obj.GetSupperClassName() {
			return nil
		}
		for _, h := range code.ExceptionTable {
			if h == nil || int(h.StartPc) < cp.delegatePC+3 {
				return nil
			}
		}
		for _, op := range ops {
			if core.GetStoreIdx(op) == 1 {
				return nil
			}
			if field := constructorMotionMember(obj, op, core.OP_PUTFIELD); field != nil && field.Name == obj.GetClassName() && field.Member == p.field && int(op.CurrentOffset) != cp.capturePC {
				return nil
			}
		}
		p.constructors[desc] = cp
	}
	if p.static {
		return p
	}
	if len(p.constructors) == 0 {
		return nil
	}
	// Memoize completed paths: each this edge is visited once, including a
	// long chain of overloads. Gray nodes identify cycles without recursion.
	state := map[string]uint8{}
	for descriptor := range p.constructors {
		path := []string{}
		for state[descriptor] != 2 {
			if !nativeProofWork(work, 1) || state[descriptor] == 1 {
				return nil
			}
			ctor := p.constructors[descriptor]
			if ctor == nil {
				return nil
			}
			state[descriptor] = 1
			path = append(path, descriptor)
			if ctor.capturePC >= 0 {
				break
			}
			descriptor = ctor.delegateDescriptor
		}
		for _, n := range path {
			state[n] = 2
		}
	}
	return p
}

func (c *ClassObjectDumper) planNativeMemberFamily() *nativeMemberFamily {
	if c.foldSiblingResolver == nil || !nativeMemberVersionMetadata(c.obj, c.Work) || !nativeSourceBinaryName(c.obj.GetClassName()) {
		return nil
	}
	if _, _, _, nested := originalMemberOwner(c.obj); nested {
		return nil
	}
	if _, _, anon := originalAnonymousOwner(c.obj); anon {
		return nil
	}
	if !c.nativeMemberAnnotationTablesRepresentable() {
		return nil
	}
	if !nativeMemberTopLevelEvidence(c.obj, c.Work) {
		return nil
	}
	modernNest, nestKnown := c.nativeModernNestOriginalScope()
	if !nestKnown {
		return nil
	}
	p := &nativeMemberFamily{modernNestObjects: modernNest, owner: c.obj.GetClassName(), children: map[string]*nativeMemberClass{}, lexicalObjects: map[string]*ClassObject{c.obj.GetClassName(): c.obj}, retainEmptyMarkers: c.options.TargetSourceVersion >= 11}
	resolveDeclaration := c.nativeAnnotationDeclarationResolver()
	queue := []*ClassObject{c.obj}
	for cursor := 0; cursor < len(queue); cursor++ {
		enclosing := queue[cursor]
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		for _, a := range enclosing.Attributes {
			inner, ok := a.(*InnerClassesAttribute)
			if !ok || inner == nil {
				continue
			}
			for _, row := range inner.Classes {
				if row == nil || !nativeProofWork(c.Work, 1) {
					return nil
				}
				owner, known := sourceBridgeClassName(enclosing, row.OuterClassInfoIndex)
				if !known || owner != enclosing.GetClassName() {
					continue
				}
				name, known := sourceBridgeClassName(enclosing, row.InnerClassInfoIndex)
				if !known || len(p.children) >= nativeMemberLayoutNodeLimit || p.children[name] != nil || p.lexicalObjects[name] != nil {
					return nil
				}
				raw, found := c.foldSiblingResolver(name)
				if !found {
					return nil
				}
				obj, e := c.parseResolved(raw)
				if e != nil || obj.GetClassName() != name {
					return nil
				}
				reader := NewClassObjectDumper(obj)
				reader.options = c.options
				reader.Work = c.Work
				reader.foldSiblingResolver = c.foldSiblingResolver
				reader.declarationResolver = c.declarationResolver
				bridges := reader.originalNativeConstructorAccessBridges()
				// Retain this exact parsed declaration before proving its lexical
				// method scope. A failed child aborts the entire family transaction.
				p.lexicalObjects[name] = obj
				child := nativeMemberProofWithDeclarations(obj, enclosing, c.Work, bridges, p.lexicalObjects, resolveDeclaration, reader.buildInvocationMetadata())
				rowName, rowKnown := sourceBridgeUTF8(enclosing, row.InnerNameIndex)
				if child == nil || !reader.nativeMemberAnnotationTablesRepresentable() || child.owner != owner || !rowKnown || rowName != child.name || row.InnerClassAccessFlags != child.flags {
					return nil
				}
				// Lexical width is not the 64-bit subset representation used by
				// the old registration solver. Keep the bounded source-node
				// profile and charge forest storage before retaining each node.
				if c.Work != nil && c.Work.CheckAlloc(int64(len(p.children)+2)*512) != nil {
					return nil
				}
				p.children[name] = child
				p.lexicalObjects[name] = obj
				if child.enumSynthesis != nil {
					if p.enumConstants == nil {
						p.enumConstants = map[string]*nativeEnumConstantBody{}
					}
					for binary, body := range child.enumSynthesis.bodies {
						if body == nil || p.enumConstants[binary] != nil || p.lexicalObjects[binary] != nil || len(p.enumConstants) >= 64 || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(len(p.enumConstants)+1)*512) != nil {
							return nil
						}
						p.enumConstants[binary] = body
						p.lexicalObjects[binary] = body.object
					}
				}
				queue = append(queue, obj)
			}
		}
	}
	var switchTablesKnown bool
	p.enumSwitchTables, switchTablesKnown = c.nativeEnumSwitchOwnedTables(p.owner)
	if !switchTablesKnown {
		return nil
	}
	if !c.planNativeMethodLocals(p) {
		return nil
	}
	if len(p.children) == 0 && len(p.enumSwitchTables) == 0 && len(p.methodLocals) == 0 {
		return nil
	}
	for name, child := range p.children {
		source, known := p.sourceName(name)
		if !known {
			return nil
		}
		child.sourceName = source
	}
	p.rootAccessBridges = c.originalNativeConstructorAccessBridges()
	if !nativeMemberCollectPrivateGettersResolved(p, c.nativeAnnotationDeclarationResolver(), c.Work) {
		return nil
	}
	if !c.planNativeMemberAccessorCompilerProfile(p) {
		return nil
	}
	// A public abstract source root keeps its private constructors and generated
	// access bridges. A package-private abstract root has a different compiler
	// access profile, so it still needs a separate proof. Synthetic/interface
	// roots, missing ACC_SUPER, and abstract+final cannot use either source form.
	flags := c.obj.AccessFlags
	ordinaryRoot := flags & ^uint16(0x0031) == 0 && flags&0x0020 != 0
	publicAbstractRoot := flags == 0x0421
	if len(p.rootAccessBridges) > 0 && !ordinaryRoot && !publicAbstractRoot {
		return nil
	}
	if p.rootAccessBridges == nil || !c.proveNativeRootBridgeDelegations(p) {
		return nil
	}
	p.emptyMarkers = map[string]*ClassObject{}
	for _, bridges := range p.bridgeOwners() {
		for _, bridge := range bridges {
			raw, known := c.foldSiblingResolver(bridge.marker)
			if !known {
				return nil
			}
			marker, e := c.parseResolved(raw)
			if e != nil || marker.GetClassName() != bridge.marker {
				return nil
			}
			if nativeMemberEmptyAccessMarker(marker, p.owner, c.Work) {
				p.emptyMarkers[bridge.marker] = marker
			}
		}
	}
	// The complete original named forest is committed together. Local and
	// anonymous scopes still require their separately proved joint plans.
	metadata := c.buildInvocationMetadata()
	for _, child := range p.children {
		if !nativeMemberSiblingSuperClosed(child, p, c.Work, metadata) {
			return nil
		}
	}
	return p
}

// Source super(...) may omit an enclosing operand only when both original
// declarations belong to this complete family and the bytecode passes this
// constructor's unchanged enclosing parameter or its proved lexical capture
// path. A qualified foreign outer or a
// computed operand needs its own source origin proof and is refused here.
func nativeMemberSiblingSuperClosed(child *nativeMemberClass, p *nativeMemberFamily, work *workbudget.Budget, metadata callbinding.Provider) bool {
	seen := map[string]bool{}
	for node := child; node != nil; node = p.children[node.object.GetSupperClassName()] {
		name := node.object.GetClassName()
		if seen[name] || !nativeProofWork(work, 1) {
			return false
		}
		seen[name] = true
	}
	parent := p.children[child.object.GetSupperClassName()]
	if parent == nil || parent.static {
		return true
	}
	if child.static {
		return false
	}
	for _, method := range child.object.Methods {
		name, _ := sourceBridgeUTF8(child.object, method.NameIndex)
		desc, _ := sourceBridgeUTF8(child.object, method.DescriptorIndex)
		ctor := child.constructors[desc]
		if name != "<init>" || ctor == nil || ctor.capturePC < 0 {
			continue
		}
		if ctor.delegateOwner != parent.object.GetClassName() || nativeMemberConstructorForAllocation(parent, ctor.delegateDescriptor) == nil {
			return false
		}
		proved := false
		for _, attr := range method.Attributes {
			code, ok := attr.(*CodeAttribute)
			if !ok {
				continue
			}
			if !nativeProofWork(work, int64(len(code.Code))) {
				return false
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(child.object.ConstantPool, i) })
			d.Work = work
			if d.ParseOpcode() != nil {
				return false
			}
			ops := constructorMotionOps(d)
			start := -1
			for i, op := range ops {
				if int(op.CurrentOffset) == ctor.capturePC {
					start = i + 1
					break
				}
			}
			params, _, err := callbinding.Descriptor(desc)

			if err == nil && start >= 0 {
				path, pathKnown := nativeMemberSuperEnclosingPath(child, parent, p, ops, start, work)
				if !pathKnown {
					return false
				}
				next, call := constructorMotionDelegationEnclosing(child.object, ops, start, params, constructorParameterSlots(params), metadata, path, 1)
				if next == 0 || call == nil {
					next, call = nativeMemberFrameDelegationWithMetadata(child.object, method, code, ops, start, work, metadata, path)
				}
				ctor.enclosingSuperPath = path
				proved = next > 0 && call != nil && call.Name == ctor.delegateOwner && call.Description == ctor.delegateDescriptor && int(ops[next-1].CurrentOffset) == ctor.delegatePC
				if proved && parent.accessBridges[ctor.delegateDescriptor] != nil {
					// The original private-super bridge carries one unused marker.
					// Its enclosing operand was independently proved from slot 1
					// by constructorMotionDelegation. Only an adjacent original
					// ACONST_NULL may be dropped, never an effectful expression.
					proved = next >= 2 && ops[next-2].Instr.OpCode == core.OP_ACONST_NULL && len(ops[next-2].Data) == 0
				}
			}
		}
		if !proved {
			return false
		}
		ctor.projectedSuper = true
	}
	return true
}

func (p *nativeMemberFamily) sourceName(binary string) (string, bool) {
	binary = strings.ReplaceAll(binary, ".", "/")
	if p == nil {
		return "", false
	}
	if child := p.children[binary]; child != nil {
		if child.sourceName != "" {
			return child.sourceName, true
		}
		parts := []string{child.name}
		owner := child.owner
		seen := map[string]bool{binary: true}
		for parent := p.children[owner]; parent != nil; parent = p.children[owner] {
			if seen[owner] || len(parts) >= 64 {
				return "", false
			}
			seen[owner] = true
			parts = append(parts, parent.name)
			owner = parent.owner
		}
		name := strings.ReplaceAll(owner, "/", ".")
		for i := len(parts) - 1; i >= 0; i-- {
			name += "." + parts[i]
		}
		return name, true
	}
	if source := p.sourceDependencies[binary]; source != "" {
		return source, true
	}
	return "", false
}

// A completed foreign declaration contributes constructor metadata solely for
// projecting a NEW's enclosing operand. It is not a child or lexical owner;
// getters, capture reads, registration and private bridges still use children.
func (p *nativeMemberFamily) allocationClass(binary string) *nativeMemberClass {
	if child := p.children[binary]; child != nil {
		return child
	}
	return p.allocationDependencies[binary]
}

type nativeMemberAllocation struct {
	child                    *nativeMemberClass
	rootObject               *ClassObject
	descriptor               string
	newPC, invokePC, checkPC int
	slot                     int
	enclosingReadPC          int
	implicitEnclosing        bool
	implicitReceiverClass    string
	enclosingParameter       *nativeMemberEnclosingParameter
	freshEnclosing           *nativeMemberFreshEnclosing
	anonymousEnclosingRead   *nativeMemberLexicalRead
	anonymousEnclosingMethod string
}

func (a *nativeMemberAllocation) allocatedObject() *ClassObject {
	if a == nil {
		return nil
	}
	if a.child != nil {
		return a.child.object
	}
	return a.rootObject
}

func (c *ClassObjectDumper) nativeMemberAllocations(p *nativeMemberFamily) (map[string]map[int]*nativeMemberAllocation, bool) {
	result := map[string]map[int]*nativeMemberAllocation{}
	originalDeclarations := c.nativeAnnotationDeclarationResolver()
	for _, constant := range c.obj.ConstantPool {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		handle, ok := constant.(*ConstantMethodHandleInfo)
		if !ok || handle == nil {
			continue
		}
		if handle.ReferenceIndex == 0 || int(handle.ReferenceIndex) > len(c.obj.ConstantPool) {
			return nil, false
		}
		ref := nativeConstantMember(c.obj.ConstantPool[handle.ReferenceIndex-1])
		if ref == nil || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(c.obj.ConstantPool) {
			return nil, false
		}
		nt, ok := c.obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			return nil, false
		}
		name, nk := sourceBridgeUTF8(c.obj, nt.NameIndex)
		desc, dk := sourceBridgeUTF8(c.obj, nt.DescriptorIndex)
		owner, known := sourceBridgeClassName(c.obj, ref.ClassIndex)
		if !nk || !dk || !known {
			return nil, false
		}
		if name == "<init>" {
			if _, method := c.obj.ConstantPool[handle.ReferenceIndex-1].(*ConstantMethodrefInfo); !method || handle.ReferenceKind != 8 || !nativeMemberOriginalConstructorAccess(p, c.obj, owner, desc, c.Work) {
				return nil, false
			}
		}
	}
	for _, m := range c.obj.Methods {
		name, _ := c.obj.getUtf8(m.NameIndex)
		desc, _ := c.obj.getUtf8(m.DescriptorIndex)
		key := name + desc
		if result[key] != nil {
			return nil, false
		}
		result[key] = map[int]*nativeMemberAllocation{}
		if name == "<init>" && nativeEnumConstantConstructorOwned(p, c.obj, desc) {
			continue
		}
		if name == "<init>" && p.constructorBridges(c.obj.GetClassName())[desc] != nil {
			if !nativeMemberJointBridgeEquivalent(p, c.obj, m, desc, c.Work) {
				return nil, false
			}
			continue
		}
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
			if !nativeProofWork(c.Work, int64(len(code.Code))) {
				return nil, false
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
			d.Work = c.Work
			if d.ParseOpcode() != nil {
				return nil, false
			}
			ops := constructorMotionOps(d)
			var allocationInvocations map[int]nativeMemberAllocationInvocation
			allocationInvocationsChecked := false
			for i, op := range ops {
				if op.Instr.OpCode != core.OP_NEW {
					continue
				}
				owner, known := sourceBridgeClassName(c.obj, core.Convert2bytesToInt(op.Data))
				child := p.allocationClass(owner)
				if known && owner == p.owner && len(p.rootAccessBridges) > 0 {
					plan, ok := nativeRootBridgeAllocation(c.obj, ops, i, p.lexicalObjects[p.owner], p.rootAccessBridges, c.Work)
					if !ok {
						if !allocationInvocationsChecked {
							var valid bool
							allocationInvocations, valid = c.nativeMemberAllocationInvocations(m, code)
							if !valid {
								return nil, false
							}
							allocationInvocationsChecked = true
						}
						plan, ok = nativeRootBridgeAllocation(c.obj, ops, i, p.lexicalObjects[p.owner], p.rootAccessBridges, c.Work, allocationInvocations)
					}
					if !ok {
						return nil, false
					}
					if plan != nil {
						if result[key][plan.invokePC] != nil {
							return nil, false
						}
						result[key][plan.invokePC] = plan
					}
					continue
				}
				if !known || child == nil {
					continue
				}
				if p.children[owner] == nil && len(child.accessBridges) != 0 {
					// A foreign dependency does not contribute registration
					// ordinals or private synthetic constructor ownership.
					return nil, false
				}
				if child.static {
					if len(child.accessBridges) == 0 {
						continue
					}
					plan, ok := nativeMemberStaticBridgeAllocation(c.obj, ops, i, child, c.Work)
					if !ok {
						if !allocationInvocationsChecked {
							var valid bool
							allocationInvocations, valid = c.nativeMemberAllocationInvocations(m, code)
							if !valid {
								return nil, false
							}
							allocationInvocationsChecked = true
						}
						plan, ok = nativeMemberStaticBridgeAllocation(c.obj, ops, i, child, c.Work, allocationInvocations)
					}
					if !ok {
						return nil, false
					}
					if plan != nil {
						if result[key][plan.invokePC] != nil {
							return nil, false
						}
						result[key][plan.invokePC] = plan
					}
					continue
				}
				if i+3 >= len(ops) || ops[i+1].Instr.OpCode != core.OP_DUP {
					return nil, false
				}
				plan := &nativeMemberAllocation{child: child, newPC: int(op.CurrentOffset), checkPC: -1, enclosingReadPC: -1, slot: -1}
				cursor := i + 3
				if ops[i+2].Instr.OpCode == core.OP_NEW {
					if !allocationInvocationsChecked {
						var valid bool
						allocationInvocations, valid = c.nativeMemberAllocationInvocations(m, code)
						if !valid {
							return nil, false
						}
						allocationInvocationsChecked = true
					}
					invocation, found := allocationInvocations[plan.newPC]
					qualifier := invocation.enclosing
					if !found || invocation.owner != owner || qualifier == nil || qualifier.owner != child.owner || qualifier.newPC != int(ops[i+2].CurrentOffset) {
						return nil, false
					}
					for cursor < len(ops) && int(ops[cursor].CurrentOffset) != qualifier.invokePC {
						if cursor-i > 512 || !nativeProofWork(c.Work, 1) {
							return nil, false
						}
						cursor++
					}
					if cursor == len(ops) {
						return nil, false
					}
					plan.freshEnclosing = qualifier
					cursor++
					// A freshly initialized NEW is nonnull. Retain an explicit
					// qualifier check if the original compiler emitted one.
					if cursor+2 < len(ops) && ops[cursor].Instr.OpCode == core.OP_DUP && nativeMemberNullCheck(c.obj, ops[cursor+1]) && ops[cursor+2].Instr.OpCode == core.OP_POP {
						plan.checkPC = int(ops[cursor+1].CurrentOffset)
						cursor += 3
					}
				} else {
					if !constructorMotionLoad(ops[i+2], "L"+child.owner+";") {
						return nil, false
					}
					plan.slot = core.GetRetrieveIdx(ops[i+2])
					// An anonymous body also has an enclosing instance. Reuse only
					// the committed forest's original lexical THIS chain, not a
					// same-typed field or a captured foreign outer object.
					if plan.slot == 0 && m.AccessFlags&8 == 0 {
						for end := cursor; end < len(ops); end++ {
							field := constructorMotionMember(c.obj, ops[end], core.OP_GETFIELD)
							if field == nil || !nativeProofWork(c.Work, 1) {
								break
							}
							pc := int(ops[end].CurrentOffset)
							read := nativeMemberAnonymousAllocationEnclosingRead(p, c.obj, key, pc, child.owner, c.Work)
							if read == nil {
								continue
							}
							plan.enclosingReadPC, plan.implicitEnclosing = pc, true
							plan.anonymousEnclosingRead, plan.anonymousEnclosingMethod = read, key
							cursor = end + 1
							if cursor+2 < len(ops) && ops[cursor].Instr.OpCode == core.OP_DUP && nativeMemberNullCheck(c.obj, ops[cursor+1]) && ops[cursor+2].Instr.OpCode == core.OP_POP {
								plan.checkPC = int(ops[cursor+1].CurrentOffset)
								cursor += 3
								plan.implicitEnclosing = false
							}
							break
						}
					}
					// An unqualified sibling allocation reads the current member's
					// original enclosing capture. This is an origin witness, not
					// a same-erasure field or an assumption that an outer is nonnull.
					if current := p.children[c.obj.GetClassName()]; current != nil && !current.static && current.owner == child.owner && m.AccessFlags&8 == 0 && plan.slot == 0 && cursor < len(ops) {
						field := constructorMotionMember(c.obj, ops[cursor], core.OP_GETFIELD)
						if field != nil && field.Name == current.object.GetClassName() && field.Member == current.field && field.Description == "L"+child.owner+";" {
							plan.enclosingReadPC = int(ops[cursor].CurrentOffset)
							cursor++
							plan.implicitEnclosing = true
							// Explicit Outer.this.new uses the same capture but retains
							// the original qualifier check. Do not erase that protocol.
							if cursor+2 < len(ops) && ops[cursor].Instr.OpCode == core.OP_DUP && nativeMemberNullCheck(c.obj, ops[cursor+1]) && ops[cursor+2].Instr.OpCode == core.OP_POP {
								plan.checkPC = int(ops[cursor+1].CurrentOffset)
								cursor += 3
								plan.implicitEnclosing = false
							}
						}
					}
					if plan.enclosingReadPC < 0 && plan.slot == 0 && cursor < len(ops) && ops[cursor].Instr.OpCode != core.OP_DUP && nativeMemberInheritedAllocationThis(c.obj, m, ops, child, originalDeclarations, c.Work) {
						plan.implicitReceiverClass = c.obj.GetClassName()
					}
					if plan.enclosingReadPC < 0 && cursor < len(ops) && ops[cursor].Instr.OpCode != core.OP_DUP {
						plan.enclosingParameter = nativeMemberConstructorEnclosingParameter(c.obj, m, code, ops, i, p.children[c.obj.GetClassName()], child, c.Work)
						if plan.enclosingParameter != nil {
							plan.implicitEnclosing = true
						}
					}
					if plan.enclosingReadPC < 0 && plan.implicitReceiverClass == "" && plan.enclosingParameter == nil && (plan.slot != 0 || m.AccessFlags&8 != 0 || c.obj.GetClassName() != child.owner) {
						if cursor+2 >= len(ops) || ops[cursor].Instr.OpCode != core.OP_DUP {
							return nil, false
						}
						if !nativeMemberNullCheck(c.obj, ops[cursor+1]) || ops[cursor+2].Instr.OpCode != core.OP_POP {
							return nil, false
						}
						plan.checkPC = int(ops[cursor+1].CurrentOffset)
						cursor += 3
					}
				}
				// Preserve the original operand producers and their effects. A
				// nested allocation or branch requires the immutable typed frame
				// proof of this NEW's unique initialization. Rendering independently
				// checks the same NEW/invoke PCs; nominal owner equality is insufficient.
				for j := cursor; j < len(ops); j++ {
					if !nativeProofWork(c.Work, 1) {
						return nil, false
					}
					call := constructorMotionMember(c.obj, ops[j], core.OP_INVOKESPECIAL)
					invocation, identityKnown := allocationInvocations[plan.newPC]
					if call != nil && call.Name == owner && call.Member == "<init>" && (!allocationInvocationsChecked || identityKnown && invocation.pc == int(ops[j].CurrentOffset) && invocation.owner == owner && invocation.descriptor == call.Description) {
						if nativeMemberConstructorForAllocation(child, call.Description) == nil {
							return nil, false
						}
						if child.accessBridges[call.Description] != nil && (j == cursor || ops[j-1].Instr.OpCode != core.OP_ACONST_NULL || len(ops[j-1].Data) != 0) {
							return nil, false
						}
						plan.descriptor = call.Description
						plan.invokePC = int(ops[j].CurrentOffset)
						if result[key][plan.invokePC] != nil {
							return nil, false
						}
						result[key][plan.invokePC] = plan
						break
					}
					if ops[j].Instr.OpCode == core.OP_NEW || ops[j].Instr.OpCode == core.OP_GOTO {
						if !allocationInvocationsChecked {
							var valid bool
							allocationInvocations, valid = c.nativeMemberAllocationInvocations(m, code)
							if !valid {
								return nil, false
							}
							allocationInvocationsChecked = true
						}
						invocation, known := allocationInvocations[plan.newPC]
						if !known || invocation.owner != owner {
							return nil, false
						}
						continue
					}
					if ops[j].Instr.OpCode == core.OP_RETURN || ops[j].Instr.OpCode == core.OP_ARETURN {
						return nil, false
					}
				}
				if plan.descriptor == "" {
					return nil, false
				}
			}
			for _, op := range d.Opcodes() {
				call := constructorMotionMember(c.obj, op, core.OP_INVOKESPECIAL)
				if call != nil && call.Member == "<init>" && !nativeMemberOriginalConstructorAccess(p, c.obj, call.Name, call.Description, c.Work) {
					return nil, false
				}
				if call == nil || call.Member != "<init>" || p.allocationClass(call.Name) == nil || p.allocationClass(call.Name).static && p.allocationClass(call.Name).accessBridges[call.Description] == nil {
					continue
				}
				if group := p.anonymousUnits[c.obj.GetClassName()]; group != nil {
					if anonymous := group.children[c.obj.GetClassName()]; anonymous != nil && anonymous.memberSuper == p.allocationClass(call.Name) && name == "<init>" && desc == anonymous.descriptor && int(op.CurrentOffset) == anonymous.superPC && call.Description == anonymous.superDescriptor {
						continue
					}
				}
				if result[name+desc][int(op.CurrentOffset)] != nil {
					continue
				}
				// An initial owned static SUPER is a delegation, not a NEW.
				// Its separate original THIS/NULL-marker packet certificate
				// must match before this source-only bridge can be projected.
				if p.rootBridgeDelegation(c.obj, name, desc, call.Name, call.Description, int(op.CurrentOffset)) != nil {
					continue
				}
				if c.nativeMemberForeignOriginalSuper(p, m, call.Name, call.Description, int(op.CurrentOffset)) {
					continue
				}
				ctor := p.allocationClass(call.Name).constructors[desc]
				if name == "<init>" && c.obj.GetClassName() == call.Name && ctor != nil && ctor.capturePC < 0 && ctor.delegateDescriptor == call.Description && ctor.delegatePC == int(op.CurrentOffset) {
					continue
				}
				if current := p.children[c.obj.GetClassName()]; name == "<init>" && current != nil {
					ctor := current.constructors[desc]
					if ctor != nil && ctor.projectedSuper && ctor.delegateOwner == call.Name && ctor.delegateDescriptor == call.Description && ctor.delegatePC == int(op.CurrentOffset) {
						continue
					}
				}
				return nil, false
			}
		}
	}
	return result, true
}

// Source lexical ownership is not JVM private access. In the supported
// pre-nestmate profile, an original private constructor can be invoked only
// from its declaring class. A sibling uses a separately proved synthetic
// bridge; giving it a direct source call would erase the original access
// failure. This also applies to foreign users discovered by the archive index.
func nativeMemberOriginalConstructorAccess(p *nativeMemberFamily, caller *ClassObject, owner, descriptor string, work *workbudget.Budget) bool {
	if p == nil || caller == nil {
		return false
	}
	var target *ClassObject
	if owner == p.owner {
		target = p.lexicalObjects[owner]
	} else if child := p.allocationClass(owner); child != nil {
		target = child.object
	} else {
		// A separately committed dependency checks its own indexed callers.
		return true
	}
	if target == nil || target.GetClassName() != owner {
		return false
	}
	var found *MemberInfo
	for _, m := range target.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, nk := sourceBridgeUTF8(target, m.NameIndex)
		desc, dk := sourceBridgeUTF8(target, m.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if name == "<init>" && desc == descriptor {
			if found != nil {
				return false
			}
			found = m
		}
	}
	return found != nil && (found.AccessFlags&2 == 0 || caller.GetClassName() == owner || nativeModernNestPrivateConstructorAccess(p, caller, target, work))
}

func nativeMemberBinding(ctx *class_context.ClassContext, p *nativeMemberFamily, work *workbudget.Budget) *class_context.ClassContext {
	copy := *ctx
	projected := map[string]callbinding.Class{}
	copy.InvocationMetadata = func(owner string) (callbinding.Class, bool) {
		original := ctx.InvocationMetadata
		if original == nil {
			return callbinding.Class{}, false
		}
		if cl, known := projected[owner]; known {
			return cl, true
		}
		cl, ok := original(owner)
		if !ok {
			return cl, false
		}
		binary := strings.ReplaceAll(owner, ".", "/")
		child := p.allocationClass(binary)
		bridges := p.constructorBridges(binary)
		if (child == nil || child.static) && len(bridges) == 0 {
			return cl, true
		}
		if !nativeProofWork(work, int64(len(cl.Methods))) || work != nil && work.CheckAlloc(int64(len(cl.Methods))*96) != nil {
			p.failed = true
			return callbinding.Class{}, false
		}
		cl.Methods = append([]callbinding.Method(nil), cl.Methods...)
		filtered := cl.Methods[:0]
		for _, m := range cl.Methods {
			if m.Name == "<init>" && bridges[m.Desc] != nil {
				continue
			}
			filtered = append(filtered, m)
		}
		cl.Methods = filtered
		for i, m := range cl.Methods {
			if m.Name == "<init>" && child != nil {
				if ctor := child.constructors[m.Desc]; ctor != nil {
					cl.Methods[i].Desc = ctor.sourceDescriptor
				}
			}
		}
		projected[owner] = cl
		return cl, true
	}

	// Constructor Signature omits the compiler's enclosing parameter. Its
	// exact descriptor key must move with the proved source descriptor; merely
	// shortening the call loses generic inference (or binds a different ctor).
	{
		type entry struct {
			signature string
			methods   map[string]string
			known     bool
		}
		cache := map[string]entry{}
		copy.SiblingClassSig = func(owner string) (string, map[string]string, bool) {
			if hit, ok := cache[owner]; ok {
				return hit.signature, hit.methods, hit.known
			}
			if ctx.SiblingClassSig == nil {
				return "", nil, false
			}
			signature, methods, known := ctx.SiblingClassSig(owner)
			child := p.allocationClass(strings.ReplaceAll(owner, ".", "/"))
			if child == nil || child.static {
				return signature, methods, known
			}
			cl, ok := copy.InvocationMetadata(owner)
			if !ok || !nativeProofWork(work, int64(len(methods)+len(cl.Methods))) || work != nil && work.CheckAlloc(int64(len(methods)+len(cl.Methods))*96) != nil {
				p.failed = true
				return "", nil, false
			}
			projected := make(map[string]string, len(methods)+len(cl.Methods))
			for key, value := range methods {
				projected[key] = value
			}
			for descriptor := range child.constructors {
				delete(projected, class_context.MethodDescKey("<init>", descriptor))
			}
			for _, m := range cl.Methods {
				if m.Name != "<init>" {
					continue
				}
				projected[class_context.MethodDescKey(m.Name, m.Desc)] = ""
				if m.Signature == "" {
					continue
				}
				_, params, _ := types.ParseMethodSignatureFull(m.Signature, ctx)
				descriptorParams, result, err := callbinding.Descriptor(m.Desc)
				if err != nil || result != "V" || len(params) != len(descriptorParams) {
					p.failed = true
					return "", nil, false
				}
				projected[class_context.MethodDescKey(m.Name, m.Desc)] = m.Signature
			}
			cache[owner] = entry{signature, projected, known}
			return signature, projected, known
		}
	}
	return &copy
}
func (c *ClassObjectDumper) wireNativeMemberSource() {
	if child := c.nativeMemberCurrent; child != nil {
		c.FuncCtx.LexicalClassName = child.name
	}
	p := c.nativeMemberRoot
	looked := map[string]bool{}
	if p == nil && c.nativeMemberLookup != nil {
		p = &nativeMemberFamily{children: map[string]*nativeMemberClass{}}
		for _, constant := range c.obj.ConstantPool {
			if cls, ok := constant.(*ConstantClassInfo); ok && cls != nil {
				name, known := sourceBridgeUTF8(c.obj, cls.NameIndex)
				if known {
					if !looked[name] {
						looked[name] = true
						if child := c.nativeMemberLookup(name); child != nil {
							p.children[name] = child
						}
					}
				}
			}
		}
		c.nativeMemberRoot = p
	}
	if p == nil {
		return
	}
	ctx := c.FuncCtx
	ctx.SiblingLexicalTypeOwners = c.nativeMemberTypeOwners(p)
	c.wireNativeEnumSwitchSource(p, ctx)
	c.wireNativeMemberPrivateGetters(p, ctx)
	if (c.nativeMemberCurrent != nil || c.nativeMethodLocalCurrent != nil) && p.lexicalObjects != nil {
		reads, valid := nativeMemberLexicalReads(c.obj, p, c.Work)
		if !valid {
			p.failed = true
		} else {
			originalDeclarations := c.nativeAnnotationDeclarationResolver()
			ctx.SourceLexicalInvocationReceiver = func(value any) (string, bool) {
				call, ok := value.(*values.FunctionCallExpression)
				if !ok || call == nil {
					return "", false
				}
				operand, known := nativeMemberEnclosingUnpack(call.Object, c.Work)
				if !known {
					return "", false
				}
				var read *nativeMemberLexicalRead
				var child *nativeMemberClass
				var source string
				if field, ok := operand.(*values.RefMember); ok && field != nil && field.HasOriginPC {
					read = reads[ctx.FunctionName+ctx.CurrentMethodDesc][field.OriginPC]
					if read == nil || !nativeMemberLexicalReadOperand(field, read, c.Work, ctx) {
						return "", false
					}
					child = p.children[read.owner]
					source, known = ctx.SourceLexicalCapturedField(field, field.OriginPC, field.Member)
					if !known {
						return "", false
					}
				} else if current := c.nativeMemberCurrent; current != nil && !current.static && ctx.FunctionName == "<init>" && current.constructors[ctx.CurrentMethodDesc] != nil && nativeMemberSourceEnclosingParameter(operand, ctx, current.owner) {
					child = current
					read = &nativeMemberLexicalRead{descriptor: "L" + current.owner + ";", parameterOwner: current.owner}
					source = operand.String(ctx)
				} else {
					return "", false
				}
				if child == nil || child.outerFormalCount == 0 || !nativeLexicalRawInvocation(c.obj, ctx.FunctionName, ctx.CurrentMethodDesc, call, read, originalDeclarations, c.Work) {
					return "", false
				}
				owner := ctx.ShortTypeName(strings.ReplaceAll(child.owner, "/", "."))
				return "((" + owner + ")(" + source + "))", true
			}
			ctx.SourceLexicalCapturedField = func(value any, pc int, name string) (string, bool) {
				read := reads[ctx.FunctionName+ctx.CurrentMethodDesc][pc]
				if read == nil {
					if field, ok := value.(*values.RefMember); ok && field != nil && !sourceProofNil(field.Object) {
						if erased, known := values.SourceTypeErasure(field.Object.Type(), ctx); known && strings.HasPrefix(erased, "L") && strings.HasSuffix(erased, ";") {
							owner := p.children[erased[1:len(erased)-1]]
							if owner != nil && !owner.static && owner.field == name {
								p.failed = true
							}
						}
					}
					return "", false
				}
				if name != read.field || !nativeMemberLexicalReadOperand(value, read, c.Work, ctx) {
					p.failed = true
					return "", false
				}
				owner := p.children[read.owner]
				if owner == nil {
					// The local's first hop retains the existing exact capture
					// binding/placement hook. Only later named-owner hops are
					// projected here, with the complete operand chain above.
					if local := c.nativeMethodLocalCurrent; local != nil && read.prior == nil && read.owner == c.obj.GetClassName() && read.field == local.constructor.enclosingField {
						return "", false
					}
					p.failed = true
					return "", false
				}
				return ctx.ShortTypeName(strings.ReplaceAll(owner.owner, "/", ".")) + ".this", true
			}
		}
	}
	if c.obj.GetClassName() == p.owner || c.nativeMemberCurrent != nil || c.nativeEnumConstantCurrent != nil {
		ctx.LexicalTypeNames = map[string]bool{}
		for _, child := range p.children {
			ctx.LexicalTypeNames[child.name] = true
		}
	}
	prior := ctx.DeclarationSourceName
	ctx.DeclarationSourceName = func(n string) (string, bool) {
		if source, known := p.sourceName(n); known {
			return source, true
		}
		if c.nativeMemberLookup != nil {
			key := strings.ReplaceAll(n, ".", "/")
			if !looked[key] {
				looked[key] = true
				if child := c.nativeMemberLookup(key); child != nil {
					p.children[key] = child
					return p.sourceName(n)
				}
			}
		}
		if prior != nil {
			return prior(n)
		}
		return "", false
	}
	if len(p.children) == 0 && len(p.allocationDependencies) == 0 {
		return
	}
	plans, known := c.nativeMemberAllocations(p)
	if !known {
		p.failed = true
		return
	}
	c.nativeMemberCalls = plans
	c.nativeMemberChecks = map[string]map[int]bool{}
	for method, allocations := range plans {
		checks := map[int]bool{}
		for _, plan := range allocations {
			if plan.checkPC >= 0 {
				checks[plan.checkPC] = true
			}
		}
		c.nativeMemberChecks[method] = checks
	}
	binding := nativeMemberBinding(ctx, p, c.Work)
	ctx.SourceMemberCandidate = func(owner string) bool {
		binary := strings.ReplaceAll(owner, ".", "/")
		if binary == p.owner {
			return len(p.rootAccessBridges) > 0
		}
		child := p.allocationClass(binary)
		return child != nil && (!child.static || len(child.accessBridges) > 0)
	}
	ctx.SourceMemberDescriptorCandidate = func(owner, desc string) bool {
		binary := strings.ReplaceAll(owner, ".", "/")
		if binary == p.owner {
			return p.rootAccessBridges[desc] != nil
		}
		child := p.allocationClass(binary)
		return child != nil && (!child.static || child.accessBridges[desc] != nil)
	}
	ctx.SourceMemberAllocation = func(owner, desc string, newPC, pc int, args []class_context.SourceCaptureOperand) (string, bool) {
		fail := func() (string, bool) { p.failed = true; return "", false }
		plan := plans[ctx.FunctionName+ctx.CurrentMethodDesc][pc]
		if child := p.allocationClass(strings.ReplaceAll(owner, ".", "/")); child != nil && child.static && child.accessBridges[desc] == nil {
			return "", false
		}
		if plan == nil || plan.allocatedObject() == nil || plan.allocatedObject().GetClassName() != strings.ReplaceAll(owner, ".", "/") || plan.newPC != newPC || plan.descriptor != desc || len(args) == 0 {
			return fail()
		}
		if plan.rootObject != nil {
			return nativeRootBridgeSourceAllocation(plan, args, ctx, binding, p)
		}
		if plan.child.static {
			return nativeMemberStaticBridgeSource(plan, args, ctx, binding, p)
		}
		if plan.freshEnclosing != nil {
			if !nativeMemberFreshEnclosingOperand(args[0].Value, plan.freshEnclosing, c.Work) {
				return fail()
			}
		} else if plan.enclosingReadPC >= 0 {
			if !nativeMemberLexicalEnclosingOperand(args[0].Value, plan, p, c.obj.GetClassName(), c.nativeMemberBody, c.Work) {
				return fail()
			}
		} else if plan.enclosingParameter != nil {
			if !c.nativeMemberConstructorEnclosingOperand(args[0].Value, plan.enclosingParameter, ctx) {
				return fail()
			}
		} else if !args[0].Receiver && !args[0].Local {
			return fail()
		}
		outer, outerKnown := args[0].Value.(values.JavaValue)
		if !outerKnown {
			return fail()
		}
		erasure, typeKnown := values.SourceTypeErasure(outer.Type(), ctx)
		inheritedThis := plan.implicitReceiverClass != "" && args[0].Receiver && nativeMemberInheritedAllocationOperand(outer, plan.implicitReceiverClass, ctx, c.Work)
		if plan.implicitReceiverClass != "" && !inheritedThis || !typeKnown || !inheritedThis && erasure != "L"+plan.child.owner+";" {
			return fail()
		}
		ctor := nativeMemberConstructorForAllocation(plan.child, desc)
		if ctor == nil {
			return fail()
		}
		invoke := &values.FunctionCallExpression{ClassName: owner, FunctionName: "<init>", Descriptor: ctor.sourceDescriptor, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: pc}
		sourceArgs := args[1:]
		if plan.child.accessBridges[desc] != nil {
			if len(sourceArgs) == 0 || !nativeMemberBridgeSourceDummy(sourceArgs[len(sourceArgs)-1].Value) {
				return fail()
			}
			sourceArgs = sourceArgs[:len(sourceArgs)-1]
		}
		for _, arg := range sourceArgs {
			v, ok := arg.Value.(values.JavaValue)
			if !ok {
				return fail()
			}
			invoke.Arguments = append(invoke.Arguments, v)
		}
		mt, e := types.ParseMethodDescriptor(ctor.sourceDescriptor)
		if e != nil {
			return fail()
		}
		invoke.FuncType = mt.FunctionType()
		allocationBinding := *ctx
		allocationBinding.InvocationMetadata = binding.InvocationMetadata
		allocationBinding.SiblingClassSig = binding.SiblingClassSig
		arguments := invoke.ArgumentStrings(&allocationBinding)
		sourceName := plan.child.name
		if plan.enclosingParameter != nil {
			var nameKnown bool
			sourceName, nameKnown = c.nativeMemberEnclosingParameterAllocationName(plan, p, ctx)
			if !nameKnown {
				return fail()
			}
		}
		diamond := false
		if plan.child.formalCount > 0 {
			// Java forbids a raw member beneath a parameterized enclosing
			// instance (and the reverse). Infer only the member's parameters;
			// preserve a genuinely raw outer/member pair.
			typedOuter := plan.child.outerFormalCount == 0 || plan.implicitEnclosing || args[0].Receiver && c.obj.GetClassName() == plan.child.owner
			if param, ok := outer.Type().RawType().(*types.JavaParameterizedType); ok && len(param.TypeArgs) == plan.child.outerFormalCount {
				typedOuter = true
			}
			if typedOuter {
				sourceName += "<>"
				diamond = true
			}
		}
		source := "new " + sourceName + "(" + strings.Join(arguments, ",") + ")"
		if inheritedThis {
			// Bind selection to the exact declaring class, even if the subclass
			// has a same-named member or parameterized inherited view. This is
			// an original superclass widening, not a runtime CHECKCAST. The
			// qualifier check is inert because the independently bound THIS is
			// nonnull; original argument producers retain their order and effects.
			source = "((" + ctx.ShortTypeName(strings.ReplaceAll(plan.child.owner, "/", ".")) + ")(" + args[0].Text + "))." + source
		} else if rawThis := nativeMemberConstructorRawThis(p, plan.child, desc, c.obj.GetClassName(), args[0], ctx, c.Work); rawThis != "" {
			source = rawThis + "." + source
		} else if !plan.implicitEnclosing && !(args[0].Receiver && c.obj.GetClassName() == plan.child.owner) {
			source = "(" + args[0].Text + ")." + source
		}
		if plan.child.accessBridges[desc] != nil {
			// javac translates constructor arguments before registering the
			// private constructor, and a qualified enclosing operand after it.
			// Keep the unproved qualified-access registration order closed.
			if strings.Contains(args[0].Text, "jdec-owned-getter:") || strings.Contains(args[0].Text, nativeConstructorRegistrationPrefix) {
				return fail()
			}
			source += nativeMemberConstructorRegistration(p, plan.child.object.GetClassName(), desc)
		}
		if diamond {
			return nativeMemberErasedAllocation(p, plan.child, source)
		}
		return source, true
	}
	if child := c.nativeMemberCurrent; child != nil && child.static {
		ctx.SourceMemberDelegation = func(owner, desc string, pc int, args []any) (string, bool) {
			return nativeRootBridgeSourceDelegation(p, c.obj, ctx, binding, owner, desc, pc, args)
		}
	}
	if child := c.nativeMemberCurrent; child != nil && !child.static {
		ctx.SourceMemberDelegation = func(owner, desc string, pc int, args []any) (string, bool) {
			// A nonstatic caller may invoke a private root/static constructor.
			// Its own enclosing capture was separately proved and regenerated;
			// the target packet drops only the original unused marker.
			if source, known := nativeRootBridgeSourceDelegation(p, c.obj, ctx, binding, owner, desc, pc, args); known {
				return source, true
			}
			name := strings.ReplaceAll(owner, ".", "/")
			targetClass := p.allocationClass(name)
			if targetClass == nil || targetClass.static {
				return "", false
			}
			ctor := child.constructors[ctx.CurrentMethodDesc]
			keyword := "this"
			if name != child.object.GetClassName() {
				if ctor == nil || !ctor.projectedSuper || name != child.object.GetSupperClassName() {
					return "", false
				}
				keyword = "super"
			}
			target := nativeMemberConstructorForAllocation(targetClass, desc)
			if ctor == nil || target == nil || ctor.delegateOwner != name || ctor.delegatePC != pc || ctor.delegateDescriptor != desc || len(args) < 1 {
				p.failed = true
				return "", false
			}
			if p.children[name] == nil && !nativeMemberSourceEnclosingParameter(args[0], ctx, child.owner) {
				p.failed = true
				return "", false
			}
			// The closed constructor certificate above proves the original
			// invokespecial receiver and PC. Keep that witness after removing the
			// enclosing operand, so source overload selection can seal the same
			// target instead of inferring a narrower constructor from its arguments.
			receiver := values.NewJavaRef(nil, nil, types.NewJavaClass(ctx.ClassName))
			receiver.IsThis = true
			call := &values.FunctionCallExpression{ClassName: owner, FunctionName: "<init>", Descriptor: target.sourceDescriptor, Object: receiver, Kind: values.InvokeSpecial, IsSpecialInvoke: true, OriginPC: pc, HasOriginPC: true}
			if keyword == "super" && ctor.enclosingSuperPath != nil && !nativeMemberLexicalReadOperand(args[0], ctor.enclosingSuperPath, c.Work, ctx) {
				p.failed = true
				return "", false
			}
			operands := args[1:]
			if targetClass.accessBridges[desc] != nil {
				if keyword != "super" || !(ctor.enclosingSuperPath != nil && nativeMemberLexicalReadOperand(args[0], ctor.enclosingSuperPath, c.Work, ctx) || ctor.enclosingSuperPath == nil && nativeMemberSourceEnclosingParameter(args[0], ctx, child.owner)) || len(operands) == 0 || !nativeMemberBridgeSourceDummy(operands[len(operands)-1]) {
					p.failed = true
					return "", false
				}
				operands = operands[:len(operands)-1]
			}
			for _, arg := range operands {
				v, ok := arg.(values.JavaValue)
				if !ok {
					p.failed = true
					return "", false
				}
				call.Arguments = append(call.Arguments, v)
			}
			mt, e := types.ParseMethodDescriptor(target.sourceDescriptor)
			if e != nil {
				p.failed = true
				return "", false
			}
			call.FuncType = mt.FunctionType()
			delegationBinding := *ctx
			delegationBinding.InvocationMetadata = binding.InvocationMetadata
			delegationBinding.SiblingClassSig = binding.SiblingClassSig
			delegationBinding.CurrentMethodDesc = ctor.sourceDescriptor
			source := keyword + "(" + strings.Join(call.ArgumentStrings(&delegationBinding), ",") + ")"
			if targetClass.accessBridges[desc] != nil {
				// This proved nonstatic SUPER bridge accesses a private
				// constructor just like an owned allocation. Legacy javac
				// registers it after the argument subtree, before body field
				// accessors. Omitting the event loses a real symbol ordinal.
				source += nativeMemberConstructorRegistration(p, name, desc)
			}
			return source, true
		}
	}
}

func (c *ClassObjectDumper) prepareNativeMemberConstructor(code *CodeAttribute, body []statements.Statement, params []values.JavaValue, method *MemberInfo) ([]statements.Statement, *nativeMemberConstructor, error) {
	child := c.nativeMemberCurrent
	n, _ := c.obj.getUtf8(method.NameIndex)
	if child == nil || child.static || n != "<init>" {
		return body, nil, nil
	}
	desc, _ := c.obj.getUtf8(method.DescriptorIndex)
	ctor := child.constructors[desc]
	if ctor == nil || len(params) < 2 {
		return nil, nil, fmt.Errorf("unproved member constructor")
	}
	outer, ok := params[1].(*values.JavaRef)
	if !ok || outer.Id == nil || !outer.IsParam || outer.CustomValue != nil || outer.StackVar != nil {
		return nil, nil, fmt.Errorf("unproved enclosing parameter")
	}
	c.nativeConstructorEnclosing = outer
	if c.FuncCtx.LocalNames == nil {
		c.FuncCtx.LocalNames = map[*coreutils.VariableId]string{}
	}
	c.FuncCtx.LocalNames[outer.Id] = c.FuncCtx.ShortTypeName(strings.ReplaceAll(child.owner, "/", ".")) + ".this"
	if ctor.capturePC < 0 {
		return body, nil, nil
	}
	filtered := make([]statements.Statement, 0, len(body))
	found := false
	for _, st := range body {
		if a, ok := st.(*statements.AssignStatement); ok && a != nil {
			if f, ok := values.UnpackSoltValue(a.LeftValue).(*values.RefMember); ok && f != nil && f.Member == child.field {
				receiver, ok := values.UnpackSoltValue(f.Object).(*values.JavaRef)
				if !ok || receiver == nil || !receiver.IsThis || values.UnpackSoltValue(a.JavaValue) != outer || found || a.IsDeclare || a.ArrayMember != nil {
					return nil, nil, fmt.Errorf("member capture source mismatch")
				}
				found = true
				continue
			}
		}
		filtered = append(filtered, st)
	}
	if !found {
		return nil, nil, fmt.Errorf("missing member capture source")
	}
	return filtered, ctor, nil
}
func (c *ClassObjectDumper) nativeMemberSkipCheck(st statements.Statement) bool {
	p := c.nativeMemberRoot
	if p == nil {
		return false
	}
	expr, ok := st.(*statements.ExpressionStatement)
	if !ok || expr == nil {
		return false
	}
	call, ok := values.UnpackSoltValue(expr.Expression).(*values.FunctionCallExpression)
	if !ok || call == nil || !call.HasOriginPC {
		return false
	}
	return c.nativeMemberChecks[c.FuncCtx.FunctionName+c.FuncCtx.CurrentMethodDesc][call.OriginPC]
}
func (c *ClassObjectDumper) renderNativeMembers() ([]string, error) {
	c.nativeRenderedMemberNames = nil
	p := c.nativeMemberRoot
	if p == nil || c.obj.GetClassName() != p.owner && p.children[c.obj.GetClassName()] == nil {
		return nil, nil
	}
	names := make([]string, 0, len(p.children))
	for n, child := range p.children {
		if child.owner == c.obj.GetClassName() {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var out []string
	for _, name := range names {
		child := p.children[name]
		sub := NewClassObjectDumper(child.object)
		sub.options = c.options
		sub.Work = c.Work
		sub.foldSiblingResolver = c.foldSiblingResolver
		sub.declarationResolver = c.declarationResolver
		sub.nativeMemberRoot = p
		sub.nativeMemberCurrent = child
		sub.nativeAnonymousRoot = p.memberAnonymous[name]
		if !child.static {
			sub.nativeCaptureFields = map[string]string{child.field: c.FuncCtx.ShortTypeName(strings.ReplaceAll(child.owner, "/", ".")) + ".this"}
			var arguments []types.JavaType
			for _, formal := range c.FuncCtx.ClassTypeParams {
				arguments = append(arguments, types.NewJavaClass(formal))
			}
			if len(arguments) > 0 {
				sub.nativeCaptureTypes = map[string]types.JavaType{child.field: types.NewParameterizedType(strings.ReplaceAll(child.owner, "/", "."), arguments)}
			}
		}
		sub.nativeCapturedReads = map[string]map[int]string{}
		sub.nativeOuterContext = c.FuncCtx
		if !child.static {
			sub.nativeTypeParams = append([]string(nil), c.FuncCtx.TypeParams...)
		}
		for _, m := range child.object.Methods {
			n, _ := child.object.getUtf8(m.NameIndex)
			desc, _ := child.object.getUtf8(m.DescriptorIndex)
			key := n + desc
			sub.nativeCapturedReads[key] = map[int]string{}
			for _, a := range m.Attributes {
				code, ok := a.(*CodeAttribute)
				if !ok {
					continue
				}
				if !nativeProofWork(c.Work, int64(len(code.Code))) {
					return nil, fmt.Errorf("member code budget")
				}
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(child.object.ConstantPool, i) })
				d.Work = c.Work
				if d.ParseOpcode() != nil {
					return nil, fmt.Errorf("member code parse")
				}
				for _, op := range d.Opcodes() {
					if field := constructorMotionMember(child.object, op, core.OP_GETFIELD); field != nil && field.Name == name && field.Member == child.field {
						sub.nativeCapturedReads[key][int(op.CurrentOffset)] = child.field
					}
				}
			}
		}
		src, e := sub.DumpClass()
		if e != nil || sub.nativeCaptureFailed || !nativeMemberLambdaSourceClosed(child, sub, c.Work) || strings.Contains(src, DecompileStubMarker) || len(sub.constructorBoundaryHelpers) > 0 || len(sub.interfaceInitializerHelpers) > 0 || sub.privateNestOwnPlan != nil && len(sub.privateNestOwnPlan.bridges) != 0 {
			return nil, fmt.Errorf("member body unproved: %v", e)
		}
		if group := p.memberAnonymous[name]; group != nil && !group.completeOwnSource(src) {
			ordinals, _ := nativeAnonymousOrdinalsWithinOwner(src, group.owner)
			return nil, fmt.Errorf("member anonymous source layout unproved: %s failed=%v ordinals=%v children=%d", group.owner, group.failed, ordinals, len(group.children))
		}
		for _, method := range sub.dumpedMethodsSet {
			if method != nil && method.checkedEscape {
				return nil, fmt.Errorf("member requires enclosing checked escape helper")
			}
		}
		for _, imp := range javaExtractImports(src) {
			c.FuncCtx.Import(imp)
		}
		if !strings.HasPrefix(src, sub.nativeMemberUnitPrefix) {
			return nil, fmt.Errorf("member compilation unit prefix changed")
		}
		declaration := "\n" + src[len(sub.nativeMemberUnitPrefix):] + "\n"
		if layout := sub.nativeRegistrationScope; layout != nil {
			if !strings.HasPrefix(layout.source, sub.nativeMemberUnitPrefix) || layout.owner != name {
				return nil, fmt.Errorf("member registration boundary changed")
			}
			layout.source = layout.source[len(sub.nativeMemberUnitPrefix):]
			layout.declaration = declaration
			if p.registrationLayouts == nil {
				p.registrationLayouts = map[string]*nativeMemberRegistrationScope{}
			}
			p.registrationLayouts[name] = layout
		}
		out = append(out, declaration)
		c.nativeRenderedMemberNames = append(c.nativeRenderedMemberNames, name)
	}
	return out, nil
}

func nativeMemberParameterWidth(params []string) int {
	width := len(params)
	for _, p := range params {
		if p == "J" || p == "D" {
			width++
		}
	}
	return width
}

// Both javac lowerings consume the duplicated qualifier and discard a result.
// getClass is final on Object; this is the exact platform null-check protocol,
// not an arbitrary receiver method or a subclass-name heuristic.
func nativeMemberNullCheck(obj *ClassObject, op *core.OpCode) bool {
	if call := constructorMotionMember(obj, op, core.OP_INVOKESTATIC); call != nil {
		return call.Name == "java/util/Objects" && call.Member == "requireNonNull" && call.Description == "(Ljava/lang/Object;)Ljava/lang/Object;"
	}
	if call := constructorMotionMember(obj, op, core.OP_INVOKEVIRTUAL); call != nil {
		return call.Name == "java/lang/Object" && call.Member == "getClass" && call.Description == "()Ljava/lang/Class;"
	}
	return false
}

// Anonymous rows are references, not ownership declarations. A root NEW whose
// target has an unnamed InnerClasses row still requires its original enclosing
// identity and complete anonymous allocation proof. Missing bytes/attributes
// must not turn that target into an unrelated flat type during joint planning.
func nativeJointAnonymousAllocationsClosed(obj *ClassObject, anonymous *nativeAnonymousFamily, work *workbudget.Budget, members ...*nativeMemberFamily) bool {
	if obj == nil {
		return false
	}
	unnamed := map[string]bool{}
	for _, attr := range obj.Attributes {
		if table, ok := attr.(*InnerClassesAttribute); ok && table != nil {
			for _, row := range table.Classes {
				if row == nil {
					return false
				}
				if row.InnerNameIndex != 0 {
					continue
				}
				name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
				if !known {
					return false
				}
				unnamed[name] = true
			}
		}
	}
	if len(unnamed) == 0 {
		return true
	}
	for _, method := range obj.Methods {
		if method == nil {
			return false
		}
		for _, attr := range method.Attributes {
			if code, ok := attr.(*CodeAttribute); ok {
				if !nativeProofWork(work, int64(len(code.Code))) {
					return false
				}
				d := core.NewDecompiler(code.Code, func(int) values.JavaValue { return nil })
				d.Work = work
				if d.ParseOpcode() != nil {
					return false
				}
				for _, op := range d.Opcodes() {
					if op.Instr.OpCode != core.OP_NEW {
						continue
					}
					name, known := sourceBridgeClassName(obj, uint16(core.Convert2bytesToInt(op.Data)))
					if !known {
						return false
					}
					if unnamed[name] && (anonymous == nil || anonymous.children[name] == nil && anonymous.standalone[name] == nil) && !(len(members) == 1 && nativeEnumConstantAllocationOwned(members[0], obj, method, int(op.CurrentOffset), name)) {
						return false
					}
				}
			}
		}
	}
	return true
}

// A committed member plan binds the hidden constructor parameter by its
// original descriptor and capture proof, irrespective of the field's spelling.
// Signature parameters omit this one physical parameter even at deeper scopes.
func (c *ClassObjectDumper) nativeMemberConstructorHasEnclosingParameter(descriptor string) bool {
	child := c.nativeMemberCurrent
	if child == nil || child.static || c.nativeMemberRoot == nil || child.object != c.obj || c.nativeMemberRoot.children[c.obj.GetClassName()] != child {
		return false
	}
	ctor := child.constructors[descriptor]
	if ctor == nil || ctor.descriptor != descriptor || ctor.delegatePC < 0 || ctor.capturePC < 0 && ctor.delegateOwner != child.object.GetClassName() {
		return false
	}
	params, result, err := callbinding.Descriptor(descriptor)
	return err == nil && result == "V" && len(params) > 0 && params[0] == "L"+child.owner+";" && ctor.sourceDescriptor == "("+strings.Join(params[1:], "")+")V"
}
