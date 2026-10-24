package javaclassparser

import "strings"

// Generated spellings supply candidates, never a variable binding. A qualified
// member, method name or non-code token cannot require a local declaration.
// Existing typed declarations and lexical bindings remain authoritative at the
// consumer, including a binding whose spelling resembles the receiver slot.
func generatedLocalVariableCandidates(source string) []string {
	var out []string
	previous := ""
	ident := func(b byte) bool { return isJavaIdentByte(b) || b >= 0x80 }
	for i := 0; i < len(source); {
		if next, skipped := skipJavaNonCode(source, i); skipped {
			i = next
			continue
		}
		if isJavaWhitespace(source[i]) {
			i++
			continue
		}
		if ident(source[i]) {
			start := i
			i++
			for i < len(source) && ident(source[i]) {
				i++
			}
			name := source[start:i]
			if strings.HasPrefix(name, "var") && generatedLocalRefRe.FindString(name) == name {
				next := i
				for next < len(source) {
					if isJavaWhitespace(source[next]) {
						next++
						continue
					}
					if source[next] == '/' && next+1 < len(source) && (source[next+1] == '/' || source[next+1] == '*') {
						end, _ := skipJavaNonCode(source, next)
						next = end
						continue
					}
					break
				}
				if previous != "." && previous != "::" && previous != "@" && previous != "break" && previous != "continue" && (next == len(source) || source[next] != '(') {
					out = append(out, name)
				}
			}
			previous = name
			continue
		}
		if source[i] == ':' && i+1 < len(source) && source[i+1] == ':' {
			previous = "::"
			i += 2
			continue
		}
		previous = source[i : i+1]
		i++
	}
	return out
}
