package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

type serializableReferencePacket struct {
	plan  *serializableReferenceProtocol
	index int
}

type serializableReferenceMethodGraph struct {
	plan          *serializableReferenceProtocol
	method        *MemberInfo
	code          *CodeAttribute
	counts        map[*serializableReferencePacket]int
	renderedBody  string
	publishedCode string
}

// A direct serialized handle must name the actual ordinary declaring member.
// Merely finding an inherited method is insufficient: javac could emit the
// ancestor's handle, changing the SerializedLambda implementation identity.
// This is distinct from the local synthetic-body ownership query used by arrows.
func (c *ClassObjectDumper) serializableReferenceOriginalTarget(target nativeLambdaHandleTarget) bool {
	if c == nil || c.obj == nil || target.name == "<init>" || target.name == "<clinit>" {
		return false
	}
	object, known := c.resolveBridgeClass(target.owner)
	if !known || object == nil || object.GetClassName() != target.owner || (object.AccessFlags&0x0200 != 0) != target.interfaceRef || len(object.Methods) > 4096 || !nativeProofWork(c.Work, int64(len(object.Methods))+1) {
		return false
	}
	if object != c.obj {
		// The first external domain consists of actual top-level declarations.
		// Public inner flags alone do not prove enclosing-type accessibility.
		for _, attribute := range object.Attributes {
			if table, ok := attribute.(*InnerClassesAttribute); ok {
				if table == nil || len(table.Classes) > 4096 {
					return false
				}
				for _, row := range table.Classes {
					if row == nil || !nativeProofWork(c.Work, 1) {
						return false
					}
					owner, known := sourceBridgeClassName(object, row.InnerClassInfoIndex)
					if !known || owner == target.owner {
						return false
					}
				}
			}
		}
	}
	matches := 0
	for _, method := range object.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		name, named := sourceBridgeUTF8(object, method.NameIndex)
		descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !named || !typed {
			return false
		}
		if name != target.name || descriptor != target.descriptor {
			continue
		}
		if method.AccessFlags&8 == 0 || method.AccessFlags&(0x1000|0x0040|0x0400) != 0 {
			return false
		}
		matches++
	}
	return matches == 1
}

func (c *ClassObjectDumper) prepareSerializableReferenceProtocol() {
	if c == nil || c.serializableReferenceReady {
		return
	}
	c.serializableReferenceReady = true
	if c.obj == nil || core.EffectiveTargetSource(c.options.TargetSourceVersion, c.obj.MajorVersion) != 8 || (c.options.SourceCompiler != "" && c.options.SourceCompiler != ModernJavac && c.options.SourceCompiler != NativeJavac8) {
		return
	}
	hasHelper := false
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return
		}
		name, known := sourceBridgeUTF8(c.obj, method.NameIndex)
		if known && name == "$deserializeLambda$" {
			hasHelper = true
		}
	}
	if !hasHelper {
		return
	}
	plan := buildSerializableReferenceProtocol(c.obj, c.Work)
	if plan == nil || !serializableCapturingNameWritable(plan.owner) || !c.serializableOtherFactoriesClosed(plan) {
		return
	}
	for _, site := range plan.sites {
		impl := site.tuple.implementation
		if !c.serializableReferenceOriginalTarget(impl) {
			return
		}
	}
	c.serializableReferencePlan = plan
	c.serializableReferencePackets = map[int]*serializableReferencePacket{}
}

