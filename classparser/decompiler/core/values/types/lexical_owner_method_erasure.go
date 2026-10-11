package types

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// LexicalTypeScope is one original declaration in outer-to-inner order. A
// method carries its physical descriptor; callers prove ownership and static
// cuts before supplying the stack. Method scopes can occur between classes.
type LexicalTypeScope struct {
	Signature  string
	Method     bool
	Descriptor string
}

type lexicalFormal struct {
	first, signature, name string
	environment            map[string]*lexicalFormal
	erased                 string
	visiting               bool
}

// LexicalTypeParameterErasures needs no source rebinding of hidden formals:
// the concrete erased descriptor remains representable even when a bound's
// source name has been shadowed in the final lexical environment.
func LexicalTypeParameterErasures(scopes []LexicalTypeScope, requested []string) (map[string]string, bool) {
	if len(requested) > 512 {
		return nil, false
	}
	result := map[string]string{}
	known := true
	_, _, valid := eraseLexicalTypeScopes(scopes, func(scope map[string]*lexicalFormal) {
		for _, n := range requested {
			f := scope[n]
			if f == nil {
				known = false
				break
			}
			result[n] = f.erased
		}
	})
	if !known || !valid {
		return nil, false
	}
	return result, true
}

// ProjectLexicalTypeParameters closes requested formals over their bound
// dependencies. Object-bound declarations still shadow outer declarations.
// A bound referring to a hidden, differently bound declaration cannot be
// represented by reusing its spelling and is rejected rather than rebound.
func ProjectLexicalTypeParameters(scopes []LexicalTypeScope, requested []string, ctx *class_context.ClassContext) ([]string, map[string]string, map[string]string, bool) {
	if len(requested) > 512 {
		return nil, nil, nil, false
	}
	var names []string
	clauses, erasures := map[string]string{}, map[string]string{}
	projected := false
	_, _, valid := eraseLexicalTypeScopes(scopes, func(scope map[string]*lexicalFormal) {
		seen := map[string]bool{}
		boundCache := map[string]map[string]TypeParamBound{}
		var visit func(string) bool
		visit = func(name string) bool {
			f := scope[name]
			if f == nil {
				return false
			}
			if seen[name] {
				return true
			}
			seen[name] = true
			names = append(names, name)
			bounds, cached := boundCache[f.signature]
			if !cached {
				bounds = ClassFormalTypeParamBounds(f.signature, ctx)
				boundCache[f.signature] = bounds
			}
			bound := bounds[f.name]
			for _, ref := range bound.Refs {
				if f.environment[ref] != scope[ref] || !visit(ref) {
					return false
				}
			}
			if bound.Clause != "" {
				clauses[name] = bound.Clause
			}
			erasures[name] = f.erased
			return true
		}
		projected = true
		for _, name := range requested {
			if !visit(name) {
				projected = false
				break
			}
		}
	})
	if !valid || !projected {
		return nil, nil, nil, false
	}
	return names, clauses, erasures, true
}

// EraseLexicalOwnerMethodSignatureWithThrows resolves an outer-to-inner stack
// of original class signatures, followed by the original method signature.
// Each formal retains its declaration environment: an inner T cannot change
// the first bound of an outer U declared as U extends T. Static cuts are the
// caller's physical ownership proof; source renderer hints are not inputs.
func EraseLexicalOwnerMethodSignatureWithThrows(classes []string, method string) (string, []string, bool) {
	return eraseLexicalOwnerMethodSignature(classes, method, nil)
}

// EraseLexicalScopedMethodSignatureWithThrows preserves enclosing method scopes
// interleaved with class declarations. The caller owns the original lexical
// path and static cuts; every enclosing physical method descriptor is checked.
func EraseLexicalScopedMethodSignatureWithThrows(scopes []LexicalTypeScope, method string) (string, []string, bool) {
	if len(scopes) >= 129 {
		return "", nil, false
	}
	declarations := make([]LexicalTypeScope, 0, len(scopes)+1)
	declarations = append(declarations, scopes...)
	declarations = append(declarations, LexicalTypeScope{Signature: method, Method: true})
	return eraseLexicalTypeScopes(declarations, nil)
}

// LexicalOwnerTypeVariableErasure retains the same original declaration stack
// and validates the complete method before exposing any binding erasure.
func LexicalOwnerTypeVariableErasure(classes []string, method, name string) (string, string, bool) {
	result := ""
	descriptor, _, ok := eraseLexicalOwnerMethodSignature(classes, method, func(bounds map[string]string) { result = bounds[name] })
	return result, descriptor, ok && result != ""
}

func eraseLexicalOwnerMethodSignature(classes []string, method string, accept func(map[string]string)) (string, []string, bool) {
	if len(classes) > 64 {
		return "", nil, false
	}
	scopes := make([]LexicalTypeScope, 0, len(classes)+1)
	for _, s := range classes {
		scopes = append(scopes, LexicalTypeScope{Signature: s})
	}
	scopes = append(scopes, LexicalTypeScope{Signature: method, Method: true})
	return eraseLexicalTypeScopes(scopes, func(scope map[string]*lexicalFormal) {
		if accept != nil {
			bounds := map[string]string{}
			for n, f := range scope {
				bounds[n] = f.erased
			}
			accept(bounds)
		}
	})
}

