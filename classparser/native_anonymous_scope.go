package javaclassparser

import (
	"fmt"
	"strings"
)

// Anonymous members have already been rendered in their own method contexts.
// Enclosing method and class recovery must not reinterpret their declarations
// using a caller's or sibling method's bindings. Retain each member fragment as
// an opaque comment for that recovery, then restore it byte-for-byte. Allocation
// arguments remain visible, since they do belong to the caller's scope.
func nativeRewriteEnclosingScope(source string, rule func(string) string, declarations ...string) string {
	// Local declarations arrive as exact, already-rendered typed fragments.
	// Validate their complete lexical extent before treating them as opaque;
	// matching arbitrary class-like text grants no declaration permission.
	owned := make([]string, 0, len(declarations))
	seen := map[string]bool{}
	for _, declaration := range declarations {
		if declaration == "" || seen[declaration] {
			continue
		}
		if !strings.HasPrefix(declaration, "class ") && !strings.HasPrefix(declaration, "final class ") {
			return source
		}
		open := javaIndexTopBrace(declaration)
		if open < 0 || javaMatchBrace(declaration, open) != len(declaration)-1 {
			return source
		}
		seen[declaration] = true
		owned = append(owned, declaration)
	}
	const prefix = "/*jdec-owned-anonymous-ordinal:"
	type fragment struct{ token, body string }
	fragments := []fragment{}
	var masked strings.Builder
	from := 0
	state, ignored := scanNormal, 0
	for i := 0; i < len(source); i++ {
		if state == scanNormal {
			declaration := ""
			// Generated local declarations begin a block item. A same-spelled
			// suffix of an identifier, string or comment is not that item.
			boundary := i == 0 || strings.ContainsRune(" \t\r\n\f{};:", rune(source[i-1])) || i >= 2 && source[i-2:i] == "*/"
			if boundary {
				for _, candidate := range owned {
					if len(candidate) > len(declaration) && strings.HasPrefix(source[i:], candidate) {
						declaration = candidate
					}
				}
			}
			if declaration != "" {
				token := fmt.Sprintf("/*jdec lexical declaration %d*/", len(fragments))
				if strings.Contains(source, token) {
					return source
				}
				masked.WriteString(source[from:i])
				masked.WriteString(token)
				fragments = append(fragments, fragment{token, declaration})
				from = i + len(declaration)
				i = from - 1
				continue
			}
		}
		if state == scanNormal && strings.HasPrefix(source[i:], prefix) {
			end := strings.Index(source[i:], "*/")
			if end < 0 {
				return source
			}
			start := i + end + 2
			for start < len(source) && strings.ContainsRune(" \t\r\n\f", rune(source[start])) {
				start++
			}
			// Registration follows constructor arguments, immediately before
			// the anonymous body. Keep the older prefix form readable for
			// fragments already carrying that ownership witness.
			bodyMarker := start < len(source) && source[start] == '{'
			if !bodyMarker && !strings.HasPrefix(source[start:], "new ") {
				return source
			}
			depth, sawArgs, open := 0, false, -1
			innerState := scanNormal
			if bodyMarker {
				open = start
			}
			for j := start; open < 0 && j < len(source); j++ {
				innerState = scanAdvance(source, &j, innerState, &ignored)
				if innerState != scanNormal || j >= len(source) {
					continue
				}
				switch source[j] {
				case '(':
					depth++
					sawArgs = true
				case ')':
					depth--
					if depth < 0 {
						return source
					}
				case '{':
					if sawArgs && depth == 0 {
						open = j
					}
				}
				if open >= 0 {
					break
				}
			}
			if open < 0 {
				return source
			}
			close := javaMatchBrace(source, open)
			if close < 0 {
				return source
			}
			token := fmt.Sprintf("/*jdec lexical members %d*/", len(fragments))
			if strings.Contains(source, token) {
				return source
			}
			masked.WriteString(source[from : open+1])
			masked.WriteString(token)
			fragments = append(fragments, fragment{token, source[open+1 : close]})
			from = close
			i = close
			continue
		}
		state = scanAdvance(source, &i, state, &ignored)
	}
	if len(fragments) == 0 {
		return rule(source)
	}
	masked.WriteString(source[from:])
	output := rule(masked.String())
	for _, f := range fragments {
		// A recovery that drops or duplicates a nested body is outside its scope.
		if strings.Count(output, f.token) != 1 {
			return source
		}
		output = strings.Replace(output, f.token, f.body, 1)
	}
	return output
}