func serializableCapturingNameWritable(owner string) bool {
	if owner == "" || len(owner) > 65535 {
		return false
	}
	// Source-safe renaming is not a valid transformation of a serialized
	// capturing class. Check both package and binary simple name with the
	// very same identifier policy used by the concrete declaration writer.
	for _, component := range strings.Split(owner, "/") {
		if component == "" || class_context.SafeIdentifier(component) != component {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) serializableReferenceFactorySource(method *MemberInfo, code *CodeAttribute, request core.CallSiteRequest) (core.SerializableReferenceSource, bool) {
	var none core.SerializableReferenceSource
	c.prepareSerializableReferenceProtocol()
	plan := c.serializableReferencePlan
	identity := core.BootstrapIdentity{Owner: "java.lang.invoke.LambdaMetafactory", Name: "altMetafactory", Descriptor: "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;", RefKind: 6}
	if plan == nil || plan.object != c.obj || c.FuncCtx == nil || c.FuncCtx.InvocationMetadata == nil || method == nil || code == nil || !request.Identity.Equal(identity) || len(request.DynamicArgs) != 0 || request.ClassMajor != c.obj.MajorVersion || core.EffectiveTargetSource(request.TargetSourceVersion, request.ClassMajor) != 8 || !nativeProofWork(c.Work, int64(len(plan.sites))+1) {
		return none, false
	}
	index := -1
	for i, site := range plan.sites {
		if site.method == method && site.code == code && site.pc == request.OriginPC && site.tuple.name == request.CallSiteName && site.tuple.descriptor == request.CallSiteDescriptor {
			if index != -1 {
				return none, false
			}
			index = i
		}
	}
	if index < 0 || c.Work != nil && c.Work.CheckAlloc(512) != nil {
		return none, false
	}
	packet := c.serializableReferencePackets[index]
	if packet == nil {
		packet = &serializableReferencePacket{plan: plan, index: index}
		c.serializableReferencePackets[index] = packet
	}
	if packet.plan != plan || packet.index != index {
		return none, false
	}
	impl := plan.sites[index].tuple.implementation
	target := callbinding.Witness{Owner: impl.owner, Name: impl.name, Desc: impl.descriptor, Kind: callbinding.Static, PC: plan.sites[index].pc}
	return core.SerializableReferenceSource{Metadata: c.compilerReferenceAccessProvider(c.FuncCtx.InvocationMetadata), Target: target, Retain: func(value *values.CustomValue) (*values.CustomValue, bool) {
		if c.serializableReferencePlan != plan || packet.plan.object != c.obj || packet.index != index || value == nil || value.WriteFunc == nil || value.InstantiatedMtdDesc != plan.sites[index].tuple.instantiated || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		emission, recorded := c.serializableReferenceEmission(packet)
		if !recorded || emission.attempts >= 16 {
			return nil, false
		}
		pc, captures, known := value.OriginalLambdaFactoryEvaluation()
		if !known || pc != plan.sites[index].pc || len(captures) != 0 {
			return nil, false
		}
		retained := values.NewSerializableReferenceSource(value, packet)
		if retained == nil {
			return nil, false
		}
		emission.attempts++
		write := retained.WriteFunc
		retained.WriteFunc = func(context *class_context.ClassContext, out *workbudget.Writer) error {
			start := out.Len()
			if err := write(context, out); err != nil {
				return err
			}
			return c.recordSerializableReferenceEmission(packet, out.String()[start:])
		}
		return retained, true
	}}, true
}

func (c *ClassObjectDumper) recordSerializableReferenceGraph(body *dumpedMethods, method *MemberInfo, code *CodeAttribute, graph []statements.Statement, rendered string) bool {
	plan := c.serializableReferencePlan
	if plan == nil {
		return true
	}
	required := false
	for _, site := range plan.sites {
		if site.method == method {
			required = true
			if site.code != code {
				return false
			}
		}
	}
	if !required {
		return true
	}
	if body == nil || body.sourceRequest.role != methodSourceDeclaration || rendered == "" || len(rendered) > 16<<20 || len(graph) > 65536 || !nativeProofWork(c.Work, int64(len(graph))+1) {
		return false
	}
	sealed := &serializableReferenceMethodGraph{plan: plan, method: method, code: code, renderedBody: rendered}
	counts, known := c.serializableReferenceGraphCounts(sealed, graph)
	if !known {
		return false
	}
	sealed.counts = counts
	body.serializableReferences = sealed
	return true
}

func (c *ClassObjectDumper) closeSerializableReferenceSource(source string, bodies []*dumpedMethods) bool {
	c.serializableReferenceSourceClosed = false
	plan := c.serializableReferencePlan
	if plan == nil {
		return true
	}
	if plan.object != c.obj || len(source) > 16<<20 || len(bodies) > 4096 || !nativeProofWork(c.Work, int64(len(source))+int64(len(bodies))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(bodies))*64+256) != nil {
		return false
	}
	// Reopen executable protocol authority at publication. Cached positive
	// metadata cannot authorize changed original code or a different object.
	reopened := buildSerializableReferenceProtocol(c.obj, c.Work)
	if reopened == nil || reopened.owner != plan.owner || reopened.helper != plan.helper || len(reopened.sites) != len(plan.sites) || !serializableCapturingNameWritable(reopened.owner) || !c.serializableOtherFactoriesClosed(reopened) {
		return false
	}
	for i, site := range reopened.sites {
		if site != plan.sites[i] || !c.serializableReferenceOriginalTarget(site.tuple.implementation) {
			return false
		}
	}
	parents := map[*MemberInfo]*dumpedMethods{}
	for _, body := range bodies {
		if body == nil || body.member == nil {
			continue
		}
		if body.member == plan.helper {
			return false
		}
		if body.serializableReferences != nil {
			if parents[body.member] != nil {
				return false
			}
			parents[body.member] = body
		}
	}
	scanned := int64(0)
	charge := func(n int64) bool {
		if n < 0 || n > (128<<20)-scanned {
			return false
		}
		scanned += n
		return nativeProofWork(c.Work, n)
	}
	for method, parent := range parents {
		graph := parent.serializableReferences
		if graph.plan != plan || graph.method != method || parent.sourceDumper != c || parent.sourceObject != c.obj || parent.sourceRequest.role != methodSourceDeclaration || parent.sourceCacheKey == "" || c.dumpedMethodsSet[parent.sourceCacheKey] != parent || parent.sourceReconstructionFailed() || graph.renderedBody == "" || graph.renderedBody != parent.bodyCode || graph.publishedCode == "" || graph.publishedCode != parent.code || !charge(int64(len(source))+int64(len(parent.code))) || serializableJavaPacketCount(source, parent.code, true) != 1 {
			return false
		}
		packetCounts := map[serializableReferenceFingerprint]int{}
		required := 0
		for index, site := range plan.sites {
			if site.method != method {
				continue
			}
			packet := c.serializableReferencePackets[index]
			if graph.code != site.code || packet == nil || packet.plan != plan || packet.index != index || graph.counts[packet] != 1 {
				return false
			}
			required++
			emission := c.serializableReferenceEmissions[packet]
			if emission == nil || len(emission.fingerprints) == 0 || len(emission.fingerprints) > 16 {
				return false
			}
			selected, found := serializableReferenceFingerprint{}, false
			for _, fingerprint := range emission.fingerprints {
				count, known := serializableReferenceFingerprintCount(parent.code, fingerprint, charge)
				if !known {
					return false
				}
				if count > 0 {
					if found {
						return false
					}
					selected, found = fingerprint, true
				}
			}
			if !found {
				return false
			}
			packetCounts[selected]++
		}
		if len(graph.counts) != required {
			return false
		}
		for fingerprint, want := range packetCounts {
			if count, known := serializableReferenceFingerprintCount(parent.code, fingerprint, charge); !known || count != want {
				return false
			}
		}
	}
	for index, site := range plan.sites {
		if parents[site.method] == nil || c.serializableReferencePackets[index] == nil {
			return false
		}
	}
	c.serializableReferenceSourceClosed = true
	return true
}

func (c *ClassObjectDumper) serializableReferenceGraphCounts(graph *serializableReferenceMethodGraph, body []statements.Statement) (map[*serializableReferencePacket]int, bool) {
	counts := map[*serializableReferencePacket]int{}
	activeValues := map[values.JavaValue]bool{}
	activeStatements := map[statements.Statement]bool{}
	remaining := 65536
	step := func() bool { remaining--; return remaining >= 0 && nativeProofWork(c.Work, 1) }
	scratch := func() bool {
		return c.Work == nil || c.Work.CheckAlloc(int64(len(activeValues)+len(activeStatements)+len(counts)+1)*96+256) == nil
	}
	var value func(values.JavaValue) bool
	value = func(v values.JavaValue) bool {
		if sourceProofNil(v) {
			return true
		}
		if !step() || activeValues[v] || !scratch() {
			return false
		}
		activeValues[v] = true
		defer delete(activeValues, v)
		if custom, ok := v.(*values.CustomValue); ok {
			if token, known := custom.SerializableReferenceSource(); known {
				packet, ok := token.(*serializableReferencePacket)
				if !ok || packet == nil || packet.plan != graph.plan || packet.index < 0 || packet.index >= len(graph.plan.sites) || c.serializableReferencePackets[packet.index] != packet {
					return false
				}
				site := graph.plan.sites[packet.index]
				if site.method != graph.method || site.code != graph.code {
					return false
				}
				counts[packet]++
				if counts[packet] > 1 {
					return false
				}
			}
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !value(child) {
				return false
			}
		}
		return true
	}
	var walk func([]statements.Statement) bool
	walk = func(body []statements.Statement) bool {
		for _, statement := range body {
			if sourceProofNil(statement) || !step() || activeStatements[statement] || !scratch() {
				return false
			}
			activeStatements[statement] = true
			roots, children, known := nativeSourceNameChildren(statement)
			if !known {
				return false
			}
			for _, root := range roots {
				if !value(root) {
					return false
				}
			}
			for _, child := range children {
				if !walk(child) {
					return false
				}
			}
			delete(activeStatements, statement)
		}
		return true
	}
	return counts, walk(body)
}

// Exact writer packets are only located in Java code, after original graph
// ownership is established. A quoted spelling or a nested member cannot stand
// in for the capturing class's actual retained method declaration.
func serializableJavaPacketCount(source, packet string, ownMethod bool) int {
	// This compiler certificate is for source 8. A text block cannot occur
	// in its emitted code, and must never offer a quoted packet as evidence.
	if packet == "" || len(source) > 16<<20 || strings.Contains(source, `"""`) {
		return 0
	}
	state, depth, count := scanNormal, 0, 0
	start, end := 0, len(source)
	if ownMethod {
		open := javaIndexTopBrace(source)
		if open < 0 {
			return 0
		}
		close := javaMatchBrace(source, open)
		if close < 0 {
			return 0
		}
		start, end, depth = open+1, close, 1
	}
	for i := start; i < end; i++ {
		state = scanAdvance(source, &i, state, &depth)
		if state != scanNormal || i >= end {
			continue
		}
		// scanAdvance handles lexical quoting, not brace nesting. The
		// caller must maintain the actual owning class's brace depth.
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
		if (!ownMethod || depth == 1) && strings.HasPrefix(source[i:], packet) {
			count++
			i += len(packet) - 1
		}
	}
	return count
}
