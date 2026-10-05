package types

import "strings"

// UnboundedMethodErasure proves a method-owned substitution, independently of
// any same-spelled class formal. Every formal has the single Object bound;
// explicitly instantiating all of them with Object is valid and keeps the
// original erased parameter/return contract. Dependent/intersection bounds,
// class variables, and inferred substitutions are deliberately outside it.
func UnboundedMethodErasure(signature string) (descriptor string, throws, formals []string, known bool) {
	if len(signature) > 4096 || !strings.HasPrefix(signature, "<") {
		return
	}
	own, refs, valid := SignatureTypeVariableReferences(signature)
	if !valid || len(own) == 0 || len(own) > 128 {
		return
	}
	bindings := map[string]string{}
	rest := signature[1:]
	for _, name := range own {
		declaration := name + ":Ljava/lang/Object;"
		if !strings.HasPrefix(rest, declaration) || bindings[name] != "" {
			return
		}
		bindings[name] = "Ljava/lang/Object;"
		rest = rest[len(declaration):]
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
	if strings.Contains(rest, "^T") || !signatureReferenceArgumentsValid(rest, bindings) {
		return
	}
	d, ex, ok := eraseMethodSignature(rest, bindings)
	if !ok {
		return
	}
	return d, ex, own, true
}
