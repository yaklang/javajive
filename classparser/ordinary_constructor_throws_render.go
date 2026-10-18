package javaclassparser

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

var ordinaryConstructorRenderSerial atomic.Uint64

// Activation is after the actual source parameters/names have settled, and
// immediately before the dumper's final body render. Preview String calls
// before this point observe nothing. Repeated String calls inside this phase
// produce alternatives; only a retained, lexically anchored final fragment is
// consumed by finish. The restoring closure runs on every error path.
func (s *ordinaryConstructorThrowsScope) begin(ctx *class_context.ClassContext, parameters []values.JavaValue) (func(), error) {
	if s == nil {
		return func() {}, nil
	}
	if s.active || s.finished || s.failed || ctx != s.c.FuncCtx || len(s.receipts) != 1 || ctx.SourceAllocationRender != nil || ctx.SourceConstructedInvocation != nil {
		return nil, fmt.Errorf("invalid ordinary constructor render lifecycle")
	}
	s.parameterNames = map[*values.JavaRef]string{}
	for _, parameter := range parameters {
		ref, ok := parameter.(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || !nativeProofWork(s.c.Work, 1) {
			return nil, fmt.Errorf("unknown ordinary constructor source parameter binding")
		}
		name := ref.String(ctx)
		if name == "" || class_context.SafeIdentifier(name) != name {
			return nil, fmt.Errorf("invalid ordinary constructor source parameter binding")
		}
		s.parameterNames[ref] = name
	}
	for _, r := range s.receipts {
		if !r.fresh() {
			return nil, fmt.Errorf("stale ordinary constructor throws proof")
		}
	}
	priorAllocation, priorInvocation := ctx.SourceAllocationRender, ctx.SourceConstructedInvocation
	var known bool
	s.sourceContext, known = ctx.SnapshotForSourceProof(s.c.Work)
	if !known {
		return nil, fmt.Errorf("ordinary constructor source context snapshot unavailable")
	}
	s.active = true
	ctx.SourceAllocationRender = s.observeAllocation
	ctx.SourceConstructedInvocation = s.observeInvocation
	return func() {
		ctx.SourceAllocationRender = priorAllocation
		ctx.SourceConstructedInvocation = priorInvocation
		s.active = false
	}, nil
}

func (s *ordinaryConstructorThrowsScope) currentFrame() *ordinaryConstructorRenderFrame {
	if s == nil || len(s.frames) == 0 {
		return nil
	}
	return s.frames[len(s.frames)-1]
}

func (s *ordinaryConstructorThrowsScope) namesFresh() bool {
	for ref, name := range s.parameterNames {
		if ref == nil || !nativeProofWork(s.c.Work, 1) || ref.String(s.c.FuncCtx) != name {
			return false
		}
	}
	return true
}

// Only the dumper's actual final statement publisher opens this window. A
// String call in analysis, a debug print, or an unrelated statement cannot
// create an emission record, even while the method's scope is installed.
func (s *ordinaryConstructorThrowsScope) renderStatement(statement statements.Statement, render func() string) (string, error) {
	if s == nil {
		return render(), nil
	}
	if statement != s.returnStatement {
		return render(), nil
	}
	if !s.active || s.finished || s.failed || s.emitting || s.returnEmissions != 0 {
		s.failed = true
		return "", fmt.Errorf("invalid ordinary constructor return publisher")
	}
	s.emitting = true
	s.returnEmissions++
	defer func() { s.emitting = false }()
	source := render()
	if s.failed || len(s.frames) != 0 || len(s.renders) == 0 {
		s.failed = true
		return "", fmt.Errorf("unproved ordinary constructor return invocation")
	}
	return source, nil
}

func (s *ordinaryConstructorThrowsScope) observeAllocation(view class_context.AllocationRenderView) (string, string) {
	if s == nil || !s.emitting {
		return "", ""
	}
	n, ok := view.Value.(*values.NewExpression)
	if !ok || n == nil || !s.active || s.finished || s.failed || !nativeProofWork(s.c.Work, 1) {
		s.failed = true
		return "", ""
	}
	if !view.End {
		if len(s.frames) >= 32 {
			s.failed = true
			return "", ""
		}
		frame := &ordinaryConstructorRenderFrame{value: n}
		if n.ConstructorCall != nil {
			frame.receipt = s.receipts[n.ConstructorCall.OriginPC]
		}
		if r := frame.receipt; r != nil && (r.allocation != n || r.call != n.ConstructorCall || !r.fresh() || !s.namesFresh()) {
			s.failed = true
			return "", ""
		}
		s.frames = append(s.frames, frame)
		return "", ""
	}
	frame := s.currentFrame()
	if frame == nil || frame.value != n {
		s.failed = true
		return "", ""
	}
	s.frames = s.frames[:len(s.frames)-1]
	r := frame.receipt
	if r == nil {
		return "", ""
	}
	if frame.viewCount != 1 || !r.fresh() || !s.namesFresh() || view.Source == "" || len(view.Source) > 65536 || len(s.renders) >= 64 || !nativeProofWork(s.c.Work, int64(len(view.Source))+1) || s.c.Work != nil && s.c.Work.CheckAlloc(int64(len(s.renders)+1)*int64(len(view.Source)+256)) != nil {
		s.failed = true
		return "", ""
	}
	serial := ordinaryConstructorRenderSerial.Add(1)
	start := fmt.Sprintf("/*jdec-ordinary-new:%d:%d:begin*/", serial, r.initPC)
	end := fmt.Sprintf("/*jdec-ordinary-new:%d:%d:end*/", serial, r.initPC)
	s.renders = append(s.renders, &ordinaryConstructorRenderRecord{receipt: r, source: view.Source, begin: start, end: end})
	return start, end
}

func (s *ordinaryConstructorThrowsScope) observeInvocation(view class_context.ConstructedInvocationView) {
	if s == nil || !s.emitting {
		return
	}
	value, ctx := view.Invocation, view.Context
	frame := s.currentFrame()
	if frame == nil || frame.receipt == nil {
		return
	}
	r := frame.receipt
	call, ok := value.(*values.FunctionCallExpression)
	if !ok || call == nil || !call.HasOriginPC || call.OriginPC != r.initPC {
		return
	}
	if view.Owner != r.call.ClassName || view.PhysicalDescriptor != r.descriptor || view.NewPC != r.newPC || view.InitPC != r.initPC || r.plan == nil && view.SourcePlan != nil || r.plan != nil && view.SourcePlan != r.plan || r.plan == nil && view.Allocation != r.allocation || r.plan != nil && view.Allocation != nil || r.roleChild != nil && r.roleChild.formalCount > 0 && !view.Parameterized || !s.active || s.finished || s.failed || frame.viewCount != 0 || call.FunctionName != "<init>" || call.Kind != values.InvokeSpecial || !call.IsSpecialInvoke || call.IsStatic || strings.ReplaceAll(call.ClassName, ".", "/") != r.owner || call.Descriptor != r.sourceDescriptor || ctx == nil || ctx.FunctionName != s.name || ctx.CurrentMethodDesc != s.descriptor || !r.fresh() {
		s.failed = true
		return
	}
	if r.plan == nil {
		if call != r.call || call.Object != r.allocation {
			s.failed = true
			return
		}
	} else {
		if call == r.call || call.Object != nil {
			s.failed = true
			return
		}
		// The original named-member callback drops only the witnessed enclosing
		// port and (for a private bridge) its proved null marker. This domain
		// excludes lexical operand rebinding, fresh enclosing and other roles.
		end := len(r.arguments)
		if r.bridge != nil {
			end--
		}
		if end < 1 || len(call.Arguments) != end-1 {
			s.failed = true
			return
		}
		for i, arg := range call.Arguments {
			if arg != r.arguments[i+1] {
				s.failed = true
				return
			}
		}
	}
	_, scopes, known := r.sourceView(true)
	if !known {
		s.failed = true
		return
	}
	analysis, metadata := s.c.constructorThrowsProofContext()
	metadata.provider = ctx.InvocationMetadata
	exceptions, known := ordinaryConstructorThrowsOnly(analysis.FuncCtx.InvocationMetadata, r.owner, r.sourceDescriptor, scopes, s.c.Work)
	if !known || !slices.Equal(exceptions, r.exceptions) {
		s.failed = true
		return
	}
	// Repeat the same hierarchy queries whose answers entered publication.
	for _, exception := range exceptions {
		if analysis.constructorThrowsUnchecked(exception) {
			continue
		}
		concrete, known := s.c.constructorDelegationSourceThrows(s.method, s.declared)
		if !known {
			s.failed = true
			return
		}
		covered := false
		for _, allowed := range concrete {
			if !nativeProofWork(s.c.Work, 1) {
				s.failed = true
				return
			}
			covered = covered || originalExceptionCovered(exception, allowed, analysis.FuncCtx.InvocationMetadata)
		}
		if !covered {
			s.failed = true
			return
		}
	}
	if !constructorThrowsMetadataEqual(r.metadata, metadata.hashes) {
		s.failed = true
		return
	}
	frame.call = call
	frame.viewCount = 1
}

// Finish sees only this method's final body, after all its source rewrites.
// The typed render record establishes invocation identity; Java lexical state
// and a top-level return-expression range establish that this exact record was
// retained. Text never creates a type or overload proof. This small domain is
// Java 8, one terminal direct NEW return, with no Unicode-escape preprocessing
// or text blocks. scanAdvance is shared with the existing Java source scanner.
func (s *ordinaryConstructorThrowsScope) finish(ctx *class_context.ClassContext, source string) (string, error) {
	if s == nil {
		return source, nil
	}
	fail := func() (string, error) {
		s.failed = true
		return "", fmt.Errorf("unproved final ordinary constructor throws source emission")
	}
	if !s.active || s.emitting || s.returnEmissions != 1 || s.finished || s.failed || ctx != s.c.FuncCtx || len(s.frames) != 0 || len(s.receipts) != 1 || len(source) > 1<<20 || !s.namesFresh() || strings.Contains(source, `\u`) || strings.Contains(source, `"""`) || !nativeProofWork(s.c.Work, int64(len(source))+1) || s.c.Work != nil && s.c.Work.CheckAlloc(int64(len(source))*2+int64(len(s.renders))*256) != nil {
		return fail()
	}
	for _, r := range s.receipts {
		if !r.fresh() {
			return fail()
		}
	}
	starts, ends := map[string]*ordinaryConstructorRenderRecord{}, map[string]*ordinaryConstructorRenderRecord{}
	for _, r := range s.renders {
		if !nativeProofWork(s.c.Work, 1) {
			return fail()
		}
		starts[r.begin] = r
		ends[r.end] = r
	}
	const prefix = "/*jdec-ordinary-new:"
	state, ignored := scanNormal, 0
	brace, paren, bracket, returns := 0, 0, 0, 0
	lastToken := ""
	var pending *ordinaryConstructorRenderRecord
	beginAt, bodyAt, from := 0, 0, 0
	seen := map[*ordinaryConstructorThrowsReceipt]bool{}
	var output strings.Builder
	for i := 0; i < len(source); i++ {
		if !nativeProofWork(s.c.Work, 1) {
			return fail()
		}
		if strings.HasPrefix(source[i:], prefix) {
			if state != scanNormal {
				return fail()
			}
			end := strings.Index(source[i:], "*/")
			if end < 0 || end > 256 {
				return fail()
			}
			token := source[i : i+end+2]
			if record := starts[token]; record != nil {
				if pending != nil || seen[record.receipt] || brace != 0 || paren != 0 || bracket != 0 || lastToken != "return" {
					return fail()
				}
				pending, beginAt, bodyAt = record, i, i+len(token)
				output.WriteString(source[from:i])
				from = bodyAt
			} else if record := ends[token]; record != nil {
				if pending != record || i < bodyAt || source[bodyAt:i] != record.source || brace != 0 || paren != 0 || bracket != 0 {
					return fail()
				}
				next := i + len(token)
				for next < len(source) && strings.ContainsRune(" \t\r\n\f", rune(source[next])) {
					next++
				}
				if next >= len(source) || source[next] != ';' || beginAt >= bodyAt {
					return fail()
				}
				output.WriteString(source[from:i])
				from = i + len(token)
				seen[record.receipt] = true
				pending = nil
			} else {
				return fail()
			}
			i += len(token) - 1
			continue
		}
		if state != scanNormal {
			state = scanAdvance(source, &i, state, &ignored)
			continue
		}
		if strings.HasPrefix(source[i:], "//") || strings.HasPrefix(source[i:], "/*") {
			state = scanAdvance(source, &i, state, &ignored)
			continue
		}
		if source[i] == '"' || source[i] == '\'' {
			lastToken = "literal"
			state = scanAdvance(source, &i, state, &ignored)
			continue
		}
		if source[i] >= 'a' && source[i] <= 'z' || source[i] >= 'A' && source[i] <= 'Z' || source[i] == '_' || source[i] == '$' {
			start := i
			for i+1 < len(source) {
				b := source[i+1]
				if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '$') {
					break
				}
				i++
			}
			lastToken = source[start : i+1]
			if pending == nil {
				if lastToken == "return" {
					returns++
				}
				switch lastToken {
				case "if", "else", "for", "while", "do", "switch", "try", "catch", "finally", "synchronized", "class":
					return fail()
				}
			}
			continue
		}
		switch source[i] {
		case '{':
			brace++
		case '}':
			brace--
		case '(':
			paren++
		case ')':
			paren--
		case '[':
			bracket++
		case ']':
			bracket--
		}
		if brace < 0 || paren < 0 || bracket < 0 {
			return fail()
		}
		if !strings.ContainsRune(" \t\r\n\f", rune(source[i])) {
			lastToken = source[i : i+1]
		}
	}
	if state != scanNormal && state != scanLineComment || pending != nil || brace != 0 || paren != 0 || bracket != 0 || returns != 1 || len(seen) != len(s.receipts) {
		return fail()
	}
	output.WriteString(source[from:])
	s.finished = true
	s.sourceBody = output.String()
	return s.sourceBody, nil
}
