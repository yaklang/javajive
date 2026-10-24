package types

import "strings"

// ObjectWitnessMethodErasure proves that one explicit Object type argument per
// method formal satisfies the entire original bound graph. A bound is either
// exactly Object or another method-owned formal whose chain ends at Object.
// Interface/intersection/concrete constraints, free class formals and cycles
// cannot receive this certificate. This is a witness, not inferred substitution.
func ObjectWitnessMethodErasure(signature string) (descriptor string, throws, formals []string, known bool) {
	if len(signature) > 4096 || !strings.HasPrefix(signature, "<") {
		return
	}
	own, refs, valid := SignatureTypeVariableReferences(signature)
	if !valid || len(own) == 0 || len(own) > 128 {
		return
	}
	parents := map[string]string{}
	bindings := map[string]string{}
	rest := signature[1:]
	for _, name := range own {
		prefix := name + ":"
		if !strings.HasPrefix(rest, prefix) || bindings[name] != "" {
			return
		}
		rest = rest[len(prefix):]
		parent := ""
		if strings.HasPrefix(rest, "Ljava/lang/Object;") {
			rest = rest[len("Ljava/lang/Object;"):]
		} else if strings.HasPrefix(rest, "T") {
			end := strings.IndexByte(rest, ';')
			if end <= 1 {
				return
			}
			parent = rest[1:end]
			rest = rest[end+1:]
		} else {
			return
		}
		parents[name] = parent
		bindings[name] = "Ljava/lang/Object;"
	}
	if !strings.HasPrefix(rest, ">(") {
		return
	}
	rest = rest[1:]
	for _, name := range refs {
		if bindings[name] == "" {
			return
		}
	}
	// Each unary bound edge is visited once. A grey node is a cycle; black nodes
	// already have a complete path to Object, independently of declaration order.
	color := map[string]byte{}
	path := make([]string, 0, len(own))
	for _, name := range own {
		path = path[:0]
		for color[name] != 2 {
			if color[name] == 1 {
				return
			}
			parent, declared := parents[name]
			if !declared {
				return
			}
			color[name] = 1
			path = append(path, name)
			if parent == "" {
				break
			}
			name = parent
		}
		for _, n := range path {
			color[n] = 2
		}
	}
	if strings.Contains(rest, "^T") || !signatureReferenceArgumentsValid(rest, bindings) {
		return
	}
	d, ex, ok := eraseMethodSignature(rest, bindings)
	if !ok {
		return
	}
	return d, ex, own, true
}
