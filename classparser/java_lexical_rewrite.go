package javaclassparser

import (
	"fmt"
	"strconv"
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
	out.Grow(len(source))
	var spans []protectedSpan
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
		inner := fmt.Sprintf("%s%d__", prefix, len(spans))
		marker := inner
		wrapper := 0
		switch kind {
		case "line":
			marker = "//" + inner
			wrapper = 2
		case "block":
			marker = "/*" + inner + "*/"
			wrapper = 2
		case "text", "string":
			marker = `"` + inner + `"`
			wrapper = 1
		case "char":
			marker = "'" + inner + "'"
			wrapper = 1
		}
		spans = append(spans, protectedSpan{marker: marker, text: source[start:i], inner: wrapper})
		out.WriteString(marker)
	}
	result := rewrite(out.String())
	restored, ok := restoreProtectedSpans(result, prefix, spans)
	// A rule which rewrote a marker has crossed the containment boundary. Decline
	// that transformation rather than leaking a placeholder into generated Java.
	if !ok || strings.Contains(restored, prefix) {
		return source
	}
	return restored
}

// protectedSpan is one shielded literal or comment. marker is the complete
// placeholder text (including its quote or comment wrapper) and inner is the
// offset of the shared prefix inside marker.
type protectedSpan struct {
	marker string
	text   string
	inner  int
}

// restoreProtectedSpans puts the shielded literals and comments back in one
// linear pass. The previous loop called strings.ReplaceAll once per span, so a
// source with S literals paid O(S x len(result)) copying even though only the
// spans a rewrite touched need work.
//
// The common case is exact: markers emitted by the scanner do not overlap, the
// prefix does not occur in the input, span texts are slices of the input (so
// they cannot contain another marker), and no marker is a prefix of another
// (the trailing underscore of "%d__" cannot match the next digit of a longer
// id). Every prefix occurrence that resolves to a complete stored marker is
// then independent, so replacing them left to right yields the same result as
// applying every ReplaceAll in order.
//
// A rewrite that deleted or rewrote a marker's quotes can make two marker
// fragments share a wrapper character, which sequential ReplaceAll would still
// salvage by processing spans in id order. That case is detected (an occurrence
// is incomplete or overlaps the previous match) and delegated to the exact
// sequential oracle, preserving the old behavior and cost only for mangled
// marker output, which is rare.
func restoreProtectedSpans(result, prefix string, spans []protectedSpan) (string, bool) {
	if len(spans) == 0 {
		return result, !strings.Contains(result, prefix)
	}
	var out strings.Builder
	out.Grow(len(result))
	pos := 0
	for pos < len(result) {
		rel := strings.Index(result[pos:], prefix)
		if rel < 0 {
			out.WriteString(result[pos:])
			break
		}
		at := pos + rel
		idStart := at + len(prefix)
		j := idStart
		for j < len(result) && result[j] >= '0' && result[j] <= '9' {
			j++
		}
		id := -1
		if j > idStart && j+1 < len(result) && result[j] == '_' && result[j+1] == '_' {
			if n, err := strconv.Atoi(result[idStart:j]); err == nil && n >= 0 && n < len(spans) {
				id = n
			}
		}
		if id < 0 {
			return restoreProtectedSpansSequential(result, prefix, spans)
		}
		span := spans[id]
		start := at - span.inner
		end := start + len(span.marker)
		if start < pos || end > len(result) || result[start:end] != span.marker {
			return restoreProtectedSpansSequential(result, prefix, spans)
		}
		out.WriteString(result[pos:start])
		out.WriteString(span.text)
		pos = end
	}
	return out.String(), true
}

// restoreProtectedSpansSequential is the exact previous behavior: one
// strings.ReplaceAll per span, in id order. It is only used when a rewrite
// produced overlapping or incomplete markers.
func restoreProtectedSpansSequential(result, prefix string, spans []protectedSpan) (string, bool) {
	for _, span := range spans {
		result = strings.ReplaceAll(result, span.marker, span.text)
	}
	if strings.Contains(result, prefix) {
		return result, false
	}
	return result, true
}
