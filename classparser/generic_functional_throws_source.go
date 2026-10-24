package javaclassparser

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// An invocation receipt changes only a source view at an existing materialized
// lambda use. It never narrows a factory/helper descriptor, evaluates an operand
// twice, moves a lambda or copies executable bytecode.
type genericFunctionalThrowsScope struct {
	c                                  *ClassObjectDumper
	method                             *MemberInfo
	code                               *CodeAttribute
	name, descriptor                   string
	refName                            string
	body                               []statements.Statement
	ret                                *statements.ReturnStatement
	call                               *values.FunctionCallExpression
	argument                           int
	ref                                *values.JavaRef
	factory                            *values.CustomValue
	definition                         *statements.AssignStatement
	factoryPC                          int
	target                             types.JavaType
	receiverTarget                     types.JavaType
	receiver                           values.JavaValue
	receiversRendered                  int
	failure                            string
	exception                          string
	codeHash                           [32]byte
	inputHash                          [32]byte
	sealed                             bool
	metadata                           map[string][32]byte
	arguments                          []values.JavaValue
	active, emitting, finished, failed bool
	emissions, argumentsRendered       int
	renders                            []genericFunctionalRender
	definitionRender                   genericFunctionalRender
	definitionEmissions                int
	sourceContext                      *class_context.ClassContext
	sourceBody, publishedCode          string
}

type genericFunctionalRender struct{ begin, end, source string }

var genericFunctionalRenderSerial atomic.Uint64

func genericFunctionalUnwrap(value values.JavaValue, erasure string, ctx *class_context.ClassContext) values.JavaValue {
	for depth := 0; depth < 16; depth++ {
		switch v := value.(type) {
		case *values.SlotValue:
			if v == nil {
				return nil
			}
			value = v.GetValue()
		case *values.CastExpression:
			if v == nil {
				return nil
			}
			d, ok := values.SourceTypeErasure(v.TargetType, ctx)
			if !ok || d != erasure {
				return nil
			}
			value = v.Value
		default:
			return value
		}
	}
	return nil
}

func (c *ClassObjectDumper) genericFunctionalThrowsScope(code *CodeAttribute, body []statements.Statement, method *MemberInfo) *genericFunctionalThrowsScope {
	if c == nil {
		return nil
	}
	remaining := 128
	var visit func([]statements.Statement, int) (*genericFunctionalThrowsScope, bool)
	visit = func(block []statements.Statement, depth int) (*genericFunctionalThrowsScope, bool) {
		remaining -= len(block) + 1
		if remaining < 0 || depth > 8 || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if len(block) == 0 {
			return nil, true
		}
		if branch, ok := block[len(block)-1].(*statements.IfStatement); ok && branch != nil {
			left, lk := visit(branch.IfBody, depth+1)
			right, rk := visit(branch.ElseBody, depth+1)
			if !lk || !rk || left != nil && right != nil {
				return nil, false
			}
			if left != nil {
				return left, true
			}
			return right, true
		}
		return c.genericFunctionalThrowsLeaf(code, block, method), true
	}
	scope, known := visit(body, 0)
	if !known {
		return nil
	}
	return scope
}

