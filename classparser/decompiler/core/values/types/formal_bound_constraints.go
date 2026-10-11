package types

import "strings"

// FormalBoundConstraints retains every ordered bound from the original leading
// Signature section. Erasure uses only the first bound; source invocation
// inference requires all of them. No source-rendered clause is parsed back.
func FormalBoundConstraints(signature string) (map[string][]JavaType, map[int]string, bool) {
	result, rest, valid := formalBoundSection(signature)
	if !valid || !strings.HasPrefix(rest, ">(") {
		return nil, nil, false
	}
	rest = rest[2:]
	parameters := map[int]string{}
	for index := 0; len(rest) > 0 && rest[0] != ')'; index++ {
		if rest[0] == 'T' {
			end := strings.IndexByte(rest, ';')
			if end <= 1 {
				return nil, nil, false
			}
			name := rest[1:end]
			if result[name] != nil {
				parameters[index] = name
			}
		}
		if rest[0] == 'V' {
			return nil, nil, false
		}
		_, remaining, valid := parseSigType(rest)
		if !valid || remaining == rest {
			return nil, nil, false
		}
		rest = remaining
	}
	return result, parameters, strings.HasPrefix(rest, ")")
}

// FormalTypeBounds preserves all original ordered class or method bounds.
// Unlike an erasure query, it retains later interface constraints. Dependent
// bounds require a separate lexical binder proof and are refused here.
func FormalTypeBounds(signature string) (map[string][]JavaType, bool) {
	result, _, valid := formalBoundSection(signature)
	return result, valid
}

func formalBoundSection(signature string) (map[string][]JavaType, string, bool) {
	if _, _, valid := SignatureTypeVariableReferences(signature); !valid {
		return nil, "", false
	}
	if !strings.HasPrefix(signature, "<") {
		return nil, "", false
	}
	result := map[string][]JavaType{}
	rest := signature[1:]
	for len(rest) > 0 && rest[0] != '>' {
		colon := strings.IndexByte(rest, ':')
		if colon <= 0 {
			return nil, "", false
		}
		name := rest[:colon]
		if _, duplicate := result[name]; duplicate {
			return nil, "", false
		}
		rest = rest[colon:]
		var bounds []JavaType
		first := true
		for strings.HasPrefix(rest, ":") {
			rest = rest[1:]
			if first && strings.HasPrefix(rest, ":") {
				first = false
				continue
			}
			first = false
			if !strings.HasPrefix(rest, "L") {
				return nil, "", false
			}
			bound, remaining, valid := parseSigType(rest)
			if !valid || remaining == rest || bound == nil || bound.IsArray() {
				return nil, "", false
			}
			bounds = append(bounds, bound)
			rest = remaining
		}
		if len(bounds) == 0 {
			return nil, "", false
		}
		result[name] = bounds
	}
	if !strings.HasPrefix(rest, ">") || len(result) != len(ClassFormalTypeParamNames(signature)) {
		return nil, "", false
	}
	return result, rest, true
}