func eraseLexicalTypeScopes(scopes []LexicalTypeScope, accept func(map[string]*lexicalFormal)) (string, []string, bool) {
	if len(scopes) > 129 {
		return "", nil, false
	}
	scope := map[string]*lexicalFormal{}
	retained, declarations := 0, 0
	var erase func(*lexicalFormal, int) (string, bool)
	erase = func(f *lexicalFormal, depth int) (string, bool) {
		if f == nil || depth > 128 || f.visiting {
			return "", false
		}
		if f.erased != "" {
			return f.erased, true
		}
		f.visiting = true
		defer func() { f.visiting = false }()
		if strings.HasPrefix(f.first, "T") {
			end := strings.IndexByte(f.first, ';')
			if end != len(f.first)-1 || end <= 1 {
				return "", false
			}
			d, ok := erase(f.environment[f.first[1:end]], depth+1)
			if !ok {
				return "", false
			}
			f.erased = d
			return d, true
		}
		if !strings.HasPrefix(f.first, "L") {
			return "", false
		}
		parsed, rest, ok := parseSigType(f.first)
		if !ok || rest != "" {
			return "", false
		}
		raw, ok := RawClassFQN(parsed)
		if !ok || raw == "" {
			return "", false
		}
		f.erased = "L" + strings.ReplaceAll(raw, ".", "/") + ";"
		return f.erased, true
	}
	extend := func(signature string, isMethod bool) (string, map[string]string, bool) {
		names, refs, ok := SignatureTypeVariableReferences(signature)
		if !ok {
			return "", nil, false
		}
		next := make(map[string]*lexicalFormal, len(scope)+len(names))
		for n, f := range scope {
			next[n] = f
		}
		declarations += len(names)
		if declarations > 512 {
			return "", nil, false
		}
		for _, n := range names {
			next[n] = &lexicalFormal{environment: next, signature: signature, name: n}
		}
		rest := signature
		parts := []string{}
		if len(names) > 0 {
			if !strings.HasPrefix(rest, "<") {
				return "", nil, false
			}
			rest = rest[1:]
			for _, n := range names {
				if !strings.HasPrefix(rest, n+":") {
					return "", nil, false
				}
				rest = rest[len(n):]
				first := ""
				classBound := true
				for strings.HasPrefix(rest, ":") {
					rest = rest[1:]
					if classBound && strings.HasPrefix(rest, ":") {
						classBound = false
						continue
					}
					classBound = false
					if rest == "" || rest[0] != 'L' && rest[0] != 'T' {
						return "", nil, false
					}
					before := rest
					_, after, valid := parseSigType(rest)
					if !valid || len(after) >= len(before) {
						return "", nil, false
					}
					part := before[:len(before)-len(after)]
					if first == "" {
						first = part
					}
					parts = append(parts, part)
					rest = after
				}
				if first == "" {
					return "", nil, false
				}
				next[n].first = first
			}
			if !strings.HasPrefix(rest, ">") {
				return "", nil, false
			}
			rest = rest[1:]
		}
		bounds := map[string]string{}
		for n, f := range next {
			d, valid := erase(f, 0)
			if !valid {
				return "", nil, false
			}
			bounds[n] = d
		}
		for _, ref := range refs {
			if next[ref] == nil {
				return "", nil, false
			}
		}
		if isMethod {
			if !strings.HasPrefix(rest, "(") {
				return "", nil, false
			}
		} else {
			if rest == "" {
				return "", nil, false
			}
			tail := rest
			for tail != "" {
				if tail[0] != 'L' {
					return "", nil, false
				}
				_, after, valid := parseSigType(tail)
				if !valid || len(after) >= len(tail) {
					return "", nil, false
				}
				tail = after
			}
		}
		parts = append(parts, rest)
		for _, p := range parts {
			if !signatureReferenceArgumentsValid(p, bounds) {
				return "", nil, false
			}
		}
		scope = next
		return rest, bounds, true
	}
	descriptor := ""
	var throws []string
	for _, declaration := range scopes {
		s := declaration.Signature
		if len(s) > 65535 {
			return "", nil, false
		}
		retained += len(s)
		if retained > 1<<20 {
			return "", nil, false
		}
		if s == "" && !declaration.Method {
			continue
		}
		body, bounds, ok := extend(s, declaration.Method)
		if !ok {
			return "", nil, false
		}
		if declaration.Method {
			var valid bool
			descriptor, throws, valid = eraseMethodSignature(body, bounds)
			if !valid || declaration.Descriptor != "" && descriptor != declaration.Descriptor {
				return "", nil, false
			}
		}
	}
	if accept != nil {
		accept(scope)
	}
	return descriptor, throws, true
}