func (c *ClassObjectDumper) genericFunctionalThrowsLeaf(code *CodeAttribute, body []statements.Statement, method *MemberInfo) *genericFunctionalThrowsScope {
	if c == nil || c.obj == nil || c.FuncCtx == nil || method == nil || code == nil || c.getenv("JDEC_GENERIC_FUNCTIONAL_THROWS_OFF") != "" || len(body) == 0 || len(body) > 128 || len(code.Code) > 4096 || len(code.ExceptionTable) != 0 || core.EffectiveTargetSource(c.options.TargetSourceVersion, c.obj.MajorVersion) < 8 || !nativeProofWork(c.Work, 1) {
		return nil
	}
	name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !nk || !dk || strings.HasPrefix(name, "<") || method.AccessFlags&0x1040 != 0 || c.FuncCtx.FunctionName != name || c.FuncCtx.CurrentMethodDesc != desc {
		return nil
	}
	ret, ok := body[len(body)-1].(*statements.ReturnStatement)
	if !ok || ret == nil {
		return nil
	}
	for _, st := range body[:len(body)-1] {
		switch st.(type) {
		case *statements.AssignStatement, *statements.ExpressionStatement, *statements.SourceAnchorStatement, *statements.IfStatement:
		default:
			return nil
		}
	}
	var calls []*values.FunctionCallExpression
	remaining := 512
	var walk func(values.JavaValue) bool
	walk = func(v values.JavaValue) bool {
		remaining--
		if remaining < 0 || !nativeProofWork(c.Work, 1) {
			return false
		}
		if v == nil {
			return true
		}
		if call, ok := v.(*values.FunctionCallExpression); ok && call != nil && call.HasOriginPC {
			calls = append(calls, call)
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !walk(child) {
				return false
			}
		}
		return true
	}
	if !walk(ret.JavaValue) {
		return nil
	}
	var selected *genericFunctionalThrowsScope
	for _, call := range calls {
		if call.IsSpecialInvoke || call.Kind == values.InvokeDynamic || strings.HasPrefix(call.FunctionName, "<") {
			continue
		}
		ps, _, err := callbinding.Descriptor(call.Descriptor)
		if err != nil || len(ps) != len(call.Arguments) {
			continue
		}
		for index, arg := range call.Arguments {
			ref, ok := genericFunctionalUnwrap(arg, ps[index], c.FuncCtx).(*values.JavaRef)
			if !ok || ref == nil || ref.IsParam || ref.IsThis || ref.Id == nil {
				continue
			}
			factory, ok := genericFunctionalUnwrap(ref.Val, ps[index], c.FuncCtx).(*values.CustomValue)
			if !ok || factory == nil {
				continue
			}
			pc, _, ok := factory.OriginalLambdaFactoryEvaluation()
			if !ok {
				continue
			}
			s := &genericFunctionalThrowsScope{c: c, method: method, code: code, name: name, descriptor: desc, body: slices.Clone(body), ret: ret, call: call, receiver: call.Object, argument: index, ref: ref, factory: factory, factoryPC: pc, codeHash: sha256.Sum256(code.Code), arguments: slices.Clone(call.Arguments)}
			view, exception, metadata, known := s.prove()
			if !known {
				continue
			}
			if selected != nil {
				return nil
			}
			s.target, s.exception, s.metadata = view, exception, metadata
			s.sealed = true
			selected = s
		}
	}
	return selected
}

