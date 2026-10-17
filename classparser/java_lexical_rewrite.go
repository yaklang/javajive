package javaclassparser

import (
	"fmt"
	"strings"
)

// rewriteJavaCode shields data and comments while legacy code-pattern rules run.
// Markers retain lexical category, and a collision-free prefix is chosen from the
// input. This is a containment boundary for legacy rules, not a Java parser.
func rewriteJavaCode(source string, rewrite func(string) string) string {
	return rewriteJavaSourceSegments(source, rewrite, true)
}

// Legacy diagnostic comments remain signals at source-only boundaries, while
// identical text in Java literals is data. Code rewrites still shield comments.
func rewriteJavaSourceSegments(source string, rewrite func(string) string, protectComments bool) string {
	prefix := "__JDEC_PROTECTED_"
	for strings.Contains(source, prefix) {
		prefix = "_" + prefix
	}
	var out strings.Builder
	type protected struct{ marker, text string }
	var spans []protected
	for i := 0; i < len(source); {
		start := i
		kind := ""
		switch {
		case strings.HasPrefix(source[i:], "//"):
			kind = "line"
			i += 2
			for i < len(source) && source[i] != '\n' && source[i] != '\r' {
				i++
			}
		case strings.HasPrefix(source[i:], "/*"):
			kind = "block"
			i += 2
			for i < len(source) && !strings.HasPrefix(source[i:], "*/") {
				i++
			}
			if i < len(source) {
				i += 2
			}
		case strings.HasPrefix(source[i:], `"""`):
			kind = "text"
			i += 3
			for i < len(source) {
				if source[i] == '\\' {
					i++
					if i < len(source) {
						i++
					}
					continue
				}
				if strings.HasPrefix(source[i:], `"""`) {
					i += 3
					break
				}
				i++
			}
		case source[i] == '"' || source[i] == '\'':
			quote := source[i]
			kind = "string"
			if quote == '\'' {
				kind = "char"
			}
			i++
			for i < len(source) {
				b := source[i]
				i++
				if b == '\\' {
					if i < len(source) {
						i++
					}
				} else if b == quote {
					break
				}
			}
		default:
			out.WriteByte(source[i])
			i++
			continue
		}
		if !protectComments && (kind == "line" || kind == "block") {
			out.WriteString(source[start:i])
			continue
		}
		marker := fmt.Sprintf("%s%d__", prefix, len(spans))
		switch kind {
		case "line":
			marker = "//" + marker
		case "block":
			marker = "/*" + marker + "*/"
		case "text", "string":
			marker = `"` + marker + `"`
		case "char":
			marker = "'" + marker + "'"
		}
		spans = append(spans, protected{marker, source[start:i]})
		out.WriteString(marker)
	}
	result := rewrite(out.String())
	for _, span := range spans {
		result = strings.ReplaceAll(result, span.marker, span.text)
	}
	// A rule which rewrote a marker has crossed the containment boundary. Decline
	// that transformation rather than leaking a placeholder into generated Java.
	if strings.Contains(result, prefix) {
		return source
	}
	return result
}
