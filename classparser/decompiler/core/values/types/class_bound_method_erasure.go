package types

import "strings"

// EraseClassBoundMethodSignature checks erasure using only the declaring class's
// original formal declarations. Method formals and missing/dependent bounds
// remain unproved. Grammar tags retain the distinction between a class named T
// (LT;) and the formal T (TT;), which the source-rendering type model cannot.
func EraseClassBoundMethodSignature(classSignature, methodSignature string) (string, bool) {
	descriptor, _, known := EraseClassBoundMethodSignatureWithThrows(classSignature, methodSignature)
	return descriptor, known
}

// EraseClassBoundMethodSignatureWithThrows also retains the Signature throws
// erasures, so callers can check the original Exceptions attribute contract.
func EraseClassBoundMethodSignatureWithThrows(classSignature, methodSignature string) (string, []string, bool) {
	classFormals, classRefs, valid := SignatureTypeVariableReferences(classSignature)
	if !valid || len(classFormals) == 0 || !strings.HasPrefix(classSignature, "<") {
		return "", nil, false
	}
	declared := map[string]bool{}
	for _, name := range classFormals {
		declared[name] = true
	}
	for _, name := range classRefs {
		if !declared[name] {
			return "", nil, false
		}
	}
	methodFormals, refs, valid := SignatureTypeVariableReferences(methodSignature)
	if !valid || len(methodFormals) != 0 || !strings.HasPrefix(methodSignature, "(") {
		return "", nil, false
	}
	for _, name := range refs {
		if !declared[name] {
			return "", nil, false
		}
	}
	bounds := map[string]string{}
	referenceSignatures := []string{methodSignature}
	rest := classSignature[1:]
	for len(rest) > 0 && rest[0] != '>' {
		colon := strings.IndexByte(rest, ':')
		if colon <= 0 {
			return "", nil, false
		}
		name := rest[:colon]
		rest = rest[colon:]
		for len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:]
			if len(rest) > 0 && rest[0] == ':' {
				continue
			}
			// A concrete first bound has a closed erasure independent of substitutions.
			// An unknown first bound must not fall through to a later interface bound.
			if len(rest) == 0 || (rest[0] != 'L' && rest[0] != 'T' && rest[0] != '[') {
				return "", nil, false
			}
			before := rest
			_, after, ok := parseSigType(rest)
			if !ok {
				return "", nil, false
			}
			referenceSignatures = append(referenceSignatures, before[:len(before)-len(after)])
			if bounds[name] == "" {
				if before[0] != 'L' {
					return "", nil, false
				}
				t := ParseSignature(before[:len(before)-len(after)])
				raw, known := RawClassFQN(t)
				if !known || raw == "" {
					return "", nil, false
				}
				bounds[name] = "L" + strings.ReplaceAll(raw, ".", "/") + ";"
			}
			rest = after
		}
		if bounds[name] == "" {
			return "", nil, false
		}
	}
	if len(bounds) != len(classFormals) || !strings.HasPrefix(rest, ">") {
		return "", nil, false
	}
	rest = rest[1:]
	referenceSignatures = append(referenceSignatures, rest)
	// Class signatures have class/interface supers, never method or field grammar.
	for len(rest) > 0 {
		if rest[0] != 'L' {
			return "", nil, false
		}
		_, after, ok := parseSigType(rest)
		if !ok {
			return "", nil, false
		}
		rest = after
	}
	// Erasure hides arguments, but it must never license invalid original
	// reference grammar. Validate bound and superclass arguments only after
	// all declarations are available, so recursive class bounds retain their
	// own binder while primitive arguments and void arrays remain invalid.
	for _, signature := range referenceSignatures {
		if !signatureReferenceArgumentsValid(signature, bounds) {
			return "", nil, false
		}
	}
	return eraseMethodSignature(methodSignature, bounds)
}