func (s *genericFunctionalThrowsScope) prove() (types.JavaType, string, map[string][32]byte, bool) {
	bad := func() (types.JavaType, string, map[string][32]byte, bool) { return nil, "", nil, false }
	if s == nil || s.c == nil || s.c.FuncCtx == nil || s.c.obj == nil || s.method == nil || s.call == nil || s.code == nil || s.ref == nil || s.factory == nil || s.ret == nil || len(s.body) == 0 || len(s.body) > 128 || s.argument < 0 || s.argument >= len(s.arguments) || len(s.code.Code) > 4096 || len(s.code.ExceptionTable) != 0 || sha256.Sum256(s.code.Code) != s.codeHash || !s.call.HasOriginPC || s.call.OriginPC < 0 || len(s.arguments) != len(s.call.Arguments) || s.c.FuncCtx.FunctionName != s.name || s.c.FuncCtx.CurrentMethodDesc != s.descriptor || !nativeProofWork(s.c.Work, 1) {
		return bad()
	}
	for i, arg := range s.arguments {
		if arg != s.call.Arguments[i] {
			return bad()
		}
	}
	name, nk := sourceBridgeUTF8(s.c.obj, s.method.NameIndex)
	desc, dk := sourceBridgeUTF8(s.c.obj, s.method.DescriptorIndex)
	if !nk || !dk || name != s.name || desc != s.descriptor || s.method.AccessFlags&0x1040 != 0 {
		return bad()
	}
	members, codes := 0, 0
	for _, m := range s.c.obj.Methods {
		if !nativeProofWork(s.c.Work, 1) {
			return bad()
		}
		if m == s.method {
			members++
		}
	}
	for _, a := range s.method.Attributes {
		if x, ok := a.(*CodeAttribute); ok {
			if x != s.code {
				return bad()
			}
			codes++
		}
	}
	if members != 1 || codes != 1 {
		return bad()
	}
	ir, fn, known := s.c.nativeOriginalMethodSnapshot(s.method, s.code)
	if !known || len(ir.Instrs) > 2048 || len(fn.Instructions) > 2048 {
		return bad()
	}
	var use ssabuild.InstructionValues
	matches := 0
	for _, record := range fn.Instructions {
		if int(record.PC) == s.call.OriginPC {
			use = record
			matches++
		}
	}
	if matches != 1 {
		return bad()
	}
	port := s.argument
	opcode := core.OP_INVOKESTATIC
	if !s.call.IsStatic {
		port++
		opcode = core.OP_INVOKEVIRTUAL
		if s.call.Kind == values.InvokeInterface {
			opcode = core.OP_INVOKEINTERFACE
		}
	}
	if s.call.Object != s.receiver || s.call.IsSpecialInvoke || s.call.IsStatic != (s.call.Kind == values.InvokeStatic) || port >= len(use.Uses) || use.Uses[port].Kind != ssabuild.OriginInstr || int(use.Uses[port].PC) != s.factoryPC {
		return bad()
	}
	matched := false
	for _, ins := range ir.Instrs {
		if int(ins.PC) == s.call.OriginPC {
			matched = ins.Opcode == opcode && ins.Class == strings.ReplaceAll(s.call.ClassName, ".", "/") && ins.Member == s.call.FunctionName && ins.Desc == s.call.Descriptor
		}
	}
	if !matched {
		return bad()
	}
	factory := &lambdaSourceFactory{object: s.c.obj, method: s.method, code: s.code, pc: s.factoryPC}
	if !factory.seal(s.c.Work) {
		return bad()
	}
	site, known := s.c.originalCompilerReferenceSite(factory)
	if !known || site.captureCount != 0 {
		return bad()
	}
	// Freeze decoded instruction/CP identities as well as Code bytes. The
	// bootstrap packet is read again from typed CP edges on every admission.
	units := int64(len(ir.Instrs)) * 512
	for _, ins := range ir.Instrs {
		units += int64(len(ins.Data)+len(ins.Class)+len(ins.Member)+len(ins.Desc)+len(ins.Const.String)+len(ins.Const.Class)) * 8
	}
	if units > 2<<20 || !nativeProofWork(s.c.Work, units) || s.c.Work != nil && s.c.Work.CheckAlloc(units) != nil {
		return bad()
	}
	input, err := json.Marshal(struct {
		Instructions  any
		Stack, Locals uint16
		Site          []string
		FactoryPC     int
	}{ir.Instrs, s.code.MaxStack, s.code.MaxLocals, []string{site.samName, site.functional, site.erased, site.actual, site.implementation.Name, site.implementation.Member, site.implementation.Description}, s.factoryPC})
	if err != nil {
		return bad()
	}
	hash := sha256.Sum256(input)
	if s.sealed && hash != s.inputHash {
		return bad()
	}
	if !s.sealed {
		s.inputHash = hash
	}
	pc, captures, known := s.factory.OriginalLambdaFactoryEvaluation()
	if !known || pc != s.factoryPC || len(captures) != 0 || s.factory.InstantiatedMtdDesc != site.actual {
		return bad()
	}
	ps, _, err := callbinding.Descriptor(s.call.Descriptor)
	if err != nil || s.argument >= len(ps) || site.functional != ps[s.argument] || genericFunctionalUnwrap(s.call.Arguments[s.argument], site.functional, s.c.FuncCtx) != s.ref || genericFunctionalUnwrap(s.ref.Val, site.functional, s.c.FuncCtx) != s.factory {
		return bad()
	}
	erasure, known := values.SourceTypeErasure(s.ref.Type(), s.c.FuncCtx)
	if !known || erasure != site.functional {
		return bad()
	}
	// The AST use must still name the unique emitted declaration of this
	// sealed producer. SSA independently proves that exact factory reaches
	// the original call; mutable JavaRef.Val alone never grants permission.
	declarations := 0
	for _, st := range s.body[:len(s.body)-1] {
		if assignment, ok := st.(*statements.AssignStatement); ok {
			ref, ok := assignment.LeftValue.(*values.JavaRef)
			if ok && ref != nil && (ref == s.ref || ref.SharesSourceDeclaration(s.ref)) {
				declarations++
				if genericFunctionalUnwrap(assignment.JavaValue, site.functional, s.c.FuncCtx) != s.factory {
					return bad()
				}
				if s.definition != nil && s.definition != assignment {
					return bad()
				}
				s.definition = assignment
			}
		}
	}
	if declarations != 1 || s.body[len(s.body)-1] != s.ret {
		return bad()
	}
	_, eager := s.ref.OriginalLambdaResult(s.factory)
	if !eager {
		store, slot, local := s.ref.OriginalLocalDeclaration(s.ref.Val)
		ap, as, ak := s.definition.OriginalLocalStore()
		if !local || !ak || store != ap || slot != as {
			return bad()
		}
	}
	reader, metadata := s.c.constructorThrowsProofContext()
	view, receiver, exception, known := genericFunctionalThrowsReceiver(reader.FuncCtx.InvocationMetadata, strings.ReplaceAll(s.call.ClassName, ".", "/"), s.call.FunctionName, s.call.Descriptor, s.call.IsStatic, s.argument, site.samName, site.erased, site.actual, s.c.Work)
	if !known {
		return bad()
	}
	if receiver != nil {
		if s.call.IsStatic || s.receiver == nil {
			return bad()
		}
		erasure, valid := values.SourceTypeErasure(s.receiver.Type(), s.c.FuncCtx)
		if !valid || erasure != "L"+strings.ReplaceAll(s.call.ClassName, ".", "/")+";" {
			return bad()
		}
	}
	if !s.sealed {
		s.receiverTarget = receiver
	} else {
		if (receiver == nil) != (s.receiverTarget == nil) {
			return bad()
		}
		if receiver != nil && receiver.String(&class_context.ClassContext{}) != s.receiverTarget.String(&class_context.ClassContext{}) {
			return bad()
		}
	}
	return view, exception, metadata.hashes, true
}

