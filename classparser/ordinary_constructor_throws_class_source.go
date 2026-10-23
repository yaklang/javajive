package javaclassparser

import (
	"fmt"
	"strings"
)

// The two concrete writers supply these coordinates. Source spelling never
// discovers a method, a declaration boundary, or permission to edit one.
type ordinaryConstructorMethodSourceSpan struct {
	method *dumpedMethods
	start  int
}

type ordinaryConstructorClassSourceReceipt struct {
	c        *ClassObjectDumper
	current  string
	segments []string
	bodies   []string
	bound    []*dumpedMethods
	opaque   bool
}

type ordinaryConstructorClassSourceDraft struct {
	before, after string
	bodies        []string
}

// Class-level recovery gets body-only ports for unsealed ordinary methods.
// Imports, type/field declarations, method headers, constructors, initializers,
// and the entire sealed method stay in immutable segments. A constructor
// awaiting executable lowering is also sealed: its typed SUPER operands and
// capture schedule cannot be reinterpreted by a later source-text rule. An
// added field/type cannot capture a name in a previously proved NEW expression.
func (c *ClassObjectDumper) newOrdinaryConstructorClassSource(source string, methods []*dumpedMethods, spans []ordinaryConstructorMethodSourceSpan) (*ordinaryConstructorClassSourceReceipt, error) {
	s := &ordinaryConstructorClassSourceReceipt{c: c, current: source}
	if !nativeProofWork(c.Work, int64(len(methods))+1) {
		return nil, fmt.Errorf("ordinary constructor class-source inventory budget")
	}
	for _, method := range methods {
		if method != nil && (method.ordinaryConstructorSource != nil || method.constructorLowering != nil) {
			s.bound = append(s.bound, method)
		}
	}
	if len(s.bound) == 0 {
		return nil, nil
	}
	if len(spans) != len(methods) {
		return nil, fmt.Errorf("incomplete actual method publisher inventory")
	}
	if !s.closed() {
		return nil, fmt.Errorf("stale ordinary constructor source publication")
	}
	if len(source) > 1<<20 || c.Work != nil && c.Work.CheckAlloc(int64(len(source))*2+int64(len(spans))*128) != nil {
		s.opaque = true
		return s, nil
	}
	from, methodEnd := 0, 0
	for i, span := range spans {
		m := span.method
		if m == nil || m != methods[i] || span.start < methodEnd || span.start > len(source) || len(m.code) > len(source)-span.start || source[span.start:span.start+len(m.code)] != m.code || !nativeProofWork(c.Work, int64(len(m.code))+1) {
			return nil, fmt.Errorf("unproved actual class-source method extent")
		}
		methodEnd = span.start + len(m.code)
		if m.ordinaryConstructorSource != nil || m.constructorLowering != nil || !m.hasSourceBodyRange || strings.HasPrefix(m.methodName, "<") || m.sourceRequest.role != methodSourceDeclaration {
			continue
		}
		if m.sourceBodyStart < 0 || m.sourceBodyEnd < m.sourceBodyStart || m.sourceBodyEnd > len(m.code) || m.code[m.sourceBodyStart:m.sourceBodyEnd] != m.bodyCode {
			return nil, fmt.Errorf("stale declaration-writer body extent")
		}
		start, end := span.start+m.sourceBodyStart, span.start+m.sourceBodyEnd
		s.segments = append(s.segments, source[from:start])
		s.bodies = append(s.bodies, source[start:end])
		from = end
		// Unknown Java preprocessing grants no editable body port. The original
		// writer output remains intact; no text-based binding is manufactured.
		if !ordinaryConstructorBodyScopeClosed(source[start:end]) {
			s.opaque = true
		}
	}
	s.segments = append(s.segments, source[from:])
	return s, nil
}