// EraseConcreteMethodSignatureWithThrows admits parameterized types whose
// erasure needs no class or method type-variable substitution. In particular a
// class literally named T remains LT;, whereas TT; requires a declaration.
func EraseConcreteMethodSignatureWithThrows(signature string) (string, []string, bool) {
	formals, refs, valid := SignatureTypeVariableReferences(signature)
	if !valid || len(formals) != 0 || len(refs) != 0 || !strings.HasPrefix(signature, "(") || !concreteSignatureArgumentsValid(signature) {
		return "", nil, false
	}
	return eraseMethodSignature(signature, nil)
}

// The general type renderer tolerates primitive wildcard bounds. Original
// Signature type arguments require reference types; validate that grammar
// before letting erasure hide a malformed parameterization.
func concreteSignatureArgumentsValid(signature string) bool {
	return signatureReferenceArgumentsValid(signature, nil)
}

func signatureReferenceArgumentsValid(signature string, variables map[string]string) bool {
	for cursor := 0; cursor < len(signature); cursor++ {
		if signature[cursor] != '<' {
			continue
		}
		rest := signature[cursor+1:]
		for len(rest) > 0 && rest[0] != '>' {
			if rest[0] == '*' {
				rest = rest[1:]
				continue
			}
			if rest[0] == '+' || rest[0] == '-' {
				rest = rest[1:]
			}
			if len(rest) == 0 || rest[0] != 'L' && rest[0] != '[' && rest[0] != 'T' {
				return false
			}
			base := strings.TrimLeft(rest, "[")
			if base == "" || base[0] == 'V' {
				return false
			}
			if base[0] == 'T' {
				end := strings.IndexByte(base, ';')
				if end <= 1 || variables[base[1:end]] == "" {
					return false
				}
			}
			_, after, known := parseSigType(rest)
			if !known {
				return false
			}
			rest = after
		}
	}
	return true
}

func eraseMethodSignature(methodSignature string, bounds map[string]string) (string, []string, bool) {
	var erase func(string, int, bool) (string, string, bool)
	erase = func(sig string, depth int, void bool) (string, string, bool) {
		if sig == "" || depth > 128 {
			return "", "", false
		}
		switch sig[0] {
		case 'V':
			if void {
				return "V", sig[1:], true
			}
		case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
			return sig[:1], sig[1:], true
		case '[':
			d, after, ok := erase(sig[1:], depth+1, false)
			return "[" + d, after, ok
		case 'T':
			end := strings.IndexByte(sig, ';')
			if end <= 1 {
				return "", "", false
			}
			d := bounds[sig[1:end]]
			return d, sig[end+1:], d != ""
		case 'L':
			t, after, ok := parseSigType(sig)
			if !ok {
				return "", "", false
			}
			raw, known := RawClassFQN(t)
			if !known || raw == "" {
				return "", "", false
			}
			return "L" + strings.ReplaceAll(raw, ".", "/") + ";", after, true
		}
		return "", "", false
	}
	rest := methodSignature[1:]
	var descriptor strings.Builder
	descriptor.WriteByte('(')
	for len(rest) > 0 && rest[0] != ')' {
		d, after, ok := erase(rest, 0, false)
		if !ok {
			return "", nil, false
		}
		descriptor.WriteString(d)
		rest = after
	}
	if !strings.HasPrefix(rest, ")") {
		return "", nil, false
	}
	d, rest, ok := erase(rest[1:], 0, true)
	if !ok {
		return "", nil, false
	}
	descriptor.WriteByte(')')
	descriptor.WriteString(d)
	var throws []string
	for strings.HasPrefix(rest, "^") {
		if len(rest) < 2 || (rest[1] != 'L' && rest[1] != 'T') {
			return "", nil, false
		}
		d, after, ok := erase(rest[1:], 0, false)
		if !ok {
			return "", nil, false
		}
		throws = append(throws, d)
		rest = after
	}
	return descriptor.String(), throws, rest == ""
}