func (s *genericFunctionalThrowsScope) fresh() bool {
	if s == nil || !s.sealed {
		return false
	}
	if s.refName != "" && (s.ref == nil || s.definition == nil || s.definition.LeftValue == nil || s.ref.String(s.c.FuncCtx) != s.refName || s.definition.LeftValue.String(s.c.FuncCtx) != s.refName) {
		return false
	}
	view, exception, metadata, known := s.prove()
	if !known || exception != s.exception || !constructorThrowsMetadataEqual(metadata, s.metadata) {
		return false
	}
	remaining, occurrences := 512, 0
	var walk func(values.JavaValue) bool
	walk = func(v values.JavaValue) bool {
		remaining--
		if remaining < 0 || !nativeProofWork(s.c.Work, 1) {
			return false
		}
		if v == nil {
			return true
		}
		if v == s.call {
			occurrences++
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !walk(child) {
				return false
			}
		}
		return true
	}
	if !walk(s.ret.JavaValue) || occurrences != 1 {
		return false
	}
	ctx := &class_context.ClassContext{}
	return view.String(ctx) == s.target.String(ctx)
}

func (s *genericFunctionalThrowsScope) begin(ctx *class_context.ClassContext) (func(), error) {
	if s == nil {
		return func() {}, nil
	}
	if s.active || s.finished || s.failed || ctx != s.c.FuncCtx || ctx.SourceGenericInvocation != nil || ctx.SourceGenericArgument != nil || ctx.SourceGenericReceiver != nil || ctx.SourceGenericInvocationBound != nil || !s.fresh() {
		return nil, fmt.Errorf("stale generic functional invocation")
	}
	s.refName = s.ref.String(ctx)
	if s.refName == "" || class_context.SafeIdentifier(s.refName) != s.refName {
		return nil, fmt.Errorf("invalid generic factory source binding")
	}
	var known bool
	s.sourceContext, known = ctx.SnapshotForSourceProof(s.c.Work)
	if !known {
		return nil, fmt.Errorf("generic invocation context budget")
	}
	s.active = true
	ctx.SourceGenericInvocationBound = func(value any) bool {
		if !s.emitting || value != s.call {
			return false
		}
		if !s.active || s.failed || !s.fresh() {
			s.failed = true
			s.failure = "generic binding evidence changed"
			return false
		}
		return true
	}
	ctx.SourceGenericArgument = func(value any, index int) (string, bool) {
		if !s.emitting || value != s.call || index != s.argument {
			return "", false
		}
		if !s.active || s.failed || !s.fresh() {
			s.failed = true
			s.failure = "argument evidence changed"
			return "", false
		}
		s.argumentsRendered++
		return "(" + s.target.String(ctx) + ")(" + s.ref.String(ctx) + ")", true
	}
	ctx.SourceGenericReceiver = func(value any) (string, bool) {
		if !s.emitting || value != s.call || s.receiverTarget == nil {
			return "", false
		}
		if !s.active || s.failed || !s.fresh() {
			s.failed = true
			s.failure = "receiver evidence changed"
			return "", false
		}
		s.receiversRendered++
		return s.receiverTarget.String(ctx), true
	}
	ctx.SourceGenericInvocation = func(value any, source string, end bool) string {
		if !s.emitting || value != s.call {
			return source
		}
		if !s.active || s.failed || !s.fresh() {
			s.failed = true
			s.failure = "invocation evidence changed"
			return source
		}
		if !end {
			s.argumentsRendered = 0
			s.receiversRendered = 0
			return source
		}
		if s.argumentsRendered != 1 || s.receiverTarget != nil && s.receiversRendered != 1 || s.receiverTarget == nil && s.receiversRendered != 0 || source == "" || len(source) > 65536 || len(s.renders) > 32 {
			s.failed = true
			s.failure = fmt.Sprintf("argument renders=%d receiver renders=%d source bytes=%d", s.argumentsRendered, s.receiversRendered, len(source))
			return source
		}
		serial := genericFunctionalRenderSerial.Add(1)
		r := genericFunctionalRender{begin: fmt.Sprintf("/*jdec-generic-call:%d:begin*/", serial), end: fmt.Sprintf("/*jdec-generic-call:%d:end*/", serial), source: source}
		s.renders = append(s.renders, r)
		return r.begin + source + r.end
	}
	return func() {
		ctx.SourceGenericArgument = nil
		ctx.SourceGenericInvocation = nil
		ctx.SourceGenericReceiver = nil
		ctx.SourceGenericInvocationBound = nil
		s.active = false
	}, nil
}

