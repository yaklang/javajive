package types

import "strings"

// ProjectHiddenLexicalMethodSignature models the source view of a physical
// member with its own formals: outer binders cannot be redeclared without
// changing member arity. Resolve them in their original declaration scopes,
// then erase only their source occurrences. Method formals shadow outer ones.
// Direct hidden type arguments make that parameterization raw; nested type
// arguments retain their independently projected structure.
func ProjectHiddenLexicalMethodSignature(scopes []LexicalTypeScope, signature string) (string, bool) {
	return projectHiddenLexicalSignature(scopes, signature, true)
}

// ProjectHiddenLexicalClassSignature retains the member's own arity, bound
// order and supertype structure while projecting hidden enclosing occurrences.
// The supplied lexical path ends at that original class declaration.
func ProjectHiddenLexicalClassSignature(scopes []LexicalTypeScope) (string, bool) {
	if len(scopes) == 0 {
		return "", false
	}
	return projectHiddenLexicalSignature(scopes, scopes[len(scopes)-1].Signature, false)
}

func projectHiddenLexicalSignature(scopes []LexicalTypeScope, signature string, method bool) (string, bool) {
	if len(scopes) == 0 || len(scopes) >= 129 || scopes[len(scopes)-1].Method {
		return "", false
	}
	visible := map[string]bool{}
	for _, n := range ClassFormalTypeParamNames(scopes[len(scopes)-1].Signature) {
		visible[n] = true
	}
	if method {
		for _, n := range MethodFormalTypeParamNames(signature) {
			visible[n] = true
		}
	}
	declarations := append([]LexicalTypeScope(nil), scopes...)
	if method {
		declarations = append(declarations, LexicalTypeScope{Signature: signature, Method: true})
	}
	hidden := map[string]string{}
	_, _, valid := eraseLexicalTypeScopes(declarations, func(bindings map[string]*lexicalFormal) {
		for n, f := range bindings {
			if !visible[n] {
				hidden[n] = f.erased
			}
		}
	})
	if !valid {
		return "", false
	}
	p := hiddenSignatureProjection{input: signature, hidden: hidden}
	var out strings.Builder
	if p.take('<') {
		out.WriteByte('<')
		for p.at < len(signature) && signature[p.at] != '>' {
			start := p.at
			for p.at < len(signature) && signature[p.at] != ':' {
				p.at++
			}
			out.WriteString(signature[start:p.at])
			for p.take(':') {
				out.WriteByte(':')
				if p.at < len(signature) && signature[p.at] != ':' {
					typ, _, ok := p.typ(0)
					if !ok {
						return "", false
					}
					out.WriteString(typ)
				}
			}
		}
		if !p.take('>') {
			return "", false
		}
		out.WriteByte('>')
	}
	if !method {
		for p.at < len(signature) {
			typ, _, ok := p.typ(0)
			if !ok || len(typ) == 0 || typ[0] != 'L' {
				return "", false
			}
			out.WriteString(typ)
			if out.Len() > 65535 {
				return "", false
			}
		}
		return out.String(), out.Len() != 0
	}
	if !p.take('(') {
		return "", false
	}
	out.WriteByte('(')
	for p.at < len(signature) && signature[p.at] != ')' {
		typ, _, ok := p.typ(0)
		if !ok {
			return "", false
		}
		out.WriteString(typ)
	}
	if !p.take(')') {
		return "", false
	}
	out.WriteByte(')')
	typ, _, ok := p.typ(0)
	if !ok {
		return "", false
	}
	out.WriteString(typ)
	for p.take('^') {
		out.WriteByte('^')
		typ, _, ok := p.typ(0)
		if !ok {
			return "", false
		}
		out.WriteString(typ)
	}
	if p.at != len(signature) || out.Len() > 65535 {
		return "", false
	}
	return out.String(), true
}

type hiddenSignatureProjection struct {
	input            string
	at, nodes, built int
	hidden           map[string]string
}

func (p *hiddenSignatureProjection) take(c byte) bool {
	if p.at < len(p.input) && p.input[p.at] == c {
		p.at++
		return true
	}
	return false
}

// direct records a hidden variable at this argument level, including a
// wildcard bound, but excluding an array element or nested parameterization.
func (p *hiddenSignatureProjection) typ(depth int) (text string, direct, valid bool) {
	// An enclosing bound can be much longer than its variable spelling.
	// Bound cumulative intermediate construction as well as final UTF8 size;
	// checking only the final signature would allocate repeated large bounds.
	defer func() {
		p.built += len(text)
		if p.built > 1<<20 || len(text) > 65535 {
			text, direct, valid = "", false, false
		}
	}()
	p.nodes++
	if depth > 64 || p.nodes > 4096 || p.at >= len(p.input) {
		return "", false, false
	}
	c := p.input[p.at]
	p.at++
	switch c {
	case 'T':
		end := strings.IndexByte(p.input[p.at:], ';')
		if end <= 0 {
			return "", false, false
		}
		name := p.input[p.at : p.at+end]
		p.at += end + 1
		if erased, ok := p.hidden[name]; ok {
			return erased, true, true
		}
		return "T" + name + ";", false, true
	case '[', '+', '-':
		child, hidden, ok := p.typ(depth + 1)
		return string(c) + child, c != '[' && hidden, ok
	case 'L':
		var out strings.Builder
		var binary strings.Builder
		out.WriteByte('L')
		binary.WriteByte('L')
		parameterizedOwner := false
		rawMember := false
		for p.at < len(p.input) {
			if p.take(';') {
				out.WriteByte(';')
				if rawMember {
					// A partially raw member chain is not a Java source type.
					// Erase this complete owner/member chain, while an enclosing
					// parameterization still keeps this raw class as its argument.
					binary.WriteByte(';')
					return binary.String(), false, true
				}
				return out.String(), false, true
			}
			if p.take('<') {
				var args strings.Builder
				raw := false
				for p.at < len(p.input) && p.input[p.at] != '>' {
					arg, hidden, ok := p.typ(depth + 1)
					if !ok {
						return "", false, false
					}
					raw = raw || hidden
					args.WriteString(arg)
				}
				if !p.take('>') {
					return "", false, false
				}
				rawMember = rawMember || raw
				if !raw {
					out.WriteByte('<')
					out.WriteString(args.String())
					out.WriteByte('>')
					parameterizedOwner = true
				}
			} else {
				separator := p.input[p.at]
				if separator == '.' {
					binary.WriteByte('$')
				} else {
					binary.WriteByte(separator)
				}
				// Signature suffix dots encode a member of a parameterized
				// owner. Once every preceding argument list is raw, javac
				// spells that prefix with its binary '$' separators. Retain
				// dots after any surviving owner arguments: flattening those
				// would lose the owner-to-member generic binding.
				if separator == '.' && !parameterizedOwner {
					separator = '$'
				}
				out.WriteByte(separator)
				p.at++
			}
		}
		return "", false, false
	default:
		return string(c), false, strings.ContainsRune("BCDFIJSZV*", rune(c))
	}
}
