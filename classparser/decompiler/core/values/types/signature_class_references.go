package types

import "strings"

// SignatureClassReferences reads binary class identities from descriptor or
// Signature grammar, including formal bounds and nested owner arguments.
// Type-variable identifiers are skipped by their T grammar tag, so a literal
// dollar in a formal name cannot become a guessed class reference.
func SignatureClassReferences(signature string) ([]string, bool) {
	classes, _, _, ok := signatureReferences(signature)
	return classes, ok
}

// SignatureTypeVariableReferences includes variables in formal bounds as well
// as parameter, return, throws and owner arguments. Declarations are separate:
// a static member cannot inherit enclosing class variables merely because the
// renderer knows their names.
func SignatureTypeVariableReferences(signature string) (formals, references []string, valid bool) {
	_, formals, references, valid = signatureReferences(signature)
	return
}

func signatureReferences(signature string) (classes, formals, references []string, valid bool) {
	if signature == "" || len(signature) > 65535 {
		return nil, nil, nil, false
	}
	names := []string{}
	seen := map[string]bool{}
	declared := map[string]bool{}
	cursor, work, retained := 0, 0, 0
	add := func(name string) bool {
		name = SlashToDot(name)
		if !seen[name] {
			retained += len(name)
			if retained > 1<<20 {
				return false
			}
			seen[name] = true
			names = append(names, name)
		}
		return true
	}
	var typ func(int) bool
	typ = func(depth int) bool {
		work++
		if cursor >= len(signature) || depth > 128 || work > 8192 {
			return false
		}
		tag := signature[cursor]
		cursor++
		switch tag {
		case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z', 'V':
			return true
		case 'T':
			end := strings.IndexByte(signature[cursor:], ';')
			if end <= 0 {
				return false
			}
			references = append(references, signature[cursor:cursor+end])
			cursor += end + 1
			return true
		case '[':
			return typ(depth + 1)
		case 'L':
			binary := ""
			segments := 0
			for {
				segments++
				if segments > 128 {
					return false
				}
				start := cursor
				for cursor < len(signature) && !strings.ContainsRune("<.;", rune(signature[cursor])) {
					if strings.ContainsRune("[:>()", rune(signature[cursor])) {
						return false
					}
					cursor++
				}
				if cursor == start || cursor >= len(signature) {
					return false
				}
				if binary != "" {
					binary += "$"
				}
				binary += signature[start:cursor]
				if !add(binary) {
					return false
				}
				if signature[cursor] == '<' {
					cursor++
					count := 0
					for cursor < len(signature) && signature[cursor] != '>' {
						count++
						if signature[cursor] == '*' {
							cursor++
							continue
						}
						if signature[cursor] == '+' || signature[cursor] == '-' {
							cursor++
						}
						if !typ(depth + 1) {
							return false
						}
					}
					if count == 0 || cursor >= len(signature) {
						return false
					}
					cursor++
				}
				if cursor >= len(signature) {
					return false
				}
				if signature[cursor] == ';' {
					cursor++
					return true
				}
				if signature[cursor] != '.' {
					return false
				}
				cursor++
			}
		}
		return false
	}
	if signature[cursor] == '<' {
		cursor++
		for cursor < len(signature) && signature[cursor] != '>' {
			colon := strings.IndexByte(signature[cursor:], ':')
			if colon <= 0 {
				return nil, nil, nil, false
			}
			name := signature[cursor : cursor+colon]
			if strings.ContainsAny(name, ".;[/<>():") {
				return nil, nil, nil, false
			}
			work++
			if declared[name] || work > 8192 {
				return nil, nil, nil, false
			}
			declared[name] = true
			formals = append(formals, name)
			cursor += colon
			for cursor < len(signature) && signature[cursor] == ':' {
				cursor++
				if cursor < len(signature) && (signature[cursor] == ':' || signature[cursor] == '>') {
					continue
				}
				if !typ(0) {
					return nil, nil, nil, false
				}
			}
		}
		if cursor >= len(signature) {
			return nil, nil, nil, false
		}
		cursor++
	}
	if cursor >= len(signature) {
		return nil, nil, nil, false
	}
	if signature[cursor] == '(' {
		cursor++
		for cursor < len(signature) && signature[cursor] != ')' {
			if !typ(0) {
				return nil, nil, nil, false
			}
		}
		if cursor >= len(signature) {
			return nil, nil, nil, false
		}
		cursor++
		if !typ(0) {
			return nil, nil, nil, false
		}
		for cursor < len(signature) && signature[cursor] == '^' {
			cursor++
			if !typ(0) {
				return nil, nil, nil, false
			}
		}
	} else {
		for cursor < len(signature) {
			if !typ(0) {
				return nil, nil, nil, false
			}
		}
	}
	return names, formals, references, cursor == len(signature)
}