func (s *genericFunctionalThrowsScope) renderStatement(st statements.Statement, render func() string) (string, error) {
	if s == nil {
		return render(), nil
	}
	if st == s.definition {
		if !s.active || s.emitting || s.finished || s.failed || s.definitionEmissions != 0 || !s.fresh() {
			return "", fmt.Errorf("invalid functional factory declaration publisher")
		}
		source := render()
		if len(source) > 65536 || source == "" || !nativeProofWork(s.c.Work, int64(len(source))+1) {
			return "", fmt.Errorf("functional factory declaration budget")
		}
		serial := genericFunctionalRenderSerial.Add(1)
		s.definitionRender = genericFunctionalRender{begin: fmt.Sprintf("/*jdec-generic-def:%d:begin*/", serial), end: fmt.Sprintf("/*jdec-generic-def:%d:end*/", serial), source: source}
		s.definitionEmissions++
		return s.definitionRender.begin + source + s.definitionRender.end, nil
	}
	if st != s.ret {
		return render(), nil
	}
	if !s.active || s.emitting || s.finished || s.failed || s.emissions != 0 {
		return "", fmt.Errorf("invalid generic invocation publisher")
	}
	s.emitting = true
	s.emissions++
	defer func() { s.emitting = false }()
	out := render()
	if s.failed || len(s.renders) == 0 {
		return "", fmt.Errorf("generic invocation not rendered: %s (records=%d)", s.failure, len(s.renders))
	}
	return out, nil
}

