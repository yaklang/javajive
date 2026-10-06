package types

import "strings"

// EraseLexicalOwnerMethodSignatureWithThrows resolves an outer-to-inner stack
// of original class signatures, followed by the original method signature.
// Each formal retains its declaration environment: an inner T cannot change
// the first bound of an outer U declared as U extends T. Static cuts are the
// caller's physical ownership proof; source renderer hints are not inputs.
func EraseLexicalOwnerMethodSignatureWithThrows(classes []string, method string) (string, []string, bool) {
	return eraseLexicalOwnerMethodSignature(classes, method, nil)
}

// LexicalOwnerTypeVariableErasure retains the same original declaration stack
// and validates the complete method before exposing any binding erasure.
func LexicalOwnerTypeVariableErasure(classes []string, method, name string) (string, string, bool) {
	result := ""
	descriptor, _, ok := eraseLexicalOwnerMethodSignature(classes, method, func(bounds map[string]string) { result = bounds[name] })
	return result, descriptor, ok && result != ""
}

func eraseLexicalOwnerMethodSignature(classes []string, method string, accept func(map[string]string)) (string, []string, bool) {
	if len(classes) > 64 || len(method) > 65535 {
		return "", nil, false
	}
	type formal struct {
		first       string
		environment map[string]*formal
		erased      string
		visiting    bool
	}
	scope := map[string]*formal{}
	retained, declarations := len(method), 0
	var erase func(*formal, int) (string, bool)
	erase = func(f *formal, depth int) (string, bool) {
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
		next := make(map[string]*formal, len(scope)+len(names))
		for n, f := range scope {
			next[n] = f
		}
		declarations += len(names)
		if declarations > 512 {
			return "", nil, false
		}
		for _, n := range names {
			next[n] = &formal{environment: next}
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
	for _, s := range classes {
		retained += len(s)
		if retained > 1<<20 {
			return "", nil, false
		}
		if s == "" {
			continue
		}
		if _, _, ok := extend(s, false); !ok {
			return "", nil, false
		}
	}
	body, bounds, ok := extend(method, true)
	if !ok {
		return "", nil, false
	}
	descriptor, throws, valid := eraseMethodSignature(body, bounds)
	if valid && accept != nil {
		accept(bounds)
	}
	return descriptor, throws, valid
}