// Recheck the original method/cache/SSA facts at the class phase boundaries,
// not once per legacy rule. Each retained context belongs to its own actual
// publisher. A private copy selects that context without mutating the dumper's
// current method or borrowing a sibling's parameters.
func (s *ordinaryConstructorClassSourceReceipt) closed() bool {
	if s == nil || s.c == nil {
		return false
	}
	for _, m := range s.bound {
		if m == nil || m.sourceDumper != s.c || m.sourceObject != s.c.obj || m.sourceRequest.role != methodSourceDeclaration || m.sourceCacheKey == "" || m.sourceReconstructionFailed() || !s.c.methodParameterNameCacheClosed(m.parameterNameStamp) {
			return false
		}
		cached := s.c.dumpedMethodsSet[m.sourceCacheKey]
		// A trusted retry clones cache containers. The publication identity is
		// the same consumed receipt and unchanged packet, not that container's
		// address. A changed role, body, header or original-name stamp is stale.
		if cached == nil || cached.sourceDumper != m.sourceDumper || cached.sourceObject != m.sourceObject || cached.sourceRequest != m.sourceRequest || cached.sourceCacheKey != m.sourceCacheKey || cached.parameterNameStamp != m.parameterNameStamp || cached.ordinaryConstructorSource != m.ordinaryConstructorSource || cached.code != m.code || cached.bodyCode != m.bodyCode || cached.methodName != m.methodName || cached.sourceBodyStart != m.sourceBodyStart || cached.sourceBodyEnd != m.sourceBodyEnd || cached.hasSourceBodyRange != m.hasSourceBodyRange || cached.checkedEscape != m.checkedEscape || cached.sourceReconstructionFailed() {
			return false
		}
		if lowering := m.constructorLowering; lowering != nil {
			if cached.constructorLowering != lowering || cached.constructorLoweringCode != m.constructorLoweringCode || m.constructorLoweringCode != m.code || m.methodName != "<init>" || m.descriptor != lowering.descriptor || s.c.constructorLowerings[lowering.descriptor] != lowering || lowering.owner != s.c.obj.GetClassName() {
				return false
			}
			// The executable packet was proved by the typed declaration writer.
			// Preserve that exact publication; FinishClass separately reopens
			// the original bytes and verifies the fresh constructor packet.
			if m.ordinaryConstructorSource == nil {
				continue
			}
		}
		p := m.ordinaryConstructorSource
		if p == nil || p.c != s.c || !p.finished || p.active || p.failed || p.sourceContext == nil || p.publishedCode != m.code || p.sourceBody != m.bodyCode || len(p.receipts) != 1 {
			return false
		}
		reader := *s.c
		reader.FuncCtx, reader.CurrentMethod = p.sourceContext, p.method
		proof := *p
		proof.c = &reader
		if !proof.namesFresh() {
			return false
		}
		for _, original := range p.receipts {
			if original == nil || original.scope != p {
				return false
			}
			r := *original
			r.scope = &proof
			if !r.fresh() {
				return false
			}
		}
	}
	return true
}

func (s *ordinaryConstructorClassSourceReceipt) rewrite(source string, rule func(string) string) (string, *ordinaryConstructorClassSourceDraft) {
	if s == nil || source != s.current || s.opaque || len(s.bodies) == 0 {
		return source, nil
	}
	var next []string
	total, changed := int64(len(source)), false
	for i, body := range s.bodies {
		if !nativeProofWork(s.c.Work, int64(len(body))+1) {
			return source, nil
		}
		after := rule(body)
		if after == body {
			continue
		}
		if len(after) > 1<<20 || !nativeProofWork(s.c.Work, int64(len(after))+1) || !ordinaryConstructorBodyScopeClosed(after) {
			return source, nil
		}
		total += int64(len(after)) - int64(len(body))
		if total > 1<<20 {
			return source, nil
		}
		if next == nil {
			if s.c.Work != nil && s.c.Work.CheckAlloc(int64(len(s.bodies))*16+int64(len(after))) != nil {
				return source, nil
			}
			next = append([]string(nil), s.bodies...)
		}
		next[i], changed = after, true
	}
	if !changed || s.c.Work != nil && s.c.Work.CheckAlloc(total+int64(len(next))*16) != nil {
		return source, nil
	}
	var out strings.Builder
	out.Grow(int(total))
	for i, body := range next {
		out.WriteString(s.segments[i])
		out.WriteString(body)
	}
	out.WriteString(s.segments[len(next)])
	result := out.String()
	return result, &ordinaryConstructorClassSourceDraft{before: source, after: result, bodies: next}
}

func (s *ordinaryConstructorClassSourceReceipt) commit(d *ordinaryConstructorClassSourceDraft) bool {
	if s == nil || d == nil || d.before != s.current || len(d.bodies) != len(s.bodies) {
		return false
	}
	s.current, s.bodies = d.after, d.bodies
	return true
}

func (s *ordinaryConstructorClassSourceReceipt) finish(source string) bool {
	return s != nil && source == s.current && s.closed()
}

// This checks only that a replacement cannot escape its writer-supplied body
// port. It never infers an invocation type or source permission. Unknown Unicode
// preprocessing/text blocks are rejected rather than guessed from raw bytes.
func ordinaryConstructorBodyScopeClosed(source string) bool {
	if strings.Contains(source, `\u`) || strings.Contains(source, `"""`) {
		return false
	}
	state, ignored := scanNormal, 0
	brace, paren, bracket := 0, 0, 0
	for i := 0; i < len(source); i++ {
		state = scanAdvance(source, &i, state, &ignored)
		if state != scanNormal || i >= len(source) {
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
			return false
		}
	}
	return state == scanNormal && brace == 0 && paren == 0 && bracket == 0
}