func (s *genericFunctionalThrowsScope) finish(ctx *class_context.ClassContext, source string) (string, error) {
	if s == nil {
		return source, nil
	}
	fail := func() (string, error) {
		s.failed = true
		return "", fmt.Errorf("unproved generic invocation source publication")
	}
	if !s.active || s.emitting || s.finished || s.failed || s.emissions != 1 || s.definitionEmissions != 1 || ctx != s.c.FuncCtx || len(source) > 1<<20 || strings.Contains(source, `\u`) || strings.Contains(source, `"""`) || !s.fresh() || !nativeProofWork(s.c.Work, int64(len(source))+1) {
		return fail()
	}
	// A lexer, not a substring occurrence, establishes that the typed call
	// survived the actual return publisher and is not a comment/string decoy.
	state, ignored := scanNormal, 0
	brace := 0
	inReturn := false
	found := 0
	definitionFound := false
	definitionDepth := -1
	result := source
	for i := 0; i < len(source); i++ {
		if strings.HasPrefix(source[i:], "/*jdec-generic-def:") {
			r := s.definitionRender
			packet := r.begin + r.source + r.end
			if state != scanNormal || brace < 0 || inReturn || found != 0 || definitionFound || !strings.HasPrefix(source[i:], packet) || !ordinaryConstructorBodyScopeClosed(r.source) {
				return fail()
			}
			result = strings.Replace(result, packet, r.source, 1)
			i += len(packet) - 1
			definitionFound = true
			definitionDepth = brace
			continue
		}
		if state == scanNormal && strings.HasPrefix(source[i:], "/*jdec-generic-call:") {
			if brace != definitionDepth || !inReturn || !definitionFound {
				return fail()
			}
			matched := false
			for _, r := range s.renders {
				packet := r.begin + r.source + r.end
				if strings.HasPrefix(source[i:], packet) {
					if !ordinaryConstructorBodyScopeClosed(r.source) {
						return fail()
					}
					result = strings.Replace(result, packet, r.source, 1)
					i += len(packet) - 1
					found++
					matched = true
					break
				}
			}
			if !matched || found != 1 {
				return fail()
			}
			continue
		}
		if state == scanNormal {
			if source[i] == '{' {
				brace++
			}
			if source[i] == '}' {
				brace--
			}
			if source[i] == ';' {
				inReturn = false
			}
			if strings.HasPrefix(source[i:], "return ") {
				inReturn = true
			}
		}
		state = scanAdvance(source, &i, state, &ignored)
	}
	if found != 1 || !definitionFound || state != scanNormal {
		return fail()
	}
	s.finished = true
	s.sourceBody = result
	return result, nil
}
